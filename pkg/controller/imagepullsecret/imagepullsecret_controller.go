package imagepullsecret

import (
	"context"
	"fmt"
	"time"

	credentialmanagerv1alpha1 "github.com/mgoltzsche/credential-manager/pkg/apis/credentialmanager/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	RequeueDelaySeconds      = 30 * time.Second
	RequeueDelayErrorSeconds = 5 * time.Second
)

var log = logf.Log.WithName("controller_imagepullsecret")

// Add creates a new ImagePullSecret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileImagePullSecret{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("imagepullsecret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ImagePullSecret
	err = c.Watch(&source.Kind{Type: &credentialmanagerv1alpha1.ImagePullSecret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Secrets and requeue the owner ImagePullSecret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &credentialmanagerv1alpha1.ImagePullSecret{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileImagePullSecret implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImagePullSecret{}

// ReconcileImagePullSecret reconciles a ImagePullSecret object
type ReconcileImagePullSecret struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ImagePullSecret object and makes changes based on the state read
// and what is in the ImagePullSecret.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImagePullSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ImagePullSecret")

	// Fetch the ImagePullSecret instance
	instance := &credentialmanagerv1alpha1.ImagePullSecret{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Define a new Secret object
	secret := newSecretForCR(instance)

	// Set ImagePullSecret instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	rotationInterval := time.Minute * 1

	// Check if the Secret already exists
	found := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.rotatePassword(instance, secret)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Create(context.TODO(), secret)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Secret created successfully - schedule renewal check
		return reconcile.Result{RequeueAfter: rotationInterval}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Rotate the credentials
	secretAge := time.Now().Sub(instance.Status.RotationDate.Time)
	if secretAge > rotationInterval {
		reqLogger.Info("Rotating Secret", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
		err = r.rotatePassword(instance, found)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Secret updated successfully - schedule renewal check
		return reconcile.Result{RequeueAfter: rotationInterval}, nil
	}

	// Secret up-to-date and untouched - schedule next renewal check
	return reconcile.Result{RequeueAfter: rotationInterval - secretAge + 30*time.Second}, nil
}

func (r *ReconcileImagePullSecret) rotatePassword(cr *credentialmanagerv1alpha1.ImagePullSecret, secret *corev1.Secret) (err error) {
	cr.Status.Rotation++
	user := newUserNameForCR(cr, cr.Status.Rotation)
	secret.Data["user"] = user
	passwd := secret.Data["nextpassword"]
	if passwd == nil {
		passwd = generatePassword()
		if cr.Status.Passwords, err = shiftPassword(cr.Status.Passwords, passwd); err != nil {
			return
		}
	}
	secret.Data["password"] = passwd
	secret.Data[".dockerconfigjson"] = generateDockerConfigJson("https://myregistry", string(user), string(passwd))
	nextPasswd := generatePassword()
	if cr.Status.Passwords, err = shiftPassword(cr.Status.Passwords, nextPasswd); err != nil {
		return
	}
	secret.Data["nextpassword"] = nextPasswd
	cr.Status.RotationDate = metav1.Time{time.Now()}
	return
}

func shiftPassword(old []string, newPasswd []byte) (hashed []string, err error) {
	b, err := bcryptPassword(newPasswd)
	if err != nil {
		return nil, err
	}
	hashed = append([]string{string(b)}, old...)
	if len(hashed) > 2 {
		return hashed[:2], nil
	}
	return hashed, nil
}

func newUserNameForCR(cr *credentialmanagerv1alpha1.ImagePullSecret, rotation uint64) []byte {
	return []byte(fmt.Sprintf("%s/%s/%d", cr.Namespace, cr.Name, rotation))
}

func newSecretForCR(cr *credentialmanagerv1alpha1.ImagePullSecret) *corev1.Secret {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-image-pull-secret",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{},
	}
}

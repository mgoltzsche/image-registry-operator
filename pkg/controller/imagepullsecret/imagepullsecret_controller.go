package imagepullsecret

import (
	"context"
	"fmt"
	"strconv"
	"time"

	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/status"
	"golang.org/x/crypto/bcrypt"
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
	ConditionReady           = "ready"
	ConditionSynced          = "synced"
	AnnotationSecretRotation = "registry.mgoltzsche.github.com/rotation"
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
	err = c.Watch(&source.Kind{Type: &registryapi.ImagePullSecret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Secrets and requeue the owner ImagePullSecret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryapi.ImagePullSecret{},
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
	instance := &registryapi.ImagePullSecret{}
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

	putSecret := func(ctx context.Context, o runtime.Object) error { return r.client.Create(ctx, o) }
	create := true

	// Check if the Secret already exists
	found := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
	if err == nil {
		secret = found
		putSecret = func(ctx context.Context, o runtime.Object) error { return r.client.Update(ctx, o) }
		create = false
	} else if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	pwAge := time.Now().Sub(instance.Status.RotationDate.Time)
	secretRotation, _ := strconv.Atoi(secret.Annotations[AnnotationSecretRotation])
	if pwAge > rotationInterval || secretRotation == 0 {
		reqLogger.Info("Updating Secret", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
		if instance.Status.Rotation != uint64(secretRotation+1) {
			err = rotatePassword(instance, secret)
			if err != nil {
				return reconcile.Result{}, err
			}
			if create {
				instance.Status.RotationDate = metav1.Time{time.Time{}.Add(time.Second)}
				instance.Status.Conditions.SetCondition(status.Condition{
					Type:   ConditionReady,
					Status: corev1.ConditionFalse,
				})
			}
			instance.Status.Conditions.SetCondition(status.Condition{
				Type:    ConditionSynced,
				Status:  corev1.ConditionFalse,
				Reason:  "SecretOutOfSync",
				Message: "Secret is not in sync with CR",
			})
			err = r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			err = putSecret(context.TODO(), secret)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		instance.Status.Rotation++
		instance.Status.RotationDate = metav1.Time{time.Now()}
		instance.Status.Conditions.SetCondition(status.Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		})
		instance.Status.Conditions.SetCondition(status.Condition{
			Type:   ConditionSynced,
			Status: corev1.ConditionTrue,
		})
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Secret & CR updated - schedule renewal
		return reconcile.Result{RequeueAfter: rotationInterval}, nil
	}

	// Secret & CR are up-to-date and untouched - schedule next renewal check
	return reconcile.Result{RequeueAfter: rotationInterval - pwAge + 30*time.Second}, nil
}

func rotatePassword(cr *registryapi.ImagePullSecret, secret *corev1.Secret) (err error) {
	rotationCount := cr.Status.Rotation + 1
	activeHashedPws := cr.Status.Passwords
	user := newUserNameForCR(cr, rotationCount)
	passwd := secret.Data["nextpassword"]
	if cr.Status.Conditions.IsFalseFor(ConditionSynced) && len(cr.Status.Passwords) > 0 {
		// Remove last added hashed password which did not make it into the secret
		cr.Status.Passwords = cr.Status.Passwords[0 : len(cr.Status.Passwords)-1]
	}

	if passwd == nil || !matchPassword(passwd, cr.Status.Passwords) {
		passwd = generatePassword()
		if activeHashedPws, err = shiftPassword(activeHashedPws, passwd); err != nil {
			return
		}
	}
	nextPasswd := generatePassword()
	if activeHashedPws, err = shiftPassword(activeHashedPws, nextPasswd); err != nil {
		return
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[AnnotationSecretRotation] = strconv.Itoa(int(cr.Status.Rotation + 1))
	secret.Data["user"] = user
	secret.Data["password"] = passwd
	secret.Data["nextpassword"] = nextPasswd
	secret.Data[".dockerconfigjson"] = generateDockerConfigJson("https://myregistry", string(user), string(passwd))
	cr.Status.Passwords = activeHashedPws
	return
}

func matchPassword(pw []byte, hashed []string) bool {
	for _, hashedPw := range hashed {
		if err := bcrypt.CompareHashAndPassword([]byte(hashedPw), pw); err == nil {
			return true
		}
	}
	return false
}

func shiftPassword(old []string, newPasswd []byte) (hashed []string, err error) {
	b, err := bcryptPassword(newPasswd)
	if err != nil {
		return nil, err
	}
	hashed = append(old, string(b))
	if len(hashed) > 2 {
		hashed = hashed[1:]
	}
	return hashed, nil
}

func newUserNameForCR(cr *registryapi.ImagePullSecret, rotation uint64) []byte {
	return []byte(fmt.Sprintf("%s/%s/%d", cr.Namespace, cr.Name, rotation))
}

func newSecretForCR(cr *registryapi.ImagePullSecret) *corev1.Secret {
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

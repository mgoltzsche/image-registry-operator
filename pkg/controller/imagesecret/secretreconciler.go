package imagesecret

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/auth"
	"github.com/mgoltzsche/image-registry-operator/pkg/controller/imageregistry"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	RequeueDelaySeconds      = 30 * time.Second
	RequeueDelayErrorSeconds = 5 * time.Second
	ConditionReady           = "ready"
	ConditionSynced          = "synced"
	ReasonRegistryNotFound   = "RegistryNotFound"
	EnvDefaultRegistryName   = "OPERATOR_DEFAULT_REGISTRY_NAME"
	AnnotationSecretRotation = "registry.mgoltzsche.github.com/rotation"
	secretCaCertKey          = "ca.crt"
)

type ReconcileImageSecretConfig struct {
	Intent          registryapi.ImageSecretType
	SecretType      corev1.SecretType
	DockerConfigKey string
	CRFactory       SecretResourceFactory
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger logr.Logger, cfg ReconcileImageSecretConfig) reconcile.Reconciler {
	defaultRegistryName := os.Getenv(EnvDefaultRegistryName)
	if defaultRegistryName == "" {
		defaultRegistryName = "registry"
	}
	ns, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		ns = os.Getenv("WATCH_NAMESPACE")
		if ns == "" || ns == "*" {
			panic("could not detect operator namespace")
		}
	}
	return &ReconcileImageSecret{
		client:          mgr.GetClient(),
		scheme:          mgr.GetScheme(),
		cache:           mgr.GetCache(),
		logger:          logger,
		cfg:             cfg,
		defaultRegistry: registryapi.ImageRegistryRef{Name: defaultRegistryName, Namespace: ns},
	}
}

type SecretResourceFactory func() registryapi.ImageSecretInterface

// blank assignment to verify that ReconcileImageSecret implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImageSecret{}

type ReconcileImageSecret struct {
	client          client.Client
	scheme          *runtime.Scheme
	cache           cache.Cache
	logger          logr.Logger
	cfg             ReconcileImageSecretConfig
	defaultRegistry registryapi.ImageRegistryRef
}

// Reconcile reads that state of the cluster for a ImagePullSecret object and makes changes based on the state read
// and what is in the ImagePullSecret.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImageSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.logger.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ImagePullSecret")

	// Fetch the ImagePullSecret instance
	instance := r.cfg.CRFactory()
	instanceStatus := instance.GetStatus()
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
	secret := r.newSecretForCR(instance)

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

	registry, err := r.getRegistryForCR(instance)
	if err != nil {
		instanceStatus.Conditions.SetCondition(status.Condition{
			Type:    ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  ReasonRegistryNotFound,
			Message: err.Error(),
		})
		instanceStatus.Conditions.SetCondition(status.Condition{
			Type:    ConditionSynced,
			Status:  corev1.ConditionFalse,
			Reason:  ReasonRegistryNotFound,
			Message: err.Error(),
		})
		err = r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	hostnameCaChanged := string(secret.Data["hostname"]) != registry.Hostname || string(secret.Data["ca.crt"]) != string(registry.CA)
	pwAge := time.Duration(0)
	if instanceStatus.RotationDate != nil {
		pwAge = time.Now().Sub(instanceStatus.RotationDate.Time)
	}
	secretRotation, _ := strconv.Atoi(secret.Annotations[AnnotationSecretRotation])
	needsRenewal := instanceStatus.RotationDate == nil || pwAge > rotationInterval
	if needsRenewal || hostnameCaChanged {
		reqLogger.Info("Updating Secret", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
		if int(instanceStatus.Rotation) <= secretRotation {
			err = r.rotatePassword(instance, registry, secret)
			if err != nil {
				return reconcile.Result{}, err
			}
			if create {
				instanceStatus.Conditions.SetCondition(status.Condition{
					Type:   ConditionReady,
					Status: corev1.ConditionFalse,
				})
			}
			instanceStatus.Conditions.SetCondition(status.Condition{
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
		instanceStatus.Rotation++
		instanceStatus.RotationDate = &metav1.Time{time.Now()}
		instanceStatus.Conditions.SetCondition(status.Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionTrue,
		})
		instanceStatus.Conditions.SetCondition(status.Condition{
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

func (r *ReconcileImageSecret) rotatePassword(cr registryapi.ImageSecretInterface, reg *targetRegistry, secret *corev1.Secret) (err error) {
	crStatus := cr.GetStatus()
	rotationCount := crStatus.Rotation + 1
	activeHashedPws := crStatus.Passwords
	user := r.newUserNameForCR(cr, rotationCount)
	passwd := secret.Data["nextpassword"]
	if crStatus.Conditions.IsFalseFor(ConditionSynced) && len(crStatus.Passwords) > 0 {
		// Remove last added hashed password which did not make it into the secret
		crStatus.Passwords = crStatus.Passwords[0 : len(crStatus.Passwords)-1]
	}

	if passwd == nil || !auth.HashedPasswords(crStatus.Passwords).MatchPassword(string(passwd)) {
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
	secret.Annotations[AnnotationSecretRotation] = strconv.Itoa(int(crStatus.Rotation + 1))
	secret.Data = map[string][]byte{}
	secret.Data["user"] = user
	secret.Data["password"] = passwd
	secret.Data["nextpassword"] = nextPasswd
	secret.Data["hostname"] = []byte(reg.Hostname)
	secret.Data["ca.crt"] = reg.CA
	secret.Data[r.cfg.DockerConfigKey] = generateDockerConfigJson(reg.Hostname, string(user), string(passwd))
	crStatus.Passwords = activeHashedPws
	return
}

func (r *ReconcileImageSecret) newUserNameForCR(cr registryapi.ImageSecretInterface, rotation uint64) []byte {
	return []byte(fmt.Sprintf("%s/%s/%s/%d", cr.GetNamespace(), cr.GetName(), r.cfg.Intent, rotation))
}

func (r *ReconcileImageSecret) newSecretForCR(cr registryapi.ImageSecretInterface) *corev1.Secret {
	labels := map[string]string{
		"app": cr.GetName(),
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-image-%s-secret", cr.GetName(), r.cfg.Intent),
			Namespace: cr.GetNamespace(),
			Labels:    labels,
		},
		Type: r.cfg.SecretType,
		Data: map[string][]byte{},
	}
}

func (r *ReconcileImageSecret) getRegistryForCR(cr registryapi.ImageSecretInterface) (reg *targetRegistry, err error) {
	registry := cr.GetRegistryRef()
	if registry == nil {
		registry = &r.defaultRegistry
	} else if registry.Namespace == "" {
		registry.Namespace = cr.GetNamespace()
	}
	registryKey := types.NamespacedName{Name: registry.Name, Namespace: registry.Namespace}
	return r.getRegistry(registryKey)
}

func (r *ReconcileImageSecret) getRegistry(registryKey types.NamespacedName) (reg *targetRegistry, err error) {
	ctx := context.TODO()
	registryCR := &registryapi.ImageRegistry{}
	if err = r.cache.Get(ctx, registryKey, registryCR); err != nil {
		return
	}
	if !registryCR.Status.Conditions.IsTrueFor(imageregistry.ConditionReady) {
		return nil, fmt.Errorf("ImageRegistry %s is not ready", registryKey)
	}
	key := types.NamespacedName{Name: imageregistry.TLSSecretNameForCR(registryCR), Namespace: registryKey.Namespace}
	secret := &corev1.Secret{}
	if err = r.cache.Get(ctx, key, secret); err != nil {
		return
	}
	caCert, ok := secret.Data[secretCaCertKey]
	if !ok {
		return nil, fmt.Errorf("CA cert Secret %s does not contain %s", key, secretCaCertKey)
	}
	return &targetRegistry{
		Hostname: registryCR.Status.Hostname,
		CA:       caCert,
	}, nil
}

type targetRegistry struct {
	Hostname string
	CA       []byte
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

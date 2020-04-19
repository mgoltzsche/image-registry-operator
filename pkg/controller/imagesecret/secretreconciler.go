package imagesecret

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
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
	ConditionReady           = "Ready"
	ReasonRegistryNotFound   = "RegistryNotFound"
	ReasonFailedSync         = status.ConditionReason("FailedSync")
	EnvDefaultRegistryName   = "OPERATOR_DEFAULT_REGISTRY_NAME"
	EnvSecretTTL             = "OPERATOR_SECRET_TTL"
	AnnotationSecretRotation = "registry.mgoltzsche.github.com/rotation"
	secretCaCertKey          = "ca.crt"
	defaultAccountTTL        = 24 * time.Hour
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
	accountTTL := defaultAccountTTL
	accountTTLStr := os.Getenv(EnvSecretTTL)
	if accountTTLStr != "" {
		var err error
		accountTTL, err = time.ParseDuration(accountTTLStr)
		if err == nil && accountTTL < 1 {
			err = fmt.Errorf("ttl < 1")
		}
		if err != nil {
			panic(fmt.Sprintf("Unsupported value in env var %s: %v", EnvSecretTTL, err))
		}
	}
	return &ReconcileImageSecret{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		cache:            mgr.GetCache(),
		logger:           logger,
		cfg:              cfg,
		defaultRegistry:  registryapi.ImageRegistryRef{Name: defaultRegistryName, Namespace: ns},
		accountTTL:       accountTTL,
		rotationInterval: accountTTL / 2,
	}
}

type SecretResourceFactory func() registryapi.ImageSecretInterface

// blank assignment to verify that ReconcileImageSecret implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImageSecret{}

type ReconcileImageSecret struct {
	client           client.Client
	scheme           *runtime.Scheme
	cache            cache.Cache
	logger           logr.Logger
	cfg              ReconcileImageSecretConfig
	defaultRegistry  registryapi.ImageRegistryRef
	rotationInterval time.Duration
	accountTTL       time.Duration
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

	// Fetch the registry
	registry, err := r.getRegistryForCR(instance)
	if err != nil {
		instanceStatus.Conditions.SetCondition(status.Condition{
			Type:    ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  ReasonRegistryNotFound,
			Message: err.Error(),
		})
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	// Fetch ImageRegistryAccount
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	accountExists, err := r.get(context.TODO(), account)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch Secret
	secret := &corev1.Secret{}
	secret.Name = fmt.Sprintf("%s-image-%s-secret", instance.GetName(), r.cfg.Intent)
	secret.Namespace = instance.GetNamespace()
	secretExists, err := r.get(context.TODO(), secret)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update ImageRegistryAccount & Secret
	hostnameCaChanged := string(secret.Data["hostname"]) != registry.Hostname || string(secret.Data["ca.crt"]) != string(registry.CA)
	needsRenewal := time.Now().Sub(account.CreationTimestamp.Time) > r.rotationInterval
	secretOutOfSync := secret.Annotations == nil || secret.Annotations[AnnotationSecretRotation] != strconv.FormatInt(instance.GetStatus().Rotation, 10)
	if !accountExists || !secretExists || secretOutOfSync || needsRenewal || hostnameCaChanged {
		err = r.rotatePassword(instance, registry, secret, reqLogger)
		if err != nil {
			if instanceStatus.Conditions.SetCondition(status.Condition{
				Type:    ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  ReasonFailedSync,
				Message: err.Error(),
			}) {
				r.client.Status().Update(context.TODO(), instance)
			}
			return reconcile.Result{}, err
		}
	}

	if instanceStatus.Conditions.SetCondition(status.Condition{
		Type:   ConditionReady,
		Status: corev1.ConditionTrue,
	}) {
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Secret & CR are up-to-date and untouched - schedule next renewal check
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ReconcileImageSecret) get(ctx context.Context, obj runtime.Object) (bool, error) {
	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return false, err
	}
	err = r.client.Get(ctx, key, obj)
	if errors.IsNotFound(err) {
		return false, nil
	}
	return true, err
}

func (r *ReconcileImageSecret) rotatePassword(instance registryapi.ImageSecretInterface, registry *targetRegistry, secret *corev1.Secret, reqLogger logr.Logger) (err error) {
	newPassword := generatePassword()
	newPasswordHash, err := bcryptPassword(newPassword)
	if err != nil {
		return
	}

	// Increment CR rotation count.
	// This must happen before account and secret are written to avoid
	// replacing an existing account - handling accounts immutable.
	instance.GetStatus().Rotation++
	instance.GetStatus().RotationDate = &metav1.Time{time.Now()}
	if err = r.client.Status().Update(context.TODO(), instance); err != nil {
		return
	}
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	account.Spec.TTL = &metav1.Duration{r.accountTTL}
	account.Spec.Password = string(newPasswordHash)
	account.Spec.Labels = map[string][]string{
		"namespace":  []string{instance.GetNamespace()},
		"name":       []string{instance.GetName()},
		"accessMode": []string{string(instance.GetRegistryAccessMode())},
	}
	if err = controllerutil.SetControllerReference(instance, account, r.scheme); err != nil {
		return
	}
	reqLogger.Info("Creating ImageRegistryAccount", "ImageRegistryAccount.Namespace", account.Namespace, "ImageRegistryAccount.Name", account.Name)
	err = r.client.Create(context.TODO(), account)
	if err != nil {
		// Fail with error if account exists
		// (doing the next attempt with incremented rotation count)
		return
	}

	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Type = r.cfg.SecretType
	secret.Annotations[AnnotationSecretRotation] = strconv.FormatInt(instance.GetStatus().Rotation, 10)
	secret.Data = map[string][]byte{}
	secret.Data["username"] = []byte(account.Name)
	secret.Data["password"] = newPassword
	secret.Data["hostname"] = []byte(registry.Hostname)
	secret.Data["ca.crt"] = registry.CA
	secret.Data[r.cfg.DockerConfigKey] = generateDockerConfigJson(registry.Hostname, account.Name, string(newPassword))
	if err = controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
		return
	}
	if len(secret.UID) == 0 {
		reqLogger.Info("Creating Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.client.Create(context.TODO(), secret)
	} else {
		reqLogger.Info("Updating Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.client.Update(context.TODO(), secret)
	}
	return
}

func accountNameForCR(cr registryapi.ImageSecretInterface) string {
	return fmt.Sprintf("%s.%s.%s.%d", cr.GetRegistryAccessMode(), cr.GetNamespace(), cr.GetName(), cr.GetStatus().Rotation)
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
		Namespace: registryCR.GetNamespace(),
		Hostname:  registryCR.Status.Hostname,
		CA:        caCert,
	}, nil
}

type targetRegistry struct {
	Namespace string
	Hostname  string
	CA        []byte
}

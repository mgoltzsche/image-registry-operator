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
	"github.com/mgoltzsche/image-registry-operator/pkg/passwordgen"
	"github.com/mgoltzsche/image-registry-operator/pkg/registriesconf"
	"github.com/mgoltzsche/image-registry-operator/pkg/torequests"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	RequeueDelaySeconds         = 30 * time.Second
	RequeueDelayErrorSeconds    = 5 * time.Second
	EnvDefaultRegistryName      = "OPERATOR_DEFAULT_REGISTRY_NAME"
	EnvDefaultRegistryNamespace = "OPERATOR_DEFAULT_REGISTRY_NAMESPACE"
	EnvSecretTTL                = "OPERATOR_SECRET_TTL"
	annotationSecretRotation    = "registry.mgoltzsche.github.com/rotation"
	defaultAccountTTL           = 24 * time.Hour
)

// WatchSecondaryResources watches resources created or referenced by a secret CR
func WatchSecondaryResources(c controller.Controller, ownerType runtime.Object, registryMap torequests.Map, accountMap torequests.AnnotationToRequest) (err error) {
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    ownerType,
	})
	if err != nil {
		return
	}
	err = c.Watch(&source.Kind{Type: &registryapi.ImageRegistryAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    ownerType,
	})
	if err != nil {
		return
	}
	err = c.Watch(&source.Kind{Type: &registryapi.ImageRegistry{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: registryMap})
	if err != nil {
		return
	}
	err = c.Watch(&source.Kind{Type: &registryapi.ImageRegistryAccount{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: accountMap})
	if err != nil {
		return
	}
	return
}

// ReconcileImageSecretConfig image secret CR reconciler
type ReconcileImageSecretConfig struct {
	Intent            registryapi.ImageSecretType
	SecretType        corev1.SecretType
	DockerConfigKey   string
	CRFactory         SecretResourceFactory
	AccountAnnotation torequests.AnnotationToRequest
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, registriesMap torequests.Map, logger logr.Logger, cfg ReconcileImageSecretConfig) reconcile.Reconciler {
	defaultRegistryRef := registryapi.ImageRegistryRef{
		Name:      os.Getenv(EnvDefaultRegistryName),
		Namespace: os.Getenv(EnvDefaultRegistryNamespace),
	}
	if defaultRegistryRef.Name == "" {
		defaultRegistryRef.Name = "registry"
	}
	if defaultRegistryRef.Namespace == "" {
		ns, err := k8sutil.GetOperatorNamespace()
		if err != nil {
			ns = os.Getenv("WATCH_NAMESPACE")
			if ns == "" {
				panic(fmt.Sprintf("could not detect operator namespace to derive %s - set it alternatively", EnvDefaultRegistryNamespace))
			}
		}
		defaultRegistryRef.Namespace = ns
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
		defaultRegistry:  defaultRegistryRef,
		accountTTL:       accountTTL,
		rotationInterval: accountTTL / 2,
		registriesMap:    registriesMap,
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
	registriesMap    torequests.Map
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

	// Map registry changes to secret CR reconcile requests (in watch handler)
	registryKey := r.getRegistryKeyForCR(instance)
	r.registriesMap.Put(request.NamespacedName, []types.NamespacedName{registryKey})

	// Fetch the registry
	registry, err := r.getRegistry(registryKey)
	if err != nil {
		instanceStatus.Conditions.SetCondition(status.Condition{
			Type:    registryapi.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  registryapi.ReasonRegistryUnavailable,
			Message: err.Error(),
		})
		r.client.Status().Update(context.TODO(), instance)
		if errors.IsNotFound(err) {
			// Do not reconcile when registry doesn't exist
			// (requires reconcile event when registry changes)
			return reconcile.Result{}, nil
		}
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
	secret.Name = secretNameForCR(instance)
	secret.Namespace = instance.GetNamespace()
	secretExists, err := r.get(context.TODO(), secret)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update ImageRegistryAccount & Secret
	hostnameCaChanged := string(secret.Data[registryapi.SecretKeyRegistry]) != registry.Hostname || string(secret.Data["ca.crt"]) != string(registry.CA)
	needsRenewal := time.Now().Sub(account.CreationTimestamp.Time) > r.rotationInterval
	secretOutOfSync := secret.Annotations == nil || secret.Annotations[annotationSecretRotation] != strconv.FormatInt(instance.GetStatus().Rotation, 10)
	if !accountExists || !secretExists || secretOutOfSync || needsRenewal || hostnameCaChanged {
		err = r.rotatePassword(instance, registry, secret, reqLogger)
		if err != nil {
			if instanceStatus.Conditions.SetCondition(status.Condition{
				Type:    registryapi.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  registryapi.ReasonFailedSync,
				Message: err.Error(),
			}) {
				r.client.Status().Update(context.TODO(), instance)
			}
			return reconcile.Result{}, err
		}
	}

	if instanceStatus.Conditions.SetCondition(status.Condition{
		Type:   registryapi.ConditionReady,
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
	newPassword := passwordgen.GeneratePassword()
	newPasswordHash, err := passwordgen.BcryptPassword(newPassword)
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
	crName := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	account.Annotations = map[string]string{string(r.cfg.AccountAnnotation): crName.String()}
	account.Spec.TTL = &metav1.Duration{r.accountTTL}
	account.Spec.Password = string(newPasswordHash)
	account.Spec.Labels = map[string][]string{
		"namespace":  []string{instance.GetNamespace()},
		"name":       []string{instance.GetName()},
		"accessMode": []string{string(instance.GetRegistryAccessMode())},
	}
	reqLogger.Info("Creating ImageRegistryAccount", "ImageRegistryAccount.Namespace", account.Namespace, "ImageRegistryAccount.Name", account.Name)
	err = r.client.Create(context.TODO(), account)
	if err != nil {
		// Fail with error if account exists
		// (doing the next attempt with incremented rotation count/name)
		return
	}

	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	dockerConfig := (&registriesconf.DockerConfig{}).
		AddAuth(registry.Hostname, account.Name, string(newPassword)).
		JSON()
	secret.Type = r.cfg.SecretType
	secret.Annotations[annotationSecretRotation] = strconv.FormatInt(instance.GetStatus().Rotation, 10)
	secret.Data = map[string][]byte{}
	secret.Data[registryapi.SecretKeyUsername] = []byte(account.Name)
	secret.Data[registryapi.SecretKeyPassword] = newPassword
	secret.Data[registryapi.SecretKeyRegistry] = []byte(registry.Hostname)
	secret.Data[registryapi.SecretKeyCaCert] = registry.CA
	secret.Data[r.cfg.DockerConfigKey] = dockerConfig
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

func secretNameForCR(cr registryapi.ImageSecretInterface) string {
	return fmt.Sprintf("image%ssecret-%s", cr.GetRegistryAccessMode(), cr.GetName())
}

func (r *ReconcileImageSecret) getRegistryKeyForCR(cr registryapi.ImageSecretInterface) (reg types.NamespacedName) {
	registry := cr.GetRegistryRef()
	if registry == nil {
		registry = &r.defaultRegistry
	} else if registry.Namespace == "" {
		registry.Namespace = cr.GetNamespace()
	}
	return types.NamespacedName{Name: registry.Name, Namespace: registry.Namespace}
}

func (r *ReconcileImageSecret) getRegistry(registryKey types.NamespacedName) (reg *targetRegistry, err error) {
	ctx := context.TODO()
	registryCR := &registryapi.ImageRegistry{}
	if err = r.client.Get(ctx, registryKey, registryCR); err != nil {
		return
	}
	if !registryCR.Status.Conditions.IsTrueFor(imageregistry.ConditionReady) {
		// Allow caller to not reconcile when registry not ready
		key := registryKey.String()
		notFound := errors.NewNotFound(registryapi.SchemeGroupVersion.WithResource(key).GroupResource(), "")
		return nil, fmt.Errorf("ImageRegistry is not ready: %w", notFound)
	}
	key := types.NamespacedName{Name: registryCR.Status.TLSSecretName, Namespace: registryKey.Namespace}
	secret := &corev1.Secret{}
	if err = r.cache.Get(ctx, key, secret); err != nil {
		return
	}
	return &targetRegistry{
		Namespace: registryCR.GetNamespace(),
		Hostname:  registryCR.Status.Hostname,
		CA:        secret.Data[registryapi.SecretKeyCaCert],
	}, nil
}

type targetRegistry struct {
	Namespace string
	Hostname  string
	CA        []byte
}

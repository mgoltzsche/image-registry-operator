package imagesecret

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/backrefs"
	"github.com/mgoltzsche/image-registry-operator/pkg/controller/imageregistry"
	"github.com/mgoltzsche/image-registry-operator/pkg/merge"
	"github.com/mgoltzsche/image-registry-operator/pkg/passwordgen"
	"github.com/mgoltzsche/image-registry-operator/pkg/registriesconf"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	finalizer                   = "registry.mgoltzsche.github.com/accounts"
)

// WatchSecondaryResources watches resources created or referenced by a secret CR
func WatchSecondaryResources(c controller.Controller, ownerType runtime.Object, accountLabel string) (err error) {
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
	accountToSecret := backrefs.LabelToRequest(accountLabel)
	err = c.Watch(&source.Kind{Type: &registryapi.ImageRegistryAccount{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: accountToSecret})
	if err != nil {
		return
	}
	return
}

// ReconcileImageSecretConfig image secret CR reconciler
type ReconcileImageSecretConfig struct {
	Intent          registryapi.ImageSecretType
	SecretType      corev1.SecretType
	DockerConfigKey string
	CRFactory       SecretResourceFactory
	AccountLabel    string
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger logr.Logger, cfg ReconcileImageSecretConfig) reconcile.Reconciler {
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
			ns = os.Getenv(k8sutil.WatchNamespaceEnvVar)
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
		dnsZone:          imageregistry.DNSZone(),
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
	dnsZone          string
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

	// finalize: Delete associated ImageRegistryAccounts
	isFinalizerPresent := merge.HasFinalizer(instance, finalizer)
	if !instance.GetDeletionTimestamp().IsZero() {
		if isFinalizerPresent {
			reqLogger.Info("Finalizing")
			registryNs := instance.GetStatus().Registry.Namespace
			if registryNs != "" {
				err = r.deleteOrphanAccounts(reqLogger, request.NamespacedName, registryNs)
				if err != nil {
					reqLogger.Error(err, "finalizer failed to delete accounts")
					return reconcile.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(instance, finalizer)
			err = r.client.Update(context.TODO(), instance)
		}
		return reconcile.Result{}, err
	}

	// Add finalizer
	if !isFinalizerPresent {
		controllerutil.AddFinalizer(instance, finalizer)
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			r.setSyncStatus(instance, registryapi.ConditionSynced, corev1.ConditionFalse, registryapi.ReasonFailedUpdate, err.Error())
			return reconcile.Result{}, err
		}
		// Stop here since update triggered another reconcile request anyway
		return reconcile.Result{}, nil
	}

	// Fetch the registry
	registry, err := r.getRegistry(r.getRegistryKeyForCR(instance))
	if err != nil {
		err = r.setSyncStatus(instance, registryapi.ConditionReady, corev1.ConditionFalse, registryapi.ReasonRegistryUnavailable, err.Error())
		// Reconcile delayed when registry does not exist (yet)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
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
	now := time.Now()
	needsRenewal := time.Now().Sub(account.CreationTimestamp.Time) > r.rotationInterval
	secretOutOfSync := secret.Annotations == nil || secret.Annotations[annotationSecretRotation] != strconv.FormatInt(instance.GetStatus().Rotation, 10)
	if !accountExists || !secretExists || secretOutOfSync || needsRenewal || hostnameCaChanged {
		err = r.rotatePassword(instance, registry, secret, reqLogger)
		if err != nil {
			err = r.setSyncStatus(instance, registryapi.ConditionReady, corev1.ConditionFalse, registryapi.ReasonFailedSync, err.Error())
			return reconcile.Result{}, err
		}
	}

	err = r.setSyncStatus(instance, registryapi.ConditionReady, corev1.ConditionTrue, "", "")
	if err != nil {
		return reconcile.Result{}, err
	}

	// CR, account and secret are up-to-date - schedule next renewal check
	renewalTTL := account.CreationTimestamp.Time.Add(r.rotationInterval).Sub(now) + 30*time.Second
	return reconcile.Result{RequeueAfter: renewalTTL}, nil
}

func (r *ReconcileImageSecret) setSyncStatus(cr registryapi.ImageSecretInterface, ctype status.ConditionType, s corev1.ConditionStatus, reason status.ConditionReason, msg string) error {
	syncCond := status.Condition{
		Type:    ctype,
		Status:  s,
		Reason:  reason,
		Message: msg,
	}
	st := cr.GetStatus()
	generation := cr.GetGeneration()
	if st.Conditions.SetCondition(syncCond) ||
		st.ObservedGeneration != generation {
		st.ObservedGeneration = generation
		return r.client.Status().Update(context.TODO(), cr)
	}
	return nil
}

func (r *ReconcileImageSecret) deleteOrphanAccounts(reqLogger logr.Logger, name types.NamespacedName, registryNamespace string) error {
	opts := client.DeleteAllOfOptions{}
	opts.LabelSelector = labels.SelectorFromSet(map[string]string{r.cfg.AccountLabel: name.String()})
	opts.Namespace = registryNamespace
	return r.client.DeleteAllOf(context.TODO(), &registryapi.ImageRegistryAccount{}, &opts)
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

	// Delete old registry account if any associated
	lastRegistryNs := instance.GetStatus().Registry.Namespace
	if lastRegistryNs != "" && lastRegistryNs != registry.Namespace {
		key := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		err = r.deleteOrphanAccounts(reqLogger, key, lastRegistryNs)
	}

	// Increment CR rotation count.
	// This must happen before account and secret are written to avoid
	// replacing an existing account - handling accounts immutable.
	instance.GetStatus().Rotation++
	instance.GetStatus().RotationDate = &metav1.Time{time.Now()}
	instance.GetStatus().Registry.Namespace = registry.Namespace
	if err = r.client.Status().Update(context.TODO(), instance); err != nil {
		return
	}
	crName := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	account.Labels = map[string]string{r.cfg.AccountLabel: backrefs.ToMapValue(crName)}
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
	if !registryCR.Status.Conditions.IsTrueFor(registryapi.ConditionReady) {
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
		Hostname:  imageregistry.RegistryHostname(registryCR, r.dnsZone),
		CA:        secret.Data[registryapi.SecretKeyCaCert],
	}, nil
}

type targetRegistry struct {
	Namespace string
	Hostname  string
	CA        []byte
}

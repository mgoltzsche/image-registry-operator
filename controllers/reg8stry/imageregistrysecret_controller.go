/*
Copyright 2021 Max Goltzsche.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reg8stry

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	registryapi "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
	"github.com/mgoltzsche/reg8stry/internal/backrefs"
	"github.com/mgoltzsche/reg8stry/internal/passwordgen"
	"github.com/mgoltzsche/reg8stry/internal/registriesconf"
	"github.com/mgoltzsche/reg8stry/internal/status"
)

const (
	annotationSecretRotation = "reg8stry.mgoltzsche.github.com/rotation"
	accountFinalizer         = "reg8stry.mgoltzsche.github.com/accounts"
)

// ImageSecretConfig image secret CR config
type ImageSecretConfig struct {
	DefaultRegistry               registryapi.ImageRegistryRef
	DNSZone                       string
	AccountTTL                    time.Duration
	RotationInterval              time.Duration
	RequeueDelayOnMissingRegistry time.Duration
}

type SecretResourceFactory func() registryapi.ImageSecretInterface

// ImageSecretReconcilerConfig image secret CR reconciler config
type ImageSecretReconcilerConfig struct {
	ImageSecretConfig
	CRFactory       SecretResourceFactory
	SecretType      corev1.SecretType
	DockerConfigKey string
	AccountLabel    string
	intent          registryapi.ImageSecretType
}

// ImageSecretReconciler reconciles an ImagePullSecret object
type ImageSecretReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	logger logr.Logger
	cfg    ImageSecretReconcilerConfig
}

// NewImageSecretReconciler returns a new reconcile.Reconciler
func NewImageSecretReconciler(cfg ImageSecretReconcilerConfig) *ImageSecretReconciler {
	cfg.intent = cfg.CRFactory().GetRegistryAccessMode()
	return &ImageSecretReconciler{cfg: cfg}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.logger = mgr.GetLogger()
	customResource := r.cfg.CRFactory()

	return ctrl.NewControllerManagedBy(mgr).
		For(customResource).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    customResource,
		}).
		Watches(&source.Kind{Type: &registryapi.ImageRegistryAccount{}}, handler.EnqueueRequestsFromMapFunc(
			backrefs.LabelToRequestMapper(r.cfg.AccountLabel),
		)).
		Complete(r)
}

//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepullsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepullsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepullsecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepushsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepushsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imagepushsecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ImageSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling Image%sSecret", r.cfg.intent)

	// Fetch the Image*Secret instance
	instance := r.cfg.CRFactory()
	err := r.client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// finalize: Delete associated ImageRegistryAccounts
	isFinalizerPresent := controllerutil.ContainsFinalizer(instance, accountFinalizer)
	if !instance.GetDeletionTimestamp().IsZero() {
		if isFinalizerPresent {
			reqLogger.Info("Finalizing")
			registryNs := instance.GetStatus().Registry.Namespace
			if registryNs != "" {
				err = r.deleteOrphanAccounts(ctx, reqLogger, req.NamespacedName, registryNs)
				if err != nil {
					reqLogger.Error(err, "finalizer failed to delete accounts")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(instance, accountFinalizer)
			err = r.client.Update(ctx, instance)
		}
		return ctrl.Result{}, err
	}

	// Add finalizer
	if !isFinalizerPresent {
		controllerutil.AddFinalizer(instance, accountFinalizer)
		err := r.client.Update(ctx, instance)
		if err != nil {
			r.setSyncStatus(ctx, instance, registryapi.ConditionSynced, metav1.ConditionFalse, registryapi.ReasonFailedSync, err.Error())
			return ctrl.Result{}, err
		}
		// Stop here since update triggered another reconcile request anyway
		return ctrl.Result{}, nil
	}

	// Fetch the registry
	registry, err := r.getRegistry(ctx, r.getRegistryKeyForCR(instance))
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			err = r.setSyncStatus(ctx, instance, registryapi.ConditionReady, metav1.ConditionFalse, registryapi.ReasonRegistryUnavailable, err.Error())
			// Reconcile delayed when registry does not exist (yet)
			return ctrl.Result{RequeueAfter: r.cfg.RequeueDelayOnMissingRegistry}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch ImageRegistryAccount
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	accountExists, err := r.get(ctx, account)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch Secret
	secret := &corev1.Secret{}
	secret.Name = secretNameForCR(instance)
	secret.Namespace = instance.GetNamespace()
	secretExists, err := r.get(ctx, secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update ImageRegistryAccount & Secret
	hostnameCaChanged := string(secret.Data[registryapi.SecretKeyRegistry]) != registry.Hostname || string(secret.Data["ca.crt"]) != string(registry.CA)
	now := time.Now()
	needsRenewal := time.Now().Sub(account.CreationTimestamp.Time) > r.cfg.RotationInterval
	secretOutOfSync := secret.Annotations == nil || secret.Annotations[annotationSecretRotation] != strconv.FormatInt(instance.GetStatus().Rotation, 10)
	if !accountExists || !secretExists || secretOutOfSync || needsRenewal || hostnameCaChanged {
		err = r.rotatePassword(ctx, instance, registry, secret, reqLogger)
		if err != nil {
			err = r.setSyncStatus(ctx, instance, registryapi.ConditionReady, metav1.ConditionFalse, registryapi.ReasonFailedSync, err.Error())
			return ctrl.Result{}, err
		}
	}

	err = r.setSyncStatus(ctx, instance, registryapi.ConditionReady, metav1.ConditionTrue, "", "")
	if err != nil {
		return ctrl.Result{}, err
	}

	// CR, account and secret are up-to-date - schedule next renewal check
	renewalTTL := account.CreationTimestamp.Time.Add(r.cfg.RotationInterval).Sub(now) + 30*time.Second
	return ctrl.Result{RequeueAfter: renewalTTL}, nil
}

func (r *ImageSecretReconciler) setSyncStatus(ctx context.Context, cr registryapi.ImageSecretInterface, ctype string, s metav1.ConditionStatus, reason, msg string) error {
	st := cr.GetStatus()
	generation := cr.GetGeneration()
	cond := metav1.Condition{
		Type:               ctype,
		Status:             s,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: cr.GetGeneration(),
	}
	if status.SetCondition(&st.Conditions, cond) ||
		st.ObservedGeneration != generation {
		st.ObservedGeneration = generation
		return r.client.Status().Update(ctx, cr)
	}
	return nil
}

func (r *ImageSecretReconciler) deleteOrphanAccounts(ctx context.Context, reqLogger logr.Logger, name types.NamespacedName, registryNamespace string) error {
	opts := client.DeleteAllOfOptions{}
	opts.LabelSelector = labels.SelectorFromSet(map[string]string{r.cfg.AccountLabel: name.String()})
	opts.Namespace = registryNamespace
	return r.client.DeleteAllOf(ctx, &registryapi.ImageRegistryAccount{}, &opts)
}

func (r *ImageSecretReconciler) get(ctx context.Context, obj client.Object) (bool, error) {
	key := client.ObjectKeyFromObject(obj)
	err := r.client.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return true, err
}

func (r *ImageSecretReconciler) rotatePassword(ctx context.Context, instance registryapi.ImageSecretInterface, registry *targetRegistry, secret *corev1.Secret, reqLogger logr.Logger) error {
	newPassword := passwordgen.GeneratePassword()
	newPasswordHash, err := passwordgen.BcryptPassword(newPassword)
	if err != nil {
		return err
	}

	// Delete old registry account if any associated
	lastRegistryNs := instance.GetStatus().Registry.Namespace
	if lastRegistryNs != "" && lastRegistryNs != registry.Namespace {
		key := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		err = r.deleteOrphanAccounts(ctx, reqLogger, key, lastRegistryNs)
		if err != nil {
			return err
		}
	}

	// Increment CR rotation count.
	// This must happen before account and secret are written to avoid
	// replacing an existing account - handling accounts immutable.
	instance.GetStatus().Rotation++
	instance.GetStatus().RotationDate = &metav1.Time{time.Now()}
	instance.GetStatus().Registry.Namespace = registry.Namespace
	err = r.client.Status().Update(ctx, instance)
	if err != nil {
		return err
	}
	crName := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
	account := &registryapi.ImageRegistryAccount{}
	account.Name = accountNameForCR(instance)
	account.Namespace = registry.Namespace
	account.Labels = map[string]string{r.cfg.AccountLabel: backrefs.ToMapValue(crName)}
	account.Spec.TTL = &metav1.Duration{r.cfg.AccountTTL}
	account.Spec.Password = string(newPasswordHash)
	account.Spec.Labels = map[string][]string{
		"namespace":  []string{instance.GetNamespace()},
		"name":       []string{instance.GetName()},
		"accessMode": []string{string(instance.GetRegistryAccessMode())},
	}
	reqLogger.Info("Creating ImageRegistryAccount", "ImageRegistryAccount.Namespace", account.Namespace, "ImageRegistryAccount.Name", account.Name)
	err = r.client.Create(ctx, account)
	if err != nil {
		// Fail with error if account exists
		// (doing the next attempt with incremented rotation count/name)
		return err
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
		return err
	}
	secretKey := types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}
	if len(secret.UID) == 0 {
		reqLogger.Info("Creating Secret", "secret", secretKey.String())
		err = r.client.Create(ctx, secret)
	} else {
		reqLogger.Info("Updating Secret", "secret", secretKey.String())
		err = r.client.Update(ctx, secret)
	}
	return err
}

func accountNameForCR(cr registryapi.ImageSecretInterface) string {
	return fmt.Sprintf("%s.%s.%s.%d", cr.GetRegistryAccessMode(), cr.GetNamespace(), cr.GetName(), cr.GetStatus().Rotation)
}

func secretNameForCR(cr registryapi.ImageSecretInterface) string {
	return fmt.Sprintf("image%ssecret-%s", cr.GetRegistryAccessMode(), cr.GetName())
}

func (r *ImageSecretReconciler) getRegistryKeyForCR(cr registryapi.ImageSecretInterface) (reg types.NamespacedName) {
	registry := cr.GetRegistryRef()
	if registry == nil {
		registry = &r.cfg.DefaultRegistry
	} else if registry.Namespace == "" {
		registry.Namespace = cr.GetNamespace()
	}
	return types.NamespacedName{Name: registry.Name, Namespace: registry.Namespace}
}

func (r *ImageSecretReconciler) getRegistry(ctx context.Context, registryKey types.NamespacedName) (*targetRegistry, error) {
	registryCR := &registryapi.ImageRegistry{}
	err := r.client.Get(ctx, registryKey, registryCR)
	if err != nil {
		return nil, err
	}
	readyCondition := status.GetCondition(registryCR.Status.Conditions, registryapi.ConditionReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		// Allow caller to not reconcile when registry not ready
		key := registryKey.String()
		notFound := apierrors.NewNotFound(registryapi.GroupVersion.WithResource(key).GroupResource(), "")
		return nil, fmt.Errorf("ImageRegistry is not ready: %w", notFound)
	}
	key := types.NamespacedName{Name: registryCR.Status.TLSSecretName, Namespace: registryKey.Namespace}
	secret := &corev1.Secret{}
	err = r.client.Get(ctx, key, secret)
	if err != nil {
		return nil, err
	}
	return &targetRegistry{
		Namespace: registryCR.GetNamespace(),
		Hostname:  registryHostname(registryCR, r.cfg.DNSZone),
		CA:        secret.Data[registryapi.SecretKeyCaCert],
	}, nil
}

type targetRegistry struct {
	Namespace string
	Hostname  string
	CA        []byte
}

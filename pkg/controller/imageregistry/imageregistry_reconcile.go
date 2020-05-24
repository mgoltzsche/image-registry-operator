package imageregistry

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/certs"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ConditionSynced  = status.ConditionType("Synced")
	ConditionReady   = status.ConditionType("Ready")
	ReasonFailedSync = status.ConditionReason("FailedSync")
	ReasonUpdating   = status.ConditionReason("Updating")
	EnvDnsZone       = "OPERATOR_DNS_ZONE"
	EnvImageAuth     = "OPERATOR_IMAGE_AUTH"
	EnvImageNginx    = "OPERATOR_IMAGE_NGINX"
	EnvImageRegistry = "OPERATOR_IMAGE_REGISTRY"
)

// blank assignment to verify that ReconcileImageRegistry implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImageRegistry{}

// ReconcileImageRegistry reconciles a ImageRegistry object
type ReconcileImageRegistry struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client         client.Client
	scheme         *runtime.Scheme
	certManager    *certs.CertManager
	reconcileTasks []reconcileTask
	dnsZone        string
	imageAuth      string
	imageNginx     string
	imageRegistry  string
}

type reconcileTask func(*registryv1alpha1.ImageRegistry, logr.Logger) error

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileImageRegistry{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		certManager:   certs.NewCertManager(mgr.GetClient(), mgr.GetScheme(), certs.RootCASecretName()),
		dnsZone:       os.Getenv(EnvDnsZone),
		imageAuth:     os.Getenv(EnvImageAuth),
		imageNginx:    os.Getenv(EnvImageNginx),
		imageRegistry: os.Getenv(EnvImageRegistry),
	}
	if r.dnsZone == "" {
		r.dnsZone = "svc.cluster.local"
	}
	if r.imageAuth == "" {
		r.imageAuth = "mgoltzsche/image-registry-operator:latest-auth"
	}
	if r.imageNginx == "" {
		r.imageNginx = "mgoltzsche/image-registry-operator:latest-nginx"
	}
	if r.imageRegistry == "" {
		r.imageRegistry = "registry:2"
	}
	r.reconcileTasks = []reconcileTask{
		r.reconcileTokenCert,
		r.reconcileTLSCert,
		r.reconcileServiceAccount,
		r.reconcileRole,
		r.reconcileRoleBinding,
		r.reconcileService,
		r.reconcileStatefulSet,
		r.reconcilePersistentVolumeClaim,
	}
	return r
}

// Reconcile reads that state of the cluster for a ImageRegistry object and makes changes based on the state read
// and what is in the ImageRegistry.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImageRegistry) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ImageRegistry")

	// Fetch the ImageRegistry instance
	instance := &registryv1alpha1.ImageRegistry{}
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

	conditions := instance.Status.Conditions
	instance.Status.Conditions = map[status.ConditionType]status.Condition{}

	// Run reconcile tasks (may write ImageRegistry conditions)
	for _, task := range r.reconcileTasks {
		if err = task(instance, reqLogger); err != nil {
			break
		}
	}

	// Update ImageRegistry status
	syncCond := status.Condition{
		Type:   ConditionSynced,
		Status: corev1.ConditionTrue,
	}
	if err != nil {
		syncCond.Status = corev1.ConditionFalse
		syncCond.Message = err.Error()
	}
	instance.Status.Conditions.SetCondition(syncCond)
	if syncCond.Status == corev1.ConditionFalse {
		instance.Status.Conditions.SetCondition(status.Condition{
			Type:   ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: ReasonFailedSync,
		})
	}
	changedCond := false
	for _, c := range instance.Status.Conditions {
		if conditions.SetCondition(c) {
			changedCond = true
		}
	}
	instance.Status.Conditions = conditions
	hostname := r.externalHostnameForCR(instance)
	tlsSecretName := tlsSecretNameForCR(instance)
	changedGeneration := instance.Status.ObservedGeneration != instance.Generation
	changedHost := instance.Status.Hostname != hostname
	changedTLSSecretName := instance.Status.TLSSecretName != tlsSecretName
	if changedCond || changedGeneration || changedHost || changedTLSSecretName {
		instance.Status.ObservedGeneration = instance.Generation
		instance.Status.Hostname = hostname
		instance.Status.TLSSecretName = tlsSecretName
		if e := r.client.Status().Update(context.TODO(), instance); e != nil && err == nil {
			err = e
		}
	}

	return reconcile.Result{}, err
}

type namespacedObject interface {
	runtime.Object
	metav1.Object
}

func (r *ReconcileImageRegistry) upsert(owner *registryv1alpha1.ImageRegistry, obj namespacedObject, reqLogger logr.Logger, modify func() error) (err error) {
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, obj, func() (e error) {
		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(map[string]string{})
		}
		obj.SetLabels(selectorLabelsForCR(owner))
		if e = controllerutil.SetControllerReference(owner, obj, r.scheme); e != nil {
			return
		}
		return modify()
	})
	if err != nil {
		err = fmt.Errorf("upsert %s %s: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
	}
	return
}

func (r *ReconcileImageRegistry) externalUrlForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "https://" + r.externalHostnameForCR(cr)
}

func (r *ReconcileImageRegistry) externalHostnameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return fmt.Sprintf("%s.%s.%s", serviceNameForCR(cr), cr.Namespace, r.dnsZone)
}

func selectorLabelsForCR(cr *registryv1alpha1.ImageRegistry) map[string]string {
	return map[string]string{"app": "imageregistry-" + cr.Name}
}

func serviceNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return cr.Name
}

func pvcNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-pvc"
}

func tlsCertNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-tls"
}

func tlsSecretNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	name := cr.Spec.TLS.SecretName
	if name != nil {
		return *name
	}
	if cr.Spec.TLS.IssuerRef == nil {
		return "imageregistry-" + cr.Name + "-selfsigned-tls"
	}
	return "imageregistry-" + cr.Name + "-cm-tls"
}

func authCACertNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-auth-ca"
}

func authCASecretNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	name := cr.Spec.Auth.CA.SecretName
	if name != nil {
		return *name
	}
	if cr.Spec.Auth.CA.IssuerRef == nil {
		return "imageregistry-" + cr.Name + "-selfsigned-auth-ca"
	}
	return "imageregistry-" + cr.Name + "-cm-auth-ca"
}

func serviceAccountNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name
}

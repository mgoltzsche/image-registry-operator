package caroot

import (
	"time"

	"github.com/mgoltzsche/image-registry-operator/pkg/certs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log = logf.Log.WithName("controller_selfsignedca")
)

// Add creates a new selfsigned CA Secret Controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is started.
func Add(mgr manager.Manager) error {
	// Create/update root CA Secret initially
	// (new client required since manager client's cache is not initialized yet
	// and I don't see a way how to initialize the controller with a predefined reconcile.Request)
	opts := client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()}
	cl, err := client.New(mgr.GetConfig(), opts)
	if err != nil {
		return err
	}
	certMan := certs.NewCertManager(cl, mgr.GetScheme(), certs.RootCASecretName())
	if _, err := certMan.RenewRootCACertSecret(); err != nil {
		return err
	}

	r := newReconciler(mgr)

	// Create a new controller
	c, err := controller.New("caroot-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch secrets to trigger the reconcile requests
	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, &enqueueRequestForName{certs.RootCASecretName()})
}

type ReconcileCARootSecret struct {
	client      client.Client
	scheme      *runtime.Scheme
	certManager *certs.CertManager
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	certMan := certs.NewCertManager(mgr.GetClient(), mgr.GetScheme(), certs.RootCASecretName())
	return &ReconcileCARootSecret{
		client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		certManager: certMan,
	}
}

// Reconcile reads that state of the cluster for a CA root certificate Secret object
// and renews it if necessary.
func (r *ReconcileCARootSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CA root certificate")

	cert, err := r.certManager.RenewRootCACertSecret()
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: cert.NextRenewal().Sub(time.Now()) + 30*time.Second}, nil
}

type enqueueRequestForName struct {
	name types.NamespacedName
}

func (e *enqueueRequestForName) queue(meta metav1.Object, q workqueue.RateLimitingInterface) {
	if meta == nil || meta.GetName() != e.name.Name || meta.GetNamespace() != e.name.Namespace {
		return
	}
	q.Add(reconcile.Request{NamespacedName: e.name})
}

// Create implements EventHandler
func (e *enqueueRequestForName) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.queue(evt.Meta, q)
}

// Update implements EventHandler
func (e *enqueueRequestForName) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.queue(evt.MetaNew, q)
}

// Delete implements EventHandler
func (e *enqueueRequestForName) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.queue(evt.Meta, q)
}

// Generic implements EventHandler
func (e *enqueueRequestForName) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.queue(evt.Meta, q)
}

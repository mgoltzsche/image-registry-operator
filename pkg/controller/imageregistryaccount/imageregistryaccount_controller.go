package imageregistryaccount

import (
	"context"
	"time"

	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imageregistryaccount")

// Add creates a new ImageRegistryAccount Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileImageRegistryAccount{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("imageregistryaccount-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ImageRegistryAccount
	return c.Watch(&source.Kind{Type: &registryv1alpha1.ImageRegistryAccount{}}, &handler.EnqueueRequestForObject{})
}

// blank assignment to verify that ReconcileImageRegistryAccount implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImageRegistryAccount{}

// ReconcileImageRegistryAccount reconciles a ImageRegistryAccount object
type ReconcileImageRegistryAccount struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ImageRegistryAccount object and makes changes based on the state read
// and what is in the ImageRegistryAccount.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImageRegistryAccount) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ImageRegistryAccount")

	// Fetch the ImageRegistryAccount instance
	account := &registryv1alpha1.ImageRegistryAccount{}
	err := r.client.Get(context.TODO(), request.NamespacedName, account)
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

	// Delete account if expired
	if ttl := account.Spec.TTL; ttl != nil {
		if account.Expired() {
			reqLogger.Info("Deleting expired ImageRegistryAccount", "ImageRegistryAccount.Namespace", account.Namespace, "ImageRegistryAccount.Name", account.Name)
			err = r.client.Delete(context.TODO(), account)
			return reconcile.Result{}, err
		}
		expiryTime := account.CreationTimestamp.Time.Add(ttl.Duration)
		return reconcile.Result{RequeueAfter: expiryTime.Sub(time.Now()) + 10*time.Second}, nil
	}

	return reconcile.Result{}, nil
}

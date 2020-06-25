package imageregistry

import (
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/torequests"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imageregistry")

const (
	annotationImageRegistry = torequests.AnnotationToRequest("registry.mgoltzsche.github.com/imageregistry")
)

// Add creates a new ImageRegistry Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("imageregistry-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ImageRegistry
	err = c.Watch(&source.Kind{Type: &registryv1alpha1.ImageRegistry{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource StatefulSet and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Service and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Secret and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource PersistentVolumeClaim and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: annotationImageRegistry})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ServiceAccount and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Role and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &rbac.Role{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource RoleBinding and requeue the owner ImageRegistry
	err = c.Watch(&source.Kind{Type: &rbac.RoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageRegistry{},
	})
	if err != nil {
		return err
	}

	return nil
}

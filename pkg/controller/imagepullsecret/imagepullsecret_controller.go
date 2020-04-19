package imagepullsecret

import (
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/controller/imagesecret"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imagepullsecret")

// Add creates a new ImagePullSecret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return imagesecret.NewReconciler(mgr, log, imagesecret.ReconcileImageSecretConfig{
		CRFactory:       func() registryapi.ImageSecretInterface { return &registryapi.ImagePullSecret{} },
		Intent:          registryapi.TypePull,
		SecretType:      corev1.SecretTypeDockerConfigJson,
		DockerConfigKey: ".dockerconfigjson",
	})
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

	// Watch for changes to secondary resource ImageRegistryAccount and requeue the owner ImagePullSecret
	err = c.Watch(&source.Kind{Type: &registryapi.ImageRegistryAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryapi.ImagePullSecret{},
	})
	if err != nil {
		return err
	}

	return nil
}

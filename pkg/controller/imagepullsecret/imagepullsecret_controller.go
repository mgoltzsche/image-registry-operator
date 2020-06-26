package imagepullsecret

import (
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/controller/imagesecret"
	"github.com/mgoltzsche/image-registry-operator/pkg/torequests"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imagepullsecret")

const pullAccountAnnotation = torequests.AnnotationToRequest("registry.mgoltzsche.github.com/imagepullsecret")

// Add creates a new ImagePullSecret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	registryMap := torequests.NewMap()
	r := imagesecret.NewReconciler(mgr, registryMap, log, imagesecret.ReconcileImageSecretConfig{
		CRFactory:         func() registryapi.ImageSecretInterface { return &registryapi.ImagePullSecret{} },
		Intent:            registryapi.TypePull,
		SecretType:        corev1.SecretTypeDockerConfigJson,
		DockerConfigKey:   corev1.DockerConfigJsonKey,
		AccountAnnotation: pullAccountAnnotation,
	})

	c, err := controller.New("imagepullsecret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &registryapi.ImagePullSecret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return imagesecret.WatchSecondaryResources(c, &registryapi.ImagePullSecret{}, registryMap, pullAccountAnnotation)
}

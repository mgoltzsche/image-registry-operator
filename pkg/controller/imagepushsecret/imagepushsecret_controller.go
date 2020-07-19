package imagepushsecret

import (
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/backrefs"
	"github.com/mgoltzsche/image-registry-operator/pkg/controller/imagesecret"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imagepushsecret")

const pushAccountAnnotation = backrefs.AnnotationToRequest("registry.mgoltzsche.github.com/imagepushsecret")

// Add creates a new ImagePushSecret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := imagesecret.NewReconciler(mgr, log, imagesecret.ReconcileImageSecretConfig{
		CRFactory:         func() registryapi.ImageSecretInterface { return &registryapi.ImagePushSecret{} },
		Intent:            registryapi.TypePush,
		SecretType:        corev1.SecretTypeDockerConfigJson,
		DockerConfigKey:   corev1.DockerConfigJsonKey,
		AccountAnnotation: pushAccountAnnotation,
	})

	c, err := controller.New("imagepushsecret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &registryapi.ImagePushSecret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return imagesecret.WatchSecondaryResources(c, &registryapi.ImagePushSecret{}, pushAccountAnnotation)
}

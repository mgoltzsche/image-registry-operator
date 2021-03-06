package e2e

import (
	"testing"

	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
)

func testImagePullSecret(t *testing.T, ctx *framework.Context, registryRef *operator.ImageRegistryRef, hostname string) (secretName string) {
	c := ImageSecretTestCase{
		CRFactory: func() operator.ImageSecretInterface {
			cr := &operator.ImagePullSecret{}
			cr.Spec.RegistryRef = registryRef
			cr.SetName("my-pull-secret-cr")
			return cr
		},
		AccessMode:      operator.TypePull,
		SecretType:      corev1.SecretTypeDockerConfigJson,
		DockerConfigKey: corev1.DockerConfigJsonKey,
		ExpectHostname:  hostname,
	}
	testImageSecret(t, ctx, c)
	secretName = c.SecretName(c.CRFactory())
	return
}

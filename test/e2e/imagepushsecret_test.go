package e2e

import (
	"testing"

	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
)

func testImagePushSecret(t *testing.T, ctx *framework.Context, registryRef *operator.ImageRegistryRef, hostname string) (secretName string) {
	c := ImageSecretTestCase{
		CRFactory: func() operator.ImageSecretInterface {
			cr := &operator.ImagePushSecret{}
			cr.Spec.RegistryRef = registryRef
			cr.SetName("my-push-secret-cr")
			return cr
		},
		AccessMode:      operator.TypePush,
		SecretType:      corev1.SecretTypeDockerConfigJson,
		DockerConfigKey: corev1.DockerConfigJsonKey,
		ExpectHostname:  hostname,
	}
	testImageSecret(t, ctx, c)
	secretName = c.SecretName(c.CRFactory())
	return
}

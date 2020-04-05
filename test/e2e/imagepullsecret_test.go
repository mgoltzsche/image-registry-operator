package e2e

import (
	"testing"

	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testImagePushSecret(t *testing.T, ctx *framework.Context, namespace string) {
	testImageSecret(t, ctx, ImageSecretTestCase{
		Type: operator.TypePush,
		CR: &operator.ImagePushSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-push-secret-cr",
				Namespace: namespace,
			},
		},
		SecretType:      corev1.SecretTypeOpaque,
		DockerConfigKey: "config.json",
	})
}

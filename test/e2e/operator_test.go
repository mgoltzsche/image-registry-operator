package e2e

import (
	"testing"
	"time"

	//certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	"github.com/mgoltzsche/image-registry-operator/pkg/apis"
	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"github.com/stretchr/testify/require"
)

func TestOperator(t *testing.T) {
	err := framework.AddToFrameworkScheme(apis.AddToScheme, &operator.ImageRegistryList{})
	require.NoError(t, err)

	// enable to test with cert-manager:
	//err = framework.AddToFrameworkScheme(certmgr.AddToScheme, &certmgr.CertificateList{})
	//require.NoError(t, err)

	ctx := framework.NewContext(t)
	defer ctx.Cleanup()

	err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 30, RetryInterval: time.Second * 3})
	require.NoError(t, err)

	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)
	f := framework.Global
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "image-registry-operator", 1, time.Second*5, time.Second*30)
	require.NoError(t, err)

	t.Run("ImageRegistryAccount", func(t *testing.T) {
		testImageRegistryAccountAuth(t, ctx)
	})

	t.Run("ImageRegistry", func(t *testing.T) {
		registryCR := createImageRegistry(t, ctx)
		registryRef := &operator.ImageRegistryRef{
			Name:      registryCR.Name,
			Namespace: registryCR.Namespace,
		}

		t.Run("ImagePullSecret", func(t *testing.T) {
			testImagePullSecret(t, ctx, registryRef, registryCR.Status.Hostname)
		})

		t.Run("ImagePushSecret", func(t *testing.T) {
			testImagePushSecret(t, ctx, registryRef, registryCR.Status.Hostname)
		})
	})
}

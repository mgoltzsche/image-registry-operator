package e2e

import (
	"testing"
	"time"

	"github.com/mgoltzsche/image-registry-operator/pkg/apis"
	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"github.com/stretchr/testify/require"
)

func TestOperator(t *testing.T) {
	err := framework.AddToFrameworkScheme(apis.AddToScheme, &operator.ImagePullSecretList{})
	require.NoError(t, err)

	ctx := framework.NewContext(t)
	defer ctx.Cleanup()

	err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 30, RetryInterval: time.Second * 3})
	require.NoError(t, err)

	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)
	f := framework.Global
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "image-registry-operator", 1, time.Second*5, time.Second*30)
	require.NoError(t, err)

	t.Run("ImagePullSecret", func(t *testing.T) {
		testImagePullSecret(t, ctx, namespace)
	})
}

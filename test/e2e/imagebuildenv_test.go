package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/registriesconf"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func testImageBuildEnvs(t *testing.T, ctx *framework.Context, secretName string) {
	t.Run("ImageBuildEnv with redis", func(t *testing.T) {
		testImageBuildEnv(t, ctx, true, secretName)
	})
	t.Run("ImageBuildEnv without redis", func(t *testing.T) {
		testImageBuildEnv(t, ctx, false, secretName)
	})
}

func testImageBuildEnv(t *testing.T, ctx *framework.Context, redis bool, secretName string) {
	inputDockerConf, inputRegistry := loadInputSecret(t, ctx, secretName)
	f := framework.Global
	ns := f.Namespace

	cr := &operator.ImageBuildEnv{}
	cr.Name = "test-buildenv-" + strconv.FormatBool(redis)
	cr.Namespace = ns
	cr.Spec.Redis = redis
	cr.Spec.Secrets = []operator.ImageSecretRef{
		{SecretName: secretName},
	}
	err := f.Client.Create(context.TODO(), cr, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create ImageBuildEnv")
	err = WaitForCondition(t, cr, time.Second*30, func() (l []string) {
		if !cr.Status.Conditions.IsTrueFor(operator.ConditionReady) {
			ready := "ready"
			if c := cr.Status.Conditions.GetCondition(operator.ConditionReady); c != nil {
				ready += " " + string(c.Reason) + ": " + c.Message
			}
			l = []string{ready}
		}
		return
	})
	require.NoError(t, err, "wait for ImageBuildEnv")

	// Verify merged Secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: "imagebuildenv-" + cr.Name + "-conf", Namespace: cr.Namespace}
	err = f.Client.Get(context.TODO(), secretKey, secret)
	require.NoError(t, err, "get secret")
	require.NotNil(t, secret.Data, "secret data")
	dockerConfJSON := secret.Data[corev1.DockerConfigJsonKey]
	makisuYAML := secret.Data[operator.SecretKeyMakisuYAML]
	registry := secret.Data[operator.SecretKeyRegistry]
	redisHost := secret.Data[operator.SecretKeyRedis]
	redisPassword := secret.Data[operator.SecretKeyRedisPassword]
	require.NotNil(t, registry, "secret key %q", operator.SecretKeyRegistry)
	require.NotNil(t, dockerConfJSON, "secret key %q", corev1.DockerConfigJsonKey)
	require.NotNil(t, makisuYAML, "secret key %q", operator.SecretKeyMakisuYAML)
	if redis {
		require.NotNil(t, redisHost, "secret key %q", operator.SecretKeyRedis)
		require.NotNil(t, redisPassword, "secret key %q", operator.SecretKeyRedisPassword)
	} else {
		require.Nil(t, redisHost, "secret key %q", operator.SecretKeyRedis)
		require.Nil(t, redisPassword, "secret key %q", operator.SecretKeyRedisPassword)
	}
	dockerConf, err := registriesconf.ParseDockerConfig(dockerConfJSON)
	require.NoError(t, err, "parse docker config")
	require.NotNil(t, dockerConf, "parsed docker config")
	require.Equal(t, inputRegistry, string(registry), "secret key %q", operator.SecretKeyRegistry)
	require.Equal(t, inputDockerConf, dockerConf, "docker conf")

}

func loadInputSecret(t *testing.T, ctx *framework.Context, secretName string) (dockerConf *registriesconf.DockerConfig, registry string) {
	ns := framework.Global.Namespace
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: secretName, Namespace: ns}
	err := framework.Global.Client.Get(context.TODO(), secretKey, secret)
	require.NoError(t, err, "load input secret")
	require.NotNil(t, secret.Data, "input secret data")
	registry = string(secret.Data[operator.SecretKeyRegistry])
	dockerConfJSON := secret.Data[corev1.DockerConfigJsonKey]
	require.NotNil(t, dockerConfJSON, "input secret key %q", corev1.DockerConfigJsonKey)
	dockerConf, err = registriesconf.ParseDockerConfig(dockerConfJSON)
	require.NoError(t, err, "parse input docker config")
	require.NotNil(t, dockerConf, "parsed input docker config")
	return
}

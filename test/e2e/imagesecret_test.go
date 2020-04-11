package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ImageSecretTestCase struct {
	Type            operator.ImageSecretType
	CR              operator.ImageSecret
	SecretType      corev1.SecretType
	DockerConfigKey string
}

func testImageSecret(t *testing.T, ctx *framework.Context, c ImageSecretTestCase) {
	f := framework.Global

	// Test secret creation
	secretCR := c.CR
	namespace := secretCR.GetNamespace()
	err := f.Client.Create(context.TODO(), secretCR, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create CR")

	//waitForSecretUpdateAndAssert(t, c, 1)

	usr, pw := waitForSecretUpdateAndAssert(t, c, 1)
	t.Run("authenticator", func(t *testing.T) {
		testAuthenticator(t, ctx, namespace, secretCR.GetName(), c.Type, usr, pw)
	})

	t.Run("credential rotation", func(t *testing.T) {
		for i := 2; i < 4; i++ {
			triggerCredentialRotation(t, secretCR)
			waitForSecretUpdateAndAssert(t, c, uint64(i))
		}
	})
}

func triggerCredentialRotation(t *testing.T, secretCR operator.ImageSecret) {
	t.Logf("triggering %T %s password rotation...", secretCR, secretCR.GetName())
	secretCR.GetStatus().RotationDate = metav1.Time{time.Now().Add(-24 * 30 * time.Hour)}
	err := framework.Global.Client.Status().Update(context.TODO(), secretCR)
	require.NoError(t, err, "trigger %T %s password rotation", secretCR, secretCR.GetName())
}

func waitForSecretUpdateAndAssert(t *testing.T, c ImageSecretTestCase, rotationCount uint64) (usr, pw string) {
	secretCR := c.CR
	ns := secretCR.GetNamespace()
	secretName := fmt.Sprintf("%s-image-%s-secret", secretCR.GetName(), c.Type)
	secretKey := dynclient.ObjectKey{Name: secretName, Namespace: ns}
	status := secretCR.GetStatus()
	err := WaitForCondition(t, secretCR, secretCR.GetName(), ns, 15*time.Second, func() (c []string) {
		if status.Rotation != rotationCount {
			c = append(c, fmt.Sprintf("$.status.rotation == %d (was %v)", rotationCount, status.Rotation))
		}
		if status.RotationDate.Time.Unix() == 0 {
			c = append(c, "$.status.rotationDate should be set")
		}
		if len(status.Passwords) == 0 {
			c = append(c, "len($.status.passwords) > 0")
		}
		if !status.Conditions.IsTrueFor("ready") {
			c = append(c, "ready")
		}
		return
	})
	require.NoError(t, err, "wait for %T to update (rotation %d)", secretCR, rotationCount)
	require.True(t, len(status.Passwords) <= 2, "CR should have len(status.passwords) <= 2")
	secret := &corev1.Secret{}
	err = framework.Global.Client.Get(context.TODO(), secretKey, secret)
	require.NoError(t, err, "secret should exist")
	require.Equal(t, c.SecretType, secret.Type, "resulting secret's type")
	usr, pw = dockercfgSecretPassword(t, secret, c.DockerConfigKey, "https://myregistry")
	for _, hashedPw := range status.Passwords {
		err = bcrypt.CompareHashAndPassword([]byte(hashedPw), []byte(pw))
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "none of the %d bcrypted %T passwords matches the one within the Secret (rotation %d) - CR/Secret sync issue?", len(status.Passwords), secretCR, rotationCount)
	expectedUser := fmt.Sprintf("%s/%s/%s/%d", ns, secretCR.GetName(), c.Type, rotationCount)
	require.Equal(t, expectedUser, usr, "username")
	t.Logf("secret %s's password matches one within %T %s", secret.Name, secretCR, secretCR.GetName())
	return
}

func dockercfgSecretPassword(t *testing.T, secret *corev1.Secret, cfgKey, registryUrl string) (usr, pw string) {
	dockerConfigJson := secret.Data[cfgKey]
	dockerConfig := map[string]interface{}{}
	err := json.Unmarshal(dockerConfigJson, &dockerConfig)
	require.NoError(t, err, "json unmarshal secret.data[%q]: %s", cfgKey, string(dockerConfigJson))
	dcAuths, ok := dockerConfig["auths"].(map[string]interface{})
	require.True(t, ok && len(dcAuths) > 0, "generated dockerconfig.json does not specify auths: %v", dockerConfig["auths"])
	for registry, obj := range dcAuths {
		require.Equal(t, registryUrl, string(registry), "dockerconfig: registry")
		m, ok := obj.(map[string]interface{})
		require.True(t, ok, "dockerconfig.json auths entry is not an object but %v", obj)
		auth, ok := m["auth"].(string)
		require.True(t, ok, "dockerconfig.json auths entry's auth property is not a string but %v", m["auth"])
		b, err := base64.StdEncoding.DecodeString(string(auth))
		require.NoError(t, err, "base64 decode auth value")
		s := string(b)
		colonPos := strings.Index(s, ":")
		usr = s[0:colonPos]
		pw = s[colonPos+1:]
	}
	return
}

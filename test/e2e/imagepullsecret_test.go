package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	operator "github.com/mgoltzsche/credential-manager/pkg/apis/credentialmanager/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func testImagePullSecret(t *testing.T, ctx *framework.Context, namespace string) {
	f := framework.Global

	// Test secret creation
	secretCR := &operator.ImagePullSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pull-secret-cr",
			Namespace: namespace,
		},
	}
	err := f.Client.Create(context.TODO(), secretCR, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create CR")

	key := dynclient.ObjectKey{Name: secretCR.Name + "-image-pull-secret", Namespace: namespace}
	waitForSecretUpdateAndAssert(t, secretCR, key, 1)

	/*usr, pw := waitForSecretUpdateAndAssert(t, secretCR, key, 1)
	t.Run("authservice", func(t *testing.T) {
		testAuthService(t, ctx, namespace, secretCR.Name, usr, pw)
	})*/

	t.Run("credential rotation", func(t *testing.T) {
		for i := 2; i < 4; i++ {
			triggerCredentialRotation(t, secretCR)
			waitForSecretUpdateAndAssert(t, secretCR, key, uint64(i))
		}
	})
}

func triggerCredentialRotation(t *testing.T, secretCR *operator.ImagePullSecret) {
	t.Logf("triggering CR %s password rotation...", secretCR.Name)
	secretCR.Status.RotationDate = metav1.Time{time.Now().Add(-24 * 30 * time.Hour)}
	err := framework.Global.Client.Status().Update(context.TODO(), secretCR)
	require.NoError(t, err, "trigger CR %s password rotation", secretCR.Name)
}

func waitForSecretUpdateAndAssert(t *testing.T, secretCR *operator.ImagePullSecret, secretKey dynclient.ObjectKey, rotationCount uint64) (usr, pw string) {
	err := WaitForCondition(t, secretCR, secretCR.Name, secretCR.Namespace, 15*time.Second, func() (c []string) {
		if secretCR.Status.Rotation != rotationCount {
			c = append(c, fmt.Sprintf("$.status.rotation == %d (was %v)", rotationCount, secretCR.Status.Rotation))
		}
		if secretCR.Status.RotationDate.Time.Unix() == 0 {
			c = append(c, "$.status.rotationDate should be set")
		}
		if len(secretCR.Status.Passwords) == 0 {
			c = append(c, "len($.status.passwords) > 0")
		}
		if !secretCR.Status.Conditions.IsTrueFor("ready") {
			c = append(c, "ready")
		}
		return
	})
	require.True(t, len(secretCR.Status.Passwords) <= 2, "CR should have len(status.passwords) <= 2")
	require.NoError(t, err, "wait for ImagePullSecret CR to update (rotation %d)", rotationCount)
	secret := &corev1.Secret{}
	err = framework.Global.Client.Get(context.TODO(), secretKey, secret)
	require.NoError(t, err, "pull secret should exist")
	require.Equal(t, "kubernetes.io/dockerconfigjson", string(secret.Type), "resulting secret's type")
	usr, pw = dockercfgSecretPassword(t, secret, "https://myregistry")
	for _, hashedPw := range secretCR.Status.Passwords {
		err = bcrypt.CompareHashAndPassword([]byte(hashedPw), []byte(pw))
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "none of the %d bcrypted pull secret CR passwords matches the one within the secret (rotation %d) - CR/secret sync issue?", len(secretCR.Status.Passwords), rotationCount)
	expectedUser := fmt.Sprintf("%s/%s/%d", secretCR.Namespace, secretCR.Name, rotationCount)
	require.Equal(t, expectedUser, usr, "username")
	t.Logf("secret %s's password matches one within CR %s", secret.Name, secretCR.Name)
	return
}

func dockercfgSecretPassword(t *testing.T, secret *corev1.Secret, registryUrl string) (usr, pw string) {
	dockerConfigJson := secret.Data[".dockerconfigjson"]
	dockerConfig := map[string]interface{}{}
	err := json.Unmarshal(dockerConfigJson, &dockerConfig)
	require.NoError(t, err, "json unmarshal secret.data['.dockerconfigjson']: %s", string(dockerConfigJson))
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

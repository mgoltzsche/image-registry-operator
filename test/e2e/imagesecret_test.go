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
	"k8s.io/apimachinery/pkg/types"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ImageSecretTestCase struct {
	CR              operator.ImageSecretInterface
	AccessMode      operator.ImageSecretType
	SecretType      corev1.SecretType
	DockerConfigKey string
	ExpectHostname  string
}

func (c *ImageSecretTestCase) SecretName() string {
	return fmt.Sprintf("%s-image-%s-secret", c.CR.GetName(), c.AccessMode)
}

func testImageSecret(t *testing.T, ctx *framework.Context, c ImageSecretTestCase) {
	f := framework.Global
	registryNamespace := c.CR.GetRegistryRef().Namespace

	// Test secret creation
	secretCR := c.CR
	secretCR.SetNamespace(f.Namespace)
	err := f.Client.Create(context.TODO(), secretCR, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create CR")
	acc, usr, pw := waitForSecretUpdateAndAssert(t, c)
	require.Equal(t, int64(1), c.CR.GetStatus().Rotation, "rotation after first account created")
	expectedLabels := map[string][]string{
		"origin":  []string{"cr"},
		"account": []string{acc.Name},
	}
	for k, v := range acc.Spec.Labels {
		expectedLabels[k] = v
	}

	t.Run("authn CLI", func(t *testing.T) {
		testAuthentication(t, registryNamespace, usr, pw, expectedLabels, runAuthnCLI)
	})
	t.Run("authn plugin", func(t *testing.T) {
		testAuthentication(t, registryNamespace, usr, pw, expectedLabels, runAuthnPlugin)
	})

	t.Run("credential rotation", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			triggerCredentialRotation(t, c)
			waitForSecretUpdateAndAssert(t, c)
		}
		t.Run("plugin authn after rotation", func(t *testing.T) {
			testAuthentication(t, registryNamespace, usr, pw, expectedLabels, runAuthnPlugin)
		})
	})
}

func triggerCredentialRotation(t *testing.T, c ImageSecretTestCase) {
	t.Log("triggering account rotation...")
	client := framework.Global.Client
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: c.SecretName(), Namespace: c.CR.GetNamespace()}
	err := client.Get(context.TODO(), key, secret)
	require.NoError(t, err, "get secret for %s to trigger account rotation", c.CR.GetName())
	secret.Annotations = map[string]string{"someannotation": "someval"}
	err = client.Update(context.TODO(), secret)
	require.NoError(t, err, "update secret to trigger account rotation")
}

func waitForSecretUpdateAndAssert(t *testing.T, c ImageSecretTestCase) (account *operator.ImageRegistryAccount, usr, pw string) {
	secretCR := c.CR
	ns := secretCR.GetNamespace()
	secretKey := dynclient.ObjectKey{Name: c.SecretName(), Namespace: ns}
	status := secretCR.GetStatus()
	rotation := status.Rotation
	err := WaitForCondition(t, secretCR, secretCR.GetName(), ns, 10*time.Second, func() (c []string) {
		if !status.Conditions.IsTrueFor("Ready") {
			cond := status.Conditions.GetCondition("Ready")
			if cond == nil {
				c = append(c, "Ready")
			} else {
				c = append(c, fmt.Sprintf("Ready{%s: %s}", cond.Reason, cond.Message))
			}
		}
		if status.Rotation <= rotation {
			c = append(c, "rotationIncrement")
		}
		return
	})
	require.NoError(t, err, "wait for %T to update (rotation %d)", secretCR, rotation+1)
	require.True(t, rotation < status.Rotation && status.Rotation < rotation+4, "rotation: %d < r < %d, r = %d", rotation, rotation+4, status.Rotation)
	require.NotNil(t, status.RotationDate, "rotation date")

	// Verify generated account
	account = &operator.ImageRegistryAccount{}
	accName := fmt.Sprintf("%s.%s.%s.%d", c.AccessMode, c.CR.GetNamespace(), c.CR.GetName(), status.Rotation)
	accKey := types.NamespacedName{Name: accName, Namespace: c.CR.GetNamespace()}
	err = framework.Global.Client.Get(context.TODO(), accKey, account)
	require.NoError(t, err, "account should exist")
	require.True(t, account.Spec.Password != "", "account password set")
	require.True(t, account.Spec.TTL != nil, "account TTL set")
	require.Equal(t, 24*time.Hour, account.Spec.TTL.Duration, "account ttl")
	expectedLabels := map[string][]string{
		"name":       []string{secretCR.GetName()},
		"namespace":  []string{secretCR.GetNamespace()},
		"accessMode": []string{string(secretCR.GetRegistryAccessMode())},
	}
	require.Equal(t, expectedLabels, account.Spec.Labels, "account labels")

	// Verify generated Secret
	secret := &corev1.Secret{}
	err = framework.Global.Client.Get(context.TODO(), secretKey, secret)
	require.NoError(t, err, "secret should exist")
	require.Equal(t, c.SecretType, secret.Type, "resulting secret's type")
	require.True(t, len(secret.Data["ca.crt"]) > 0, "resulting secret should have ca.crt entry")
	require.Equal(t, c.ExpectHostname, string(secret.Data["hostname"]), "resulting secret's hostname entry")
	usr, pw = dockercfgSecretPassword(t, secret, c.DockerConfigKey, c.ExpectHostname)
	err = bcrypt.CompareHashAndPassword([]byte(account.Spec.Password), []byte(pw))
	require.NoError(t, err, "bcrypted password should match - CR/Secret sync issue?")
	require.Equal(t, accName, usr, "username")
	t.Logf("secret %s's password matches the one in account %s", secret.Name, accName)

	// Ensure that rotation does not happen when nothing changes
	rotation = status.Rotation
	secret.Annotations["someannotation"] = "someval"
	err = framework.Global.Client.Update(context.TODO(), secret)
	require.NoError(t, err, "secret update without changes")
	time.Sleep(2 * time.Second)
	key := types.NamespacedName{Name: secretCR.GetName(), Namespace: secretCR.GetNamespace()}
	err = framework.Global.Client.Get(context.TODO(), key, secretCR)
	require.NoError(t, err, "secret CR should exist")
	require.Equal(t, rotation, status.Rotation, "status.rotation: secret should not be rotated if nothing changed")
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

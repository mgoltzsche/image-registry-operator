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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type ImageSecretTestCase struct {
	CRFactory       func() operator.ImageSecretInterface
	AccessMode      operator.ImageSecretType
	SecretType      corev1.SecretType
	DockerConfigKey string
	ExpectHostname  string
}

func (c *ImageSecretTestCase) SecretName(cr operator.ImageSecretInterface) string {
	return fmt.Sprintf("image%ssecret-%s", c.AccessMode, cr.GetName())
}

func testImageSecret(t *testing.T, ctx *framework.Context, c ImageSecretTestCase) {
	ns := framework.Global.Namespace
	registryNamespace := ns

	// Test secret creation
	secretCR := c.CRFactory()
	secretCR.SetNamespace(ns)
	client := framework.Global.Client
	err := client.Create(context.TODO(), secretCR, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create CR")
	acc, usr, pw := waitForSecretUpdateAndAssert(t, c, secretCR)
	require.Equal(t, int64(1), secretCR.GetStatus().Rotation, "rotation after first account created")
	expectedLabels := map[string][]string{
		"origin":  []string{"cr"},
		"account": []string{acc.Name},
	}
	for k, v := range acc.Spec.Labels {
		expectedLabels[k] = v
	}

	t.Run("authn CLI", func(t *testing.T) {
		testAuthentication(t, ctx, registryNamespace, usr, pw, expectedLabels, runAuthnCLI)
	})
	t.Run("authn plugin", func(t *testing.T) {
		testAuthentication(t, ctx, registryNamespace, usr, pw, expectedLabels, runAuthnPlugin)
	})

	t.Run("credential rotation", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			triggerCredentialRotation(t, secretCR, c.SecretName(secretCR))
			waitForSecretUpdateAndAssert(t, c, secretCR)
		}
		t.Run("plugin authn after rotation", func(t *testing.T) {
			testAuthentication(t, ctx, registryNamespace, usr, pw, expectedLabels, runAuthnPlugin)
		})
	})

	t.Run("delete account when secret deleted", func(t *testing.T) {
		secretCR := c.CRFactory()
		secretCR.SetName(secretCR.GetName() + "-delete")
		secretCR.SetNamespace(ns)
		client := framework.Global.Client
		err := client.Create(context.TODO(), secretCR, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
		require.NoError(t, err, "create CR")
		acc, _, _ := waitForSecretUpdateAndAssert(t, c, secretCR)
		err = client.Delete(context.TODO(), secretCR)
		require.NoError(t, err, "delete CR")
		err = WaitForCondition(t, secretCR, 10*time.Second, func() []string {
			return []string{"deletion"}
		})
		require.Error(t, err, "should be deleted")
		require.True(t, errors.IsNotFound(err), "should be not found error after deletion but was %s", err)
		accKey := types.NamespacedName{Name: acc.Name, Namespace: acc.Namespace}
		_ = client.Get(context.TODO(), accKey, acc)
		err = client.Get(context.TODO(), accKey, acc)
		require.Error(t, err, "account should not exist anymore after CR has been deleted")
		require.True(t, errors.IsNotFound(err), "account should not exist after deletion but client.Get() returned error %s", err)
	})
}

func triggerCredentialRotation(t *testing.T, cr operator.ImageSecretInterface, secretName string) {
	t.Log("triggering account rotation...")
	client := framework.Global.Client
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: secretName, Namespace: cr.GetNamespace()}
	err := client.Get(context.TODO(), key, secret)
	require.NoError(t, err, "get secret for %s to trigger account rotation", cr.GetName())
	secret.Annotations = map[string]string{"someannotation": "someval"}
	err = client.Update(context.TODO(), secret)
	require.NoError(t, err, "update secret to trigger account rotation")
}

func waitForSecretUpdateAndAssert(t *testing.T, c ImageSecretTestCase, secretCR operator.ImageSecretInterface) (account *operator.ImageRegistryAccount, usr, pw string) {
	ns := secretCR.GetNamespace()
	status := secretCR.GetStatus()
	rotation := status.Rotation
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: c.SecretName(secretCR), Namespace: ns}
	account = &operator.ImageRegistryAccount{}
	accKey := types.NamespacedName{Namespace: secretCR.GetNamespace()}
	err := WaitForCondition(t, secretCR, 10*time.Second, func() (pending []string) {
		if !status.Conditions.IsTrueFor("Ready") {
			cond := status.Conditions.GetCondition("Ready")
			if cond == nil {
				pending = append(pending, "Ready")
			} else {
				pending = append(pending, fmt.Sprintf("Ready{%s: %s}", cond.Reason, cond.Message))
			}
			return
		}
		if status.Rotation <= rotation {
			pending = append(pending, "rotationIncrement")
			return
		}

		// Fetch account
		accKey.Name = fmt.Sprintf("%s.%s.%s.%d", c.AccessMode, secretCR.GetNamespace(), secretCR.GetName(), status.Rotation)
		if e := framework.Global.Client.Get(context.TODO(), accKey, account); e != nil {
			pending = append(pending, "account: "+e.Error())
			return
		}
		// Fetch & verify secret
		e := framework.Global.Client.Get(context.TODO(), secretKey, secret)
		require.NoError(t, e, "secret lookup when CR ready")
		require.Equal(t, c.SecretType, secret.Type, "resulting secret's type")
		require.True(t, len(secret.Data["ca.crt"]) > 0, "resulting secret should have ca.crt entry")
		require.Equal(t, c.ExpectHostname, string(secret.Data["registry"]), "resulting secret's registry entry")
		usr, pw = dockercfgSecretPassword(t, secret, c.DockerConfigKey, c.ExpectHostname)
		if accKey.Name != usr {
			pending = append(pending, fmt.Sprintf("secretloginname{%s -> %d}", usr, status.Rotation))
		}
		require.Equal(t, accKey.Name, usr, "username")
		return
	})
	require.NoError(t, err, "wait for %T to update (rotation %d)", secretCR, rotation+1)
	require.True(t, rotation < status.Rotation && status.Rotation < rotation+4, "rotation: %d < r < %d, r = %d", rotation, rotation+4, status.Rotation)
	require.NotNil(t, status.RotationDate, "rotation date")

	// Verify account
	require.True(t, account.Spec.Password != "", "account password set")
	require.True(t, account.Spec.TTL != nil, "account TTL set")
	require.Equal(t, 24*time.Hour, account.Spec.TTL.Duration, "account ttl")
	expectedLabels := map[string][]string{
		"name":       []string{secretCR.GetName()},
		"namespace":  []string{secretCR.GetNamespace()},
		"accessMode": []string{string(secretCR.GetRegistryAccessMode())},
	}
	require.Equal(t, expectedLabels, account.Spec.Labels, "account labels")

	// Verify password matches
	err = bcrypt.CompareHashAndPassword([]byte(account.Spec.Password), []byte(pw))
	require.NoError(t, err, "bcrypted password should match - CR/Secret sync issue?")
	t.Logf("secret %s's password matches the one in account %s", secret.Name, accKey.Name)

	// Ensure that rotation does not happen when nothing changes
	rotation = status.Rotation
	secret.Annotations["someannotation"] = "someval"
	err = framework.Global.Client.Update(context.TODO(), secret)
	require.NoError(t, err, "secret update without changes")
	time.Sleep(3 * time.Second)
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

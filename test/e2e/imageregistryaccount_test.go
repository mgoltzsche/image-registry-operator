package e2e

import (
	"context"
	"testing"
	"time"

	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func testImageRegistryAccountAuth(t *testing.T, ctx *framework.Context) {
	f := framework.Global
	user := "push.user.ns"
	pw := "fake-password"
	pwHash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	require.NoError(t, err, "bcrypt")

	// Insert ImageRegistryAccount CR
	labels := map[string][]string{
		"namespace": []string{"ns"},
		"somelabel": []string{"somevalue"},
	}
	cr := &registryapi.ImageRegistryAccount{}
	cr.Name = user
	cr.Namespace = f.Namespace
	cr.Spec.Password = string(pwHash)
	cr.Spec.Labels = labels
	err = f.Client.Create(context.TODO(), cr, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create ImageRegistryAccount")
	labels["origin"] = []string{"cr"}
	labels["account"] = []string{user}

	t.Run("authn CLI", func(t *testing.T) {
		testAuthentication(t, cr.Namespace, user, pw, labels, runAuthnCLI)
	})
	t.Run("authn plugin", func(t *testing.T) {
		testAuthentication(t, cr.Namespace, user, pw, labels, runAuthnPlugin)
	})
}

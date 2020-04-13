package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"runtime"
	"sync"
	"testing"

	"github.com/cesanta/docker_auth/auth_server/api"
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

var authnPluginBuildMutex = &sync.Mutex{}

func testAuthnPlugin(t *testing.T, ctx *framework.Context, namespace, name string, intent registryapi.ImageSecretType, user, pw string) {
	t.Run("missing authn", func(t *testing.T) {
		runAuthnPlugin(t, "", "", false)
	})
	t.Run("invalid authn", func(t *testing.T) {
		runAuthnPlugin(t, "unknown", "invalid", false)
	})
	t.Run("valid authn", func(t *testing.T) {
		labels := runAuthnPlugin(t, user, pw, true)
		require.Equal(t, []string{"cr"}, labels["type"], "authn result type")
		require.Equal(t, []string{name}, labels["name"], "authn result name")
		require.Equal(t, []string{namespace}, labels["namespace"], "authn result namespace")
		require.Equal(t, []string{string(intent)}, labels["intent"], "authn result intent")
	})
}

func runAuthnPlugin(t *testing.T, usr, pw string, succeed bool) map[string][]string {
	pluginBin := buildAuthnPlugin(t)

	origKubeconfigEnv := os.Getenv(k8sutil.KubeConfigEnvVar)
	os.Setenv(k8sutil.KubeConfigEnvVar, clientcmd.NewDefaultClientConfigLoadingRules().Precedence[0])
	defer os.Setenv(k8sutil.KubeConfigEnvVar, origKubeconfigEnv)

	plug, err := plugin.Open(pluginBin)
	require.NoError(t, err)
	symAuthn, err := plug.Lookup("Authn")
	require.NoError(t, err)
	authn := symAuthn.(api.Authenticator)

	authenticated, labels, err := authn.Authenticate(usr, api.PasswordString(pw))
	require.NoError(t, err, "plugin.Authenticate()")
	require.Equal(t, succeed, authenticated)
	return labels
}

func buildAuthnPlugin(t *testing.T) string {
	_, testFileName, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(testFileName, "..", "..", "..")
	outputBinName := filepath.Join(projectRoot, "build", "_output", "bin", "authn-plugin.so")

	if _, err := os.Stat(outputBinName); err != nil {
		authnPluginBuildMutex.Lock()
		defer authnPluginBuildMutex.Unlock()
		// Build binary
		goBinName, err := exec.LookPath("go")
		require.NoError(t, err, "find go binary")
		outBuf := &bytes.Buffer{}
		goBuildCmd := exec.Command(goBinName, "build", "-o", outputBinName, "-buildmode=plugin")
		goBuildCmd.Dir = filepath.Join(projectRoot, "docker-authn-plugin")
		goBuildCmd.Stdout = outBuf
		goBuildCmd.Stderr = outBuf
		err = goBuildCmd.Run()
		require.NoError(t, err, "build authn plugin, log:\n%s", outBuf.String())
	}

	return outputBinName
}

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
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

var authnPluginBuildMutex = &sync.Mutex{}

func runAuthnPlugin(t *testing.T, namespace, usr, pw string, succeed bool) map[string][]string {
	pluginBin := buildAuthnPlugin(t)

	origKubeconfigEnv := os.Getenv(k8sutil.KubeConfigEnvVar)
	os.Setenv(k8sutil.KubeConfigEnvVar, clientcmd.NewDefaultClientConfigLoadingRules().Precedence[0])
	os.Setenv("NAMESPACE", namespace)
	defer os.Setenv(k8sutil.KubeConfigEnvVar, origKubeconfigEnv)

	plug, err := plugin.Open(pluginBin)
	require.NoError(t, err)
	symAuthn, err := plug.Lookup("Authn")
	require.NoError(t, err)
	authn := symAuthn.(api.Authenticator)

	authenticated, labels, err := authn.Authenticate(usr, api.PasswordString(pw))
	require.NoError(t, err, "plugin.Authenticate()")
	// authenticate everybody to handle anonymous users using ACL
	require.True(t, authenticated, "authenticated")
	if succeed {
		require.True(t, len(labels) > 0, "should return labels")
	} else {
		require.Equal(t, 0, len(labels), "should not return labels")
	}
	return map[string][]string(labels)
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

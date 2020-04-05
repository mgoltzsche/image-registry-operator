package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/auth"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

var authnBuildMutex = &sync.Mutex{}

func testAuthenticator(t *testing.T, ctx *framework.Context, namespace, name string, intent registryapi.ImageSecretType, user, pw string) {
	t.Run("missing authn", func(t *testing.T) {
		authn(t, "", "", false)
	})
	t.Run("invalid authn", func(t *testing.T) {
		authn(t, "unknown", "invalid", false)
	})
	t.Run("valid authn", func(t *testing.T) {
		b := authn(t, user, pw, true)
		payload := auth.Authenticated{}
		err := json.Unmarshal(b, &payload)
		require.NoError(t, err, "unmarshal authn result: %s", string(b))
		require.Equal(t, "cr", payload.Type, "authn result type")
		require.Equal(t, name, payload.Name, "authn result name")
		require.Equal(t, namespace, payload.Namespace, "authn result namespace")
		require.Equal(t, intent, payload.Intent, "authn result intent")
	})
}

func authn(t *testing.T, usr, pw string, succeed bool) (b []byte) {
	cmd := authnClientCmd(t, "-u", usr)
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBE_REGISTRY_PASSWORD=%s", pw))
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if succeed {
		require.NoError(t, err, "should authenticate %q, stdout: %s", usr, errBuf.String())
	} else {
		require.Error(t, err, "should fail to authenticate %q, stdout: %s", usr, errBuf.String())
	}
	return buf.Bytes()
}

func authnClientCmd(t *testing.T, args ...string) *exec.Cmd {
	_, testFileName, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(testFileName, "..", "..", "..")
	outputBinName := filepath.Join(projectRoot, "build", "_output", "bin", "authn-client")

	if _, err := os.Stat(outputBinName); err != nil {
		authnBuildMutex.Lock()
		defer authnBuildMutex.Unlock()
		// Build binary
		goBinName, err := exec.LookPath("go")
		require.NoError(t, err, "find go binary")
		outBuf := &bytes.Buffer{}
		goBuildCmd := exec.Command(goBinName, "build", "-o", outputBinName)
		goBuildCmd.Dir = filepath.Join(projectRoot, "cmd", "authn-client")
		goBuildCmd.Stdout = outBuf
		goBuildCmd.Stderr = outBuf
		err = goBuildCmd.Run()
		require.NoError(t, err, "build authservice binary, log:\n%s", outBuf.String())
	}

	// Build command
	localCmd := exec.Command(outputBinName, args...)
	// we can hardcode index 0 as that is the highest priority kubeconfig to be loaded and will always
	// be populated by NewDefaultClientConfigLoadingRules()
	localCmd.Env = append(os.Environ(), fmt.Sprintf("%v=%v", k8sutil.KubeConfigEnvVar,
		clientcmd.NewDefaultClientConfigLoadingRules().Precedence[0]))
	return localCmd
}

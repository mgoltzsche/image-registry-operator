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

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

var authnBuildMutex = &sync.Mutex{}

func runAuthnCLI(t *testing.T, ns, usr, pw string, succeed bool) (labels map[string][]string) {
	cmd := authnClientCmd(t, "-n", ns, "-u", usr)
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBE_REGISTRY_PASSWORD=%s", pw))
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if succeed {
		require.NoError(t, err, "authn CLI stderr: %s", errBuf.String())
	} else {
		require.Error(t, err, "authn CLI stdout: %s", buf.String())
	}
	if err == nil {
		labels = map[string][]string{}
		err = json.Unmarshal(buf.Bytes(), &labels)
		require.NoError(t, err, "unmarshal authn CLI stdout: %s", buf.String())
	}
	return
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

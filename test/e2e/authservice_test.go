package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mgoltzsche/image-registry-operator/pkg/auth"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
)

const authSvcHost = "127.0.0.1:5999"

func testAuthService(t *testing.T, ctx *framework.Context, namespace, name, user, pw string) {
	authSvcArgs := []string{"-l", authSvcHost}
	runAuthService(t, authSvcArgs, func(t *testing.T) {
		t.Run("missing auth", func(t *testing.T) {
			requestAuth(t, "", "", 401)
		})
		t.Run("invalid auth", func(t *testing.T) {
			requestAuth(t, "unknown", "invalid", 401)
		})
		t.Run("invalid auth", func(t *testing.T) {
			b := requestAuth(t, user, pw, 200)
			payload := auth.Authenticated{}
			err := json.Unmarshal(b, &payload)
			require.NoError(t, err, "unmarshal auth request body")
			require.Equal(t, name, payload.Name, "auth response name")
			require.Equal(t, namespace, payload.Namespace, "auth response namespace")
		})
	})
	t.Logf("authentication successful")
}

func requestAuth(t *testing.T, usr, pw string, expectStatus int) []byte {
	url := fmt.Sprintf("http://%s/auth", authSvcHost)
	client := &http.Client{}
	ctx, _ := context.WithTimeout(context.TODO(), 3*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err, "creating auth request")
	if usr != "" {
		req.SetBasicAuth(usr, pw)
	}
	resp, err := client.Do(req)
	require.NoError(t, err, "auth request")
	require.Equal(t, expectStatus, resp.StatusCode, "auth request status")
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "auth request body")
	return b
}

func runAuthService(t *testing.T, args []string, tests func(*testing.T)) {
	outBuf := &bytes.Buffer{}
	localCmd := setupAuthServiceCommand(t, args)
	localCmd.Stdout = outBuf
	localCmd.Stderr = outBuf

	err := localCmd.Start()
	require.NoError(t, err, "failed to run authservice")
	t.Logf("Started authservice")

	defer func() {
		if err = localCmd.Process.Kill(); err != nil {
			t.Logf("failed to stop authservice process: %s", err)
		}
		t.Logf("\n------ authservice output ------\n%s", outBuf.String())
	}()

	err = wait.PollImmediate(time.Second, 5*time.Second, func() (bool, error) {
		_, err := http.Get(fmt.Sprintf("http://%s/health", authSvcHost))
		return err == nil, nil
	})
	require.NoError(t, err, "authservice healthcheck")

	tests(t)
}

func setupAuthServiceCommand(t *testing.T, args []string) *exec.Cmd {
	_, testFileName, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(testFileName, "..", "..", "..")
	outputBinName := filepath.Join(projectRoot, "build", "_output", "bin", "authservice-local")

	// Build service binary
	goBinName, err := exec.LookPath("go")
	require.NoError(t, err, "find go binary")
	outBuf := &bytes.Buffer{}
	goBuildCmd := exec.Command(goBinName, "build", "-o", outputBinName)
	goBuildCmd.Dir = filepath.Join(projectRoot, "cmd", "authservice")
	goBuildCmd.Stdout = outBuf
	goBuildCmd.Stderr = outBuf
	err = goBuildCmd.Run()
	require.NoError(t, err, "build authservice binary, log:\n%s", outBuf.String())

	// Build command
	localCmd := exec.Command(outputBinName, args...)
	// we can hardcode index 0 as that is the highest priority kubeconfig to be loaded and will always
	// be populated by NewDefaultClientConfigLoadingRules()
	localCmd.Env = append(os.Environ(), fmt.Sprintf("%v=%v", k8sutil.KubeConfigEnvVar,
		clientcmd.NewDefaultClientConfigLoadingRules().Precedence[0]))
	return localCmd
}

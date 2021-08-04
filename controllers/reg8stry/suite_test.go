/*
Copyright 2021 Max Goltzsche.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reg8stry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	registryapi "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
	"github.com/mgoltzsche/reg8stry/internal/certs"
	"github.com/mgoltzsche/reg8stry/internal/status"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg                        *rest.Config
	k8sClient                  client.Client
	testEnv                    *envtest.Environment
	testeeCancelFunc           context.CancelFunc
	testNamespace              string
	testNamespace2             string
	testNamespaceResource      = &corev1.Namespace{}
	testNamespace2Resource     = &corev1.Namespace{}
	testDefaultRegistryRef     registryapi.ImageRegistryRef
	testCARootSecretName       types.NamespacedName
	testDNSZone                = "test.example.org"
	testSecretRotationInterval = 24 * time.Hour
	testAccountTTL             = 48 * time.Hour
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithCancel(context.Background())
	testeeCancelFunc = cancel
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	err := downloadKubebuilderAssetsIfNotExist(filepath.Join("..", "..", "build", "kubebuilder"))
	Expect(err).ShouldNot(HaveOccurred(), "Failed to download kubebuilder assets")

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = registryapi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Set up test Namespaces
	Eventually(func() error {
		testNamespace = fmt.Sprintf("test-namespace-%d", rand.Int63())
		testNamespaceResource.Name = testNamespace
		return k8sClient.Create(context.TODO(), testNamespaceResource)
	}, "10s", "1s").ShouldNot(HaveOccurred())
	Eventually(func() error {
		testNamespace2 = fmt.Sprintf("test-namespace2-%d", rand.Int63())
		testNamespace2Resource.Name = testNamespace2
		return k8sClient.Create(context.TODO(), testNamespace2Resource)
	}, "10s", "1s").ShouldNot(HaveOccurred())

	// Set up test defaults
	testDefaultRegistryRef = registryapi.ImageRegistryRef{Name: "default-test-registry", Namespace: testNamespace}
	testCARootSecretName = types.NamespacedName{Name: "test-root-ca-cert", Namespace: testNamespace}

	// Set up controller manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		LeaderElection:     false,
		MetricsBindAddress: "127.0.0.1:0",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	// Set up controllers

	certManager := certs.NewCertManager(k8sClient, mgr.GetScheme(), testCARootSecretName)
	err = (&CARootCertificateSecretReconciler{
		CARootSecretName: testCARootSecretName,
		CertManager:      certManager,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	err = (&ImageRegistryReconciler{
		CertManager:   certManager,
		DNSZone:       testDNSZone,
		ImageAuth:     "fake-auth:0.0.0",
		ImageNginx:    "fake-nginx:0.0.0",
		ImageRegistry: "fake-registry:2",
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	err = (&ImageRegistryAccountReconciler{}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	secretCfg := ImageSecretConfig{
		DefaultRegistry:               testDefaultRegistryRef,
		RotationInterval:              testSecretRotationInterval,
		RequeueDelayOnMissingRegistry: time.Second,
		AccountTTL:                    testAccountTTL,
		DNSZone:                       testDNSZone,
	}
	err = NewImagePullSecretReconciler(secretCfg).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())
	err = NewImagePushSecretReconciler(secretCfg).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	testeeCancelFunc()
	if k8sClient != nil {
		k8sClient.Delete(context.TODO(), testNamespaceResource)
		k8sClient.Delete(context.TODO(), testNamespace2Resource)
	}
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func downloadKubebuilderAssetsIfNotExist(destDir string) error {
	if os.Getenv("KUBEBUILDER_ASSETS") != "" {
		fmt.Println("Skipping kubebuilder assets download since KUBEBUILDER_ASSETS env var is specified")
		return nil
	}
	destDir, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	kubebuilderVersion := "2.3.1"
	destDir = fmt.Sprintf("%s-%s", destDir, kubebuilderVersion)
	kubebuilderSubDir := fmt.Sprintf("kubebuilder_%s_%s_%s", kubebuilderVersion, goruntime.GOOS, goruntime.GOARCH)
	kubebuilderBinDir := filepath.Join(destDir, kubebuilderSubDir, "bin")
	err = os.Setenv("KUBEBUILDER_ASSETS", kubebuilderBinDir)
	if err != nil {
		return err
	}
	if _, err = os.Stat(kubebuilderBinDir); err == nil {
		fmt.Println("Using kubebuilder assets at", kubebuilderBinDir)
		return nil // already downloaded
	}
	fmt.Println("Downloading kubebuilder assets to", destDir)
	kubebuilderTarGzURL := fmt.Sprintf("https://go.kubebuilder.io/dl/%s/%s/%s", kubebuilderVersion, goruntime.GOOS, goruntime.GOARCH)
	resp, err := http.Get(kubebuilderTarGzURL) // #nosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	destParentDir := filepath.Dir(destDir)
	err = os.MkdirAll(destParentDir, 0750)
	if err != nil {
		return err
	}
	tmpDir, err := ioutil.TempDir(destParentDir, ".tmp-kubebuilder-assets-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	tarStream, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(tarStream)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		destFile := filepath.Join(tmpDir, header.Name) // #nosec
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.Mkdir(destFile, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			err = os.MkdirAll(filepath.Dir(destFile), 0755)
			if err != nil {
				return err
			}
			f, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(f, tarReader) // #nosec
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("extract kubebuilder tar: entry %s has unknown type %d", header.Name, header.Typeflag)
		}
	}
	err = os.RemoveAll(destDir)
	if err != nil {
		return err
	}
	return os.Rename(tmpDir, destDir)
}

func verifyState(o client.Object, pollTimeout time.Duration, condition func() error) error {
	lastErrMsg := ""
	err := wait.PollImmediate(100*time.Millisecond, pollTimeout, func() (bool, error) {
		key := client.ObjectKeyFromObject(o)
		if err := k8sClient.Get(context.TODO(), key, o); err != nil {
			lastErrMsg = err.Error()
			return false, nil
		}
		if err := condition(); err != nil {
			lastErrMsg = fmt.Sprintf("%s did not meet condition: %s", key, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil && lastErrMsg != "" {
		return fmt.Errorf("%s: %s", err, lastErrMsg)
	}
	return err
}

func verifyDeleted(o client.Object, pollTimeout time.Duration) error {
	key := client.ObjectKeyFromObject(o)
	err := wait.PollImmediate(100*time.Millisecond, pollTimeout, func() (bool, error) {
		e := k8sClient.Get(context.TODO(), key, o)
		if apierrors.IsNotFound(e) {
			return true, nil
		}
		return false, e
	})
	if err != nil {
		return fmt.Errorf("%T %s was not deleted: %s", o, key, err)
	}
	return nil
}

func verifyCondition(o client.Object, conditions *[]metav1.Condition, condition string) error {
	return verifyConditionTimeout(o, conditions, condition, 10*time.Second)
}

func verifyConditionTimeout(o client.Object, conditions *[]metav1.Condition, condition string, pollTimeout time.Duration) error {
	key := client.ObjectKeyFromObject(o)
	fmt.Fprintf(GinkgoWriter, "waiting up to %v for %s condition %s...\n", pollTimeout, key, condition)
	err := verifyState(o, pollTimeout, func() error {
		c := status.GetCondition(*conditions, condition)
		if c == nil {
			return fmt.Errorf("condition %s is not present", condition)
		}
		if c.ObservedGeneration != o.GetGeneration() {
			return fmt.Errorf("condition %s observedGeneration is not up-to-date", condition)
		}
		if c.Status != metav1.ConditionTrue {
			return fmt.Errorf("condition %s status is %s: %s: %s", condition, c.Status, c.Reason, c.Message)
		}
		fmt.Fprintf(GinkgoWriter, "%s met condition %s\n", client.ObjectKeyFromObject(o), condition)
		return nil
	})
	if err != nil {
		return fmt.Errorf("waiting for the condition %s: %s", condition, err.Error())
	}
	return nil
}

func verifyExists(o client.Object, key types.NamespacedName) error {
	return k8sClient.Get(context.TODO(), key, o)
}

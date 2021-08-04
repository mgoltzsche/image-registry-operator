/*
Copyright 2021.

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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	registryv1alpha1 "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
	registrycontrollers "github.com/mgoltzsche/reg8stry/controllers/reg8stry"
	"github.com/mgoltzsche/reg8stry/internal/certs"
	"github.com/mgoltzsche/reg8stry/internal/flagext"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(registryv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr             = ":8080"
		enableLeaderElection    = false
		probeAddr               = ":8081"
		managerNamespace        string
		imageAuth               = "mgoltzsche/image-registry-operator:latest-auth"
		imageNginx              = "mgoltzsche/image-registry-operator:latest-nginx"
		imageRegistry           = "registry:2"
		dnsZone                 = "svc.cluster.local"
		defaultRegistryRef      = registryv1alpha1.ImageRegistryRef{Name: "registry"}
		accountTTL              = 24 * time.Hour
		secretRotationInterval  = 18 * time.Hour
		secretReconcileInterval = 30 * time.Second
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", metricsAddr, "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", probeAddr, "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", enableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&managerNamespace, "manager-namespace", ":8081", "The Kubernetes namespace the manager runs in.")
	flag.StringVar(&imageAuth, "image-auth", imageAuth, "The auth server container image that should be used.")
	flag.StringVar(&imageNginx, "image-nginx", imageNginx, "The nginx container image that should be used.")
	flag.StringVar(&imageRegistry, "image-registry", imageRegistry, "The docker registry container image that should be used.")
	flag.StringVar(&dnsZone, "dns-zone", dnsZone, "The DNS zone registries should be exposed under.")
	flag.StringVar(&defaultRegistryRef.Name, "default-registry-name", defaultRegistryRef.Name, "The name of the default ImageRegistry that should be used by Image*Secret resources.")
	flag.StringVar(&defaultRegistryRef.Namespace, "default-registry-namespace", defaultRegistryRef.Namespace, "The namespace of the default ImageRegistry that should be used by Image*Secret resources.")
	flag.DurationVar(&accountTTL, "account-ttl", accountTTL, "The time to live for ImageRegistryAccount resources that are created for Image*Secret resources.")
	flag.DurationVar(&secretReconcileInterval, "secret-reconcile-interval", secretReconcileInterval, "The interval in which the controller reconciles Image*Secret resources that refer a non-existing ImageRegistry.")
	flag.DurationVar(&secretRotationInterval, "secret-rotation-interval", secretRotationInterval, "The interval in which Image*Secrets are rotated - should be lower than --account-ttl.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	err := flagext.ParseFlagsAndEnvironment(flag.CommandLine, "REG8STRY_")
	if err != nil {
		flag.CommandLine.Usage()
		setupLog.Error(err, "invalid usage")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if managerNamespace == "" {
		setupLog.Error(fmt.Errorf("--manager-namespace not specified"), "invalid usage")
		os.Exit(1)
	}
	if defaultRegistryRef.Namespace == "" {
		defaultRegistryRef.Namespace = managerNamespace
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "reg8stry-6593d20b.mgoltzsche.github.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	caRootSecretName := types.NamespacedName{Name: "image-registry-root-ca", Namespace: managerNamespace}
	certManager := certs.NewCertManager(mgr.GetClient(), mgr.GetScheme(), caRootSecretName)
	err = (&registrycontrollers.CARootCertificateSecretReconciler{
		CARootSecretName: caRootSecretName,
		CertManager:      certManager,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create ca root secret reconciler")
		os.Exit(1)
	}
	if err = (&registrycontrollers.ImageRegistryReconciler{
		CertManager:   certManager,
		DNSZone:       dnsZone,
		ImageAuth:     imageAuth,
		ImageNginx:    imageNginx,
		ImageRegistry: imageRegistry,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImageRegistry")
		os.Exit(1)
	}
	secretConfig := registrycontrollers.ImageSecretConfig{
		DefaultRegistry:               defaultRegistryRef,
		DNSZone:                       dnsZone,
		AccountTTL:                    accountTTL,
		RotationInterval:              secretRotationInterval,
		RequeueDelayOnMissingRegistry: secretReconcileInterval,
	}
	if err = registrycontrollers.NewImagePullSecretReconciler(secretConfig).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImagePullSecret")
		os.Exit(1)
	}
	if err = registrycontrollers.NewImagePushSecretReconciler(secretConfig).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImagePushSecret")
		os.Exit(1)
	}
	if err = (&registrycontrollers.ImageRegistryAccountReconciler{}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImageRegistryAccount")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

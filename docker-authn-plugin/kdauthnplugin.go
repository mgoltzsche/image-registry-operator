package main

import (
	"io/ioutil"
	"os"

	"github.com/cesanta/docker_auth/auth_server/api"

	"github.com/cesanta/glog"
	"github.com/mgoltzsche/image-registry-operator/pkg/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const pluginName = "k8s-authn"

var (
	// Export cesanta/docker_auth plugin
	Authn                   = newK8sDockerAuthnPlugin()
	_     api.Authenticator = &Authn
)

type k8sDockerAuthnPlugin struct {
	auth *auth.Authenticator
}

// Authenticate authenticates a request against Kubernetes image registry operator resources.
func (p *k8sDockerAuthnPlugin) Authenticate(user string, password api.PasswordString) (bool, api.Labels, error) {
	labels, err := p.auth.Authenticate(user, string(password))
	return true, labels, err
}

// Stop finalizes resources in preparation for shutdown.
func (p *k8sDockerAuthnPlugin) Stop() {}

// Name of the docker auth plugin
func (p *k8sDockerAuthnPlugin) Name() string {
	return pluginName
}

func newK8sDockerAuthnPlugin() k8sDockerAuthnPlugin {
	var cfg *rest.Config
	var err error
	errLogger := func(err error) { glog.Error(err) }

	// Init kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		glog.Infof("starting %s plugin using in-cluster kubeconfig", pluginName)
		cfg, err = rest.InClusterConfig()
		if err != nil {
			glog.Error(err.Error() + ". Alternatively KUBECONFIG can be defined")
			os.Exit(2)
		}
	} else {
		glog.Infof("starting %s plugin using kubeconfig %q", pluginName, kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			glog.Error(err)
			os.Exit(3)
		}
	}

	// Find authenticator namespace
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		b, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			panic("Cannot detect namespace: " + err.Error())
		}
		namespace = string(b)
	}

	cfg.UserAgent = "Image Registry Auth"
	a, err := auth.NewAuthenticator(cfg, namespace, errLogger)
	if err != nil {
		glog.Error(err)
		os.Exit(4)
	}
	return k8sDockerAuthnPlugin{a}
}

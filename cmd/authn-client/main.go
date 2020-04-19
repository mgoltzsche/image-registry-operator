package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mgoltzsche/image-registry-operator/pkg/auth"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

// A CLI to authenticate users against kube registry resources
func main() {
	log.SetFlags(0)
	pwEnvVarName := "KUBE_REGISTRY_PASSWORD"
	kubeconfig := os.Getenv("KUBECONFIG")
	namespace := os.Getenv("NAMESPACE")
	user := ""
	pflag.StringVarP(&kubeconfig, "kubeconfig", "c", kubeconfig, "path to kubeconfig (env var KUBECONFIG)")
	pflag.StringVarP(&namespace, "namespace", "n", namespace, "namespace with ImageRegistryAccounts to authenticate against (env var NAMESPACE)")
	pflag.StringVarP(&user, "user", "u", user, "user name (set password via env var "+pwEnvVarName+")")
	pflag.Parse()
	if !pflag.Parsed() || len(pflag.Args()) != 0 {
		pflag.Usage()
		os.Exit(1)
	}
	pw, pwOk := os.LookupEnv(pwEnvVarName)
	if !pwOk {
		pflag.Usage()
		fmt.Fprintf(os.Stderr, "env var %s not set\n", pwEnvVarName)
		os.Exit(1)
	}
	a, err := authenticator(kubeconfig, namespace).Authenticate(user, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "authn error: %s\n", err)
		os.Exit(2)
	}
	if a == nil {
		fmt.Fprintln(os.Stderr, "invalid credentials provided")
		os.Exit(3)
	}

	b, err := json.Marshal(a)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	os.Exit(0)
}

func authenticator(kubeconfig, namespace string) *auth.Authenticator {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
	cfg.UserAgent = "Image Registry authn CLI"
	errLogger := func(err error) { log.Println(err) }
	a, err := auth.NewAuthenticator(cfg, namespace, errLogger)
	if err != nil {
		log.Fatal(err)
	}
	return a
}

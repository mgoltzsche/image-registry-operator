package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/mgoltzsche/reg8stry/internal/auth"
	"github.com/mgoltzsche/reg8stry/internal/flagext"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// A CLI to authenticate users against ImageRegistryAccount resources
func main() {
	var kubeconfig, namespace, user, password string
	log.SetFlags(0)
	flag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "The path to kubeconfig")
	flag.StringVar(&namespace, "namespace", namespace, "The namespace containing ImageRegistryAccounts to authenticate against")
	flag.StringVar(&user, "user", user, "The user name")
	flag.StringVar(&password, "password", user, "The password")
	err := flagext.ParseFlagsAndEnvironment(flag.CommandLine, "REG8STRYAUTH_")
	if err != nil {
		flag.CommandLine.Usage()
		fmt.Fprintf(os.Stderr, "invalid usage: %s", err.Error())
		os.Exit(1)
	}
	a, err := authenticator(kubeconfig, namespace).Authenticate(user, password)
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

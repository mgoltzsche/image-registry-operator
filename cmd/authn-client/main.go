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
	user := ""
	pflag.StringVarP(&kubeconfig, "kubeconfig", "c", kubeconfig, "path to kubeconfig (env var KUBECONFIG)")
	pflag.StringVarP(&user, "user", "u", user, "user name (set password via env var "+pwEnvVarName+")")
	pflag.Parse()
	if !pflag.Parsed() || len(pflag.Args()) != 0 {
		pflag.Usage()
		os.Exit(1)
	}
	password := os.Getenv(pwEnvVarName)
	if user == "" || password == "" {
		pflag.Usage()
		fmt.Fprintf(os.Stderr, "flag -u or env var %s not set\n", pwEnvVarName)
		os.Exit(1)
	}
	if a := authenticator(kubeconfig).Authenticate(user, password); a != nil {
		b, err := json.Marshal(a)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
		os.Exit(0)
	} else {
		fmt.Fprintln(os.Stderr, "invalid credentials provided")
		os.Exit(2)
	}
}

func authenticator(kubeconfig string) *auth.Authenticator {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
	cfg.UserAgent = "Image Registry authn CLI"
	errLogger := func(err error) { log.Println(err) }
	a, err := auth.NewAuthenticator(cfg, errLogger)
	if err != nil {
		log.Fatal(err)
	}
	return a
}

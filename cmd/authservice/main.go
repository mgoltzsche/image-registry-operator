package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/mgoltzsche/credential-manager/pkg/auth"
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	address    = "127.0.0.1:9090"
	kubeconfig = os.Getenv("KUBECONFIG")
)

func main() {
	log.SetFlags(0)
	pflag.StringVarP(&address, "listen", "l", address, "address the auth service is listening on")
	pflag.StringVarP(&address, "kubeconfig", "c", kubeconfig, "path to kubeconfig (env var KUBECONFIG)")
	pflag.Parse()
	if !pflag.Parsed() {
		pflag.Usage()
		os.Exit(1)
	}
	var cfg *rest.Config
	var err error
	if kubeconfig == "" {
		log.Printf("using in-cluster kubeconfig")
		cfg, err = rest.InClusterConfig()
	} else {
		log.Printf("using kubeconfig %q", kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		log.Fatal(err)
	}
	cfg.UserAgent = "Image Registry Auth"
	a, err := auth.NewAuthenticator(cfg, func(err error) { log.Println("authenticator:", err) })
	if err != nil {
		log.Fatal(err)
	}
	svc := &authService{a}
	http.HandleFunc("/auth", svc.AuthenticateRequest)
	http.HandleFunc("/health", handleHealthCheck)
	log.Println("listening on", address)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal(err)
	}
}

type authService struct {
	auth *auth.Authenticator
}

func (s *authService) AuthenticateRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	if usr, pwd, ok := r.BasicAuth(); ok {
		if authenticated := s.auth.Authenticate(usr, pwd); authenticated != nil {
			b, err := json.Marshal(authenticated)
			if err == nil {
				w.WriteHeader(200)
				w.Write(b)
				return
			} else {
				log.Printf("cannot marshal authenticated: %s\n", err)
			}
		}
	}
	w.WriteHeader(401)
	w.Write([]byte("{}"))
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

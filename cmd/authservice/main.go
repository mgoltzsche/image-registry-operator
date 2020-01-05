package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/mgoltzsche/credential-manager/pkg/auth"
	"github.com/spf13/pflag"
)

var (
	address = "127.0.0.1:9090"
)

func main() {
	log.SetFlags(0)
	pflag.StringVarP(&address, "listen", "l", address, "address the auth service is listening on")
	pflag.Parse()
	if !pflag.Parsed() {
		pflag.Usage()
		os.Exit(1)
	}
	a, err := auth.NewAuthenticator(func(err error) { log.Fatal(err) })
	if err != nil {
		log.Fatal(err)
		os.Exit(2)
	}
	svc := &authService{a}
	http.HandleFunc("/auth", svc.AuthenticateRequest)
	log.Println("Listening on", address)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal(err)
		os.Exit(2)
	}
}

type authService struct {
	auth *auth.Authenticator
}

func (s *authService) AuthenticateRequest(w http.ResponseWriter, r *http.Request) {
	if usr, pwd, ok := r.BasicAuth(); ok {
		if authenticated := s.auth.Authenticate(usr, pwd); authenticated != nil {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(200)
			b, err := json.Marshal(authenticated)
			if err != nil {
				log.Fatal("cannot marshal authenticated", err)
				b = []byte("{}")
			}
			w.Write(b)
			return
		}
	}
	w.WriteHeader(403)
}

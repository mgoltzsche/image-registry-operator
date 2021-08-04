package registriesconf

import (
	"encoding/base64"
	"strings"

	"gopkg.in/yaml.v2"
)

type MakisuRegistries map[string]MakisuRepos
type MakisuRepos map[string]MakisuRepo

// Config contains Docker registry client configuration.
// See https://github.com/uber/makisu/blob/master/docs/REGISTRY.md
type MakisuRepo struct {
	Security MakisuSecurity `yaml:"security"`
}

type MakisuSecurity struct {
	TLS       *MakisuTLS      `yaml:"tls,omitempty"`
	BasicAuth MakisuBasicAuth `yaml:"basic"`
}

type MakisuTLS struct {
	CA MakisuCert `yaml:"cert"`
}

type MakisuCert struct {
	Path string `yaml:"path"`
}

type MakisuBasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func ToMakisuBasicAuth(auth string) (u MakisuBasicAuth, err error) {
	d, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return
	}
	s := string(d)
	l := strings.SplitN(s, ":", 2)
	u.Username = l[0]
	if len(l) > 1 {
		u.Password = l[1]
	}
	return
}

func (r MakisuRegistries) AddRegistry(registry, image string, auth MakisuBasicAuth) {
	e := r[registry]
	if e == nil {
		e = map[string]MakisuRepo{}
		r[registry] = e
	}
	e[image] = MakisuRepo{Security: MakisuSecurity{BasicAuth: auth}}
}

func (r MakisuRegistries) YAML() []byte {
	y, err := yaml.Marshal(r)
	if err != nil {
		panic(err)
	}
	return y
}

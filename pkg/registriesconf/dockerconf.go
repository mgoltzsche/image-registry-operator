package registriesconf

import (
	"encoding/base64"
	"encoding/json"
)

func ParseDockerConfig(data []byte) (conf *DockerConfig, err error) {
	conf = &DockerConfig{}
	err = json.Unmarshal(data, conf)
	return
}

type DockerConfig struct {
	// Auths maps docker registries to credentials
	Auths map[string]DockerConfigUrlAuth `json:"auths"`
}

type DockerConfigUrlAuth struct {
	// Auth base64 encoded basic auth credentials
	Auth string `json:"auth"`
}

// AddAuth adds a registry auth entry
func (c *DockerConfig) AddAuth(registry, user, passwd string) *DockerConfig {
	if c.Auths == nil {
		c.Auths = map[string]DockerConfigUrlAuth{}
	}
	c.Auths[registry] = DockerConfigUrlAuth{
		Auth: base64.StdEncoding.EncodeToString([]byte(user + ":" + passwd)),
	}
	return c
}

// JSON marshals to config.json format
func (c *DockerConfig) JSON() []byte {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return b
}

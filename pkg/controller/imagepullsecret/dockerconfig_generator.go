package imagepullsecret

import (
	"encoding/base64"
	"encoding/json"
)

func generateDockerConfigJson(url, user, passwd string) []byte {
	conf := dockerConfig{map[string]dockerConfigUrlAuth{
		url: dockerConfigUrlAuth{"Basic " + base64.RawStdEncoding.EncodeToString([]byte(user+":"+passwd))},
	}}
	b, err := json.Marshal(conf)
	if err != nil {
		panic(err)
	}
	return b
}

type dockerConfig struct {
	Auths map[string]dockerConfigUrlAuth `json:"auths"`
}

type dockerConfigUrlAuth struct {
	Auth string `json:"auth"`
}

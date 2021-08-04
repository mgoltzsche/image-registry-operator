module github.com/mgoltzsche/reg8stry

go 1.16

require (
	github.com/cesanta/docker_auth/auth_server v0.0.0-20210307020609-4c6b4012296a
	github.com/cesanta/glog v0.0.0-20150527111657-22eb27a0ae19
	github.com/go-logr/logr v0.4.0
	github.com/jetstack/cert-manager v1.3.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.17.0
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	golang.org/x/oauth2 v0.0.0-20210220000619-9bb904979d93 // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	sigs.k8s.io/controller-runtime v0.9.0
)

// Pinned to cesanta/docker_auth 1.7.0.
// For plugin compatibility all dependencies (also transitive!) that appear within docker_auth as well must be pinned to the same version here.
// If the version does not match docker_auth terminates with an error when attempting to load the plugin:
//   "error while loading authn plugin: [...]: plugin was built with a different version of package [...]"
// This can be resolved by listing docker_auth's dependencies (go list -m all) and pin the corresponding package version here.
replace (
	github.com/cesanta/glog => github.com/cesanta/glog v0.0.0-20150527111657-22eb27a0ae19
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.3
	golang.org/x/net => golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/sys => golang.org/x/sys v0.0.0-20210305230114-8fe3ee5dd75b
	golang.org/x/text => golang.org/x/text v0.3.5
	google.golang.org/protobuf => google.golang.org/protobuf v1.25.0
)

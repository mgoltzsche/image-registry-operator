image-registry-operator
===

A controller that watches `ImagePullSecret` CRs and generates concrete secrets for them.
The CR usage allows to declare docker registry authentication or delegate it using RBAC rules.  

**TODO:**
* Rename go package to correspond to the new repository name.
* Support CRD `ImageRegistry`.
* Rename `ImagePullSecret` CRD to `ImageRegistryAccess` or add additional `ImagePushSecret` CRD.
  (push secret should have `config` property to avoid using it as pullSecret and allow to easily mount it in image build jobs)
* Provide optional PodPreset(s) to implicitly mount `default-image-push-secret` into e.g. every skaffold-kaniko pod.

# How it works
For each `ImagePullSecret` CR a dockerconfig secret is maintained.
The corresponding registry URL is configured as operator deployment environment variable.
The username is derived from the CR following the scheme `<namespace>/<name>/<status.rotation>`.
The password is rotated using the secret's `nextpassword` field to allow asynchronous credential
sync by publishing its bcrypt hash with the CR's `passwords` status field one rotation before its
plain text version is rotated into the `.dockerconfigjson` key within the generated `Secret`
to be actually used by clients.
Correspondingly the active two bcrypt password hashes are maintained within the CR's status field `passwords`.
Other applications may use them to authenticate users.  

For authentication against the CRs an image with [cesanta/docker_auth](https://github.com/cesanta/docker_auth)
and the `docker-authn-plugin` within this repository is built.
In order to use the plugin containers should be configured correspondingly (`auth_config.yml`):
```
plugin_authn:
  plugin_path: /docker_auth/k8s-docker-authn.so
```

Authorization can be specified using [docker_auth's ACL](https://github.com/cesanta/docker_auth/blob/master/docs/Labels.md).


# How to build
Build the operator and [docker_auth](https://github.com/cesanta/docker_auth) images (requires make and docker/podman):
```
make operator docker_auth
```


# How to test
Run unit tests:
```
make unit-tests
```
Run e2e tests (requires a kubernetes cluster and its KUBECONFIG env var populated):
```
make e2e-tests
```


# Development notes

The operator skeleton has been generated using the [operator-sdk](https://github.com/operator-framework):
* The `deploy` directory contains the corresponding kubernetes manifests.
* The `deploy/crds` directory is generated from `pkg/apis/registry/v1alpha1/*_types.go`.
* The `pkg/controller/*` directories contain the code that handles the corresponding CRD.

The CRD files in `deploy/crd` need to be regenerated as follows when an API type changes:
```
make generate
```
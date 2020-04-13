image-registry-operator
===

A Kubernetes operator that maintains in-cluster docker registries as well as
corresponding pull and push secrets.
For granular authorization [docker_auth](https://github.com/cesanta/docker_auth) is integrated.

# Kubernetes cluster requirements

* LoadBalancer support
* CoreDNS' static IP (`10.96.0.10`) must be configured as first nameserver on every node (avoid DNS loops!) (to resolve registry on nodes).
* CoreDNS should be configured with the `k8s_external` plugin exposing LoadBalancer Services under your public DNS zone (`OPERATOR_DNS_ZONE`).

# Installation

Install the operator (you need to specify `OPERATOR_DNS_ZONE` env var with your public DNS zone):
```
kubectl apply -k ./deploy
```

# Usage example

Create a (the default) `ImageRegistry` (maintains a StatefulSet and LoadBalancer Service):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imageregistry_cr.yaml
```

Create an `ImagePushSecret` (maintains Secret `<CR_NAME>-image-push-secret`):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imagepushsecret_cr.yaml
```

Create an `ImagePullSecret` (maintains Secret `<CR_NAME>-image-pull-secret`):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imagepullsecret_cr.yaml
```

Configure your local host to use the previously created `ImagePushSecret`'s Docker config:
```
kubectl get -n build secret example-imagepushsecret-image-push-secret -o jsonpath='{.data.config\.json}' | base64 -d > ~/.docker/config.json
```

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
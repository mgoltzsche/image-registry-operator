image-registry-operator
===

A Kubernetes operator that maintains in-cluster docker registries, accounts
as well as push and pull secrets.
Granular authorization is supported by [cesanta/docker_auth](https://github.com/cesanta/docker_auth).  


# Custom Resource Definitions

This operator supports the following CRDs:
* `ImageRegistry` represents a [docker image registry](https://docs.docker.com/registry/) and [docker_auth](https://github.com/cesanta/docker_auth) service.
* `ImageRegistryAccount` represents an account to access the registry. A registry only authenticates accounts contained in its namespace.
* `ImagePushSecret` represents an `ImageRegistryAccount` in the referenced registry's namespace and an `Opaque` `Secret` with a docker config at key `config.json`.
* `ImagePullSecret` represents an `ImageRegistryAccount` in the referenced registry's namespace and a `kubernetes.io/dockerconfigjson` `Secret`.

By default managed push and pull secrets are rotated every 24h.  

Both push and pull secrets contain additional keys:
* `hostname` - the registry's hostname _(to be used to define registry agnostic builds)_
* `ca.crt` - the registry's CA certificate _(to support test installations using a self-signed CA)_

A `Ready` condition is maintained by the operator for `ImageRegistry`, `ImagePushSecret` and `ImagePullSecret` resources
reflecting its current status and the cause in case of an error.


# Kubernetes cluster requirements

* LoadBalancer support
* CoreDNS' static IP (`10.96.0.10`) must be configured as first nameserver on every node (avoid DNS loops!) to resolve registry on nodes.
* optional (for public access): CoreDNS should be configured with the `k8s_external` plugin exposing LoadBalancer Services under your public DNS zone (`OPERATOR_DNS_ZONE`).
* optional: [cert-manager](https://cert-manager.io/) should be installed.


# DNS

An `ImageRegistry`'s hostname looks as follows: `<NAME>.<NAMESPACE>.<OPERATOR_DNS_ZONE>`.  

Name resolution inside your k8s cluster and on its nodes can be done using the `k8s_external` CoreDNS plugin (see `./deploy/coredns-configmap.yaml`)
For DNS resolution outside your cluster (if needed) [external-dns](https://github.com/kubernetes-sigs/external-dns)
could be configured.


# TLS

By default, if neither an issuer nor a secret name are specified, the operator maintains self-signed certificates for an `ImageRegistry`'s TLS and token CA.
However an `ImageRegistry` can optionally refer to an existing secret or a [cert-manager](https://cert-manager.io/)
`Issuer` which the operator will then use to create a `Certificate`.

_Please note that, in case of a self-signed registry TLS CA, the CA certificate must be registered with the container runtime._
_For development purposes [nodehack](https://github.com/mgoltzsche/nodehack) can help with that._


# Authorization

Authorization can be specified per `ImageRegistry` using [docker_auth's ACL](https://github.com/cesanta/docker_auth/blob/master/docs/Labels.md).


# Installation

Install the operator (you may want to specify `OPERATOR_DNS_ZONE` env var with your public DNS zone, the default is `svc.example.org`):
```
kubectl apply -k ./deploy
```


# Usage example

Create a (the default) `ImageRegistry` (maintains a StatefulSet and LoadBalancer Service):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imageregistry_cr.yaml
```

Create an `ImagePushSecret` (maintains an `ImageRegistryAccount` and a Secret `<CR_NAME>-image-push-secret`):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imagepushsecret_cr.yaml
```

Create an `ImagePullSecret` (maintains an `ImageRegistryAccount` and a Secret `<CR_NAME>-image-pull-secret`):
```
kubectl apply -f ./deploy/crds/registry.mgoltzsche.github.com_v1alpha1_imagepullsecret_cr.yaml
```

Configure your local host to use the previously created `ImagePushSecret`'s Docker config:
```
kubectl get -n build secret example-imagepushsecret-image-push-secret -o jsonpath='{.data.config\.json}' | base64 -d > ~/.docker/config.json
```


# How to build
Build the operator as well as preconfigured [docker_auth](https://github.com/cesanta/docker_auth) and [nginx](https://www.nginx.com/) images (requires make and docker/podman):
```
make operator docker_auth nginx
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
* The `deploy/crds` directory is generated from `pkg/apis/registry/*/*_types.go`.
* The `pkg/controller/*` directories contain the code that handles the corresponding CRD.

The CRDs in `deploy/crd` and `zz_*.go` files need to be regenerated as follows when an API type changes:
```
make generate
```

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
* `registry` - the registry's hostname _(to be used to define registry agnostic builds)_
* `ca.crt` - the registry's CA certificate _(to support test installations using a self-signed CA)_
* `username` - the registry's username
* `password` - the registry's password

A `Ready` condition is maintained by the operator for `ImageRegistry`, `ImagePushSecret` and `ImagePullSecret` resources
reflecting its current status and the cause in case of an error.


# Kubernetes cluster requirements

* LoadBalancer support
* LoadBalancer `Service` names must resolve on the nodes - see DNS section below.
* optional: [cert-manager](https://cert-manager.io/) should be installed if a self-signed TLS certificate is not sufficient.


# DNS

An `ImageRegistry`'s hostname looks as follows: `<NAME>.<NAMESPACE>.<OPERATOR_DNS_ZONE>`.  

The `OPERATOR_DNS_ZONE` is an environment variable that can be specified on the operator and defaults to `svc.cluster.local`.  

Registry name resolution inside your k8s cluster and on its nodes can be done using CoreDNS:
CoreDNS' static IP (`10.96.0.10`) should be configured as first nameserver on every node (avoid DNS loops!).
_For development purposes this can be done using [nodehack](https://github.com/mgoltzsche/nodehack) as `./deploy/minikube` shows._  

For DNS propagation outside your cluster [external-dns](https://github.com/kubernetes-sigs/external-dns) can be used - registry `Service` resources are already annotated correspondingly by the operator.
Additionally CoreDNS' [k8s_external](https://coredns.io/plugins/k8s_external/) plugin can be used to resolve public (registry) names inside the cluster (see `./deploy/coredns-public-zone`) making it independent from external DNS configuration.  


# TLS

The operator maintains a self-signed CA certificate secret `image-registry-root-ca` in its own namespace.  

By default, if neither an issuer nor a secret name are specified, the operator uses it to sign the generated TLS certificate for an `ImageRegistry`.
Alternatively an `ImageRegistry` can refer to an existing secret or a [cert-manager](https://cert-manager.io/)
`Issuer` which the operator will then use to create a `Certificate`.  

_Please note that, in case of a self-signed registry TLS CA, the CA certificate must be registered with the container runtime._
_For development purposes this can be done using [nodehack](https://github.com/mgoltzsche/nodehack) as `./deploy/minikube` shows._


# Authorization

Authorization can be specified per `ImageRegistry` using [docker_auth's ACL](https://github.com/cesanta/docker_auth/blob/master/docs/Labels.md).


# Operator installation

There are multiple operator deployment variants.
In all variants listed here the `OPERATOR_DNS_ZONE` env var defaults to `svc.cluster.local`.

## Install for single namespace

Install the operator namespace-scoped in the default namespace:
```
kubectl apply -k github.com/mgoltzsche/image-registry-operator/deploy/crds
kubectl apply -k github.com/mgoltzsche/image-registry-operator/deploy/operator
```

## Install cluster-wide

Install the operator in the `image-registry-operator` namespace letting it watch all namespaces:
```
kubectl apply -k github.com/mgoltzsche/image-registry-operator/deploy/cluster-wide
```

## Install on Minikube

Create a Minikube (1.11) cluster using CRI-O:
```
minikube start --kubernetes-version=1.18.3 --network-plugin=cni --enable-default-cni --container-runtime=cri-o --bootstrapper=kubeadm
```

Install the operator cluster-wide with [nodehack](https://github.com/mgoltzsche/nodehack) in the `image-registry-operator` namespace:
```
kubectl apply -k github.com/mgoltzsche/image-registry-operator/deploy/minikube
```

# Usage examples

Create an `ImageRegistry` (a self-signed TLS certificate is used if no secret or issuer is provided):
```
kubectl apply -f - <<-EOF
	apiVersion: registry.mgoltzsche.github.com/v1alpha1
	kind: ImageRegistry
	metadata:
	  name: registry
	spec:
	  replicas: 1
	  tls: {}
	  # You may want to specify the TLS certificate
	  # by either providing a secret or referring to a cert-manager issuer:
	  #  secretName: my-registry-tls
	  #  issuerRef:
	  #    name: my-lets-encrypt-issuer
	  #    kind: Issuer
	  persistentVolumeClaim:
	    # You may want to use a different StorageClass here:
	    storageClassName: standard
	    accessModes:
	    - ReadWriteOnce # If >1 replicas ever required ReadWriteMany must be set (which is the default)
	    resources:
	      requests:
	        storage: 1Gi
EOF
```

Create an `ImagePushSecret`:
```
kubectl apply -f - <<-EOF
	apiVersion: registry.mgoltzsche.github.com/v1alpha1
	kind: ImagePushSecret
	metadata:
	  name: example
	spec:
	  registryRef: # when omitted operator's default registry is used
	    name: registry
	    #namespace: infra # another namespace's registry could be used
EOF
```

Create an `ImagePullSecret`:
```
kubectl apply -f - <<-EOF
	apiVersion: registry.mgoltzsche.github.com/v1alpha1
	kind: ImagePullSecret
	metadata:
	  name: example
	spec:
	  registryRef:
	    name: registry
EOF
```

Create an `ImageBuildEnv`:
```
kubectl apply -f - <<-EOF
	apiVersion: registry.mgoltzsche.github.com/v1alpha1
	kind: ImageBuildEnv
	metadata:
	  name: example
	spec:
	  redis: true
	  secrets:
	  - secretName: imagepushsecret-example
EOF
```

Configure your local host to use the previously created `ImagePushSecret`'s Docker config:
```
kubectl get secret imagepushsecret-example -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d > ~/.docker/config.json
```
To use a self-signed registry cert (for development) configure `/etc/docker/daemon.json` with (docker needs to be restarted):
```
{
  "insecure-registries" : ["registry.default.svc.cluster.local"]
}

```
When running the registry in minikube you need to map the `registry` Service's IP on your host.


# How to build
Build the operator as well as preconfigured [docker_auth](https://github.com/cesanta/docker_auth) and [nginx](https://www.nginx.com/) images (requires make and docker/podman):
```
make operator docker_auth nginx
```


# How to test

## Unit tests
```
make unit-tests
```

## e2e tests

### Start Minikube
```
make start-minikube
```

### Test local changes
Test the locally built operator binary without building/pushing it as new container image:
```
export KUBECONFIG=$HOME/.kube/config
make containerized-operatorsdk-tests-local
```

### Test rollout with cert-manager
```
export KUBECONFIG=$HOME/.kube/config
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.15.1/cert-manager.yaml
kubectl rollout status -w --timeout 120s -n cert-manager deploy cert-manager-webhook
kubectl wait --for condition=established --timeout 20s crd issuers.cert-manager.io
make containerized-kubectl-tests
```

# Development & contributions

Contributions are welcome.
Changes and large features should be discussed in an issue first though.

## Source structure & generation

The operator skeleton has been generated using the [operator-sdk](https://github.com/operator-framework/operator-sdk):
* The `deploy` directory contains the corresponding kubernetes manifests.
* The `deploy/crds` directory is generated from `pkg/apis/registry/*/*_types.go`.
* The `pkg/controller/*` directories contain the code that handles the corresponding CRD.

The CRDs in `deploy/crd` and `zz_*.go` files need to be regenerated as follows when an API type changes:
```
make generate
```

## Test local operator changes interactively

1) Follow the instructions above to install the operator on minikube and run the examples
2) Run the operator using [skaffold](https://skaffold.dev/) (instantly redeploys on source changes):
```
skaffold dev
```

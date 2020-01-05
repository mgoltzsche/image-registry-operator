secret manager
===

A controller that watches `ImagePullSecret` CRs and generates concrete secrets for them.
The CR usage allows to declare docker registry authentication or delegate it using RBAC rules.  

For each `ImagePullSecret` CR a dockerconfig secret is maintained.
The corresponding registry URL is configured as operator deployment environment variable.
The username is derived from the CR following the scheme `<namespace>/<name>/<status.rotation>`.
The password is rotated using the secret's `nextpassword` field to allow asynchronous credential
sync by publishing its bcrypt hash with the CR's `passwords` status field one rotation before its
plain text version is rotated into the `.dockerconfigjson` key within the generated `Secret`
to be actually used by clients.
Correspondingly the active two bcrypt password hashes are maintained within the CR's status field `passwords`.
Other applications may use them to authenticate users.  

The `authservice` is meant to run in a pod with [cesanta/docker_auth](https://github.com/cesanta/docker_auth)
which could simply call it using `wget` configured as `ext_auth` to delegate authentication
while authorization could be specified using [docker_auth's ACL](https://github.com/cesanta/docker_auth/blob/master/docs/Labels.md).  

_Initially the idea was to generate a htpasswd into a secret within the
operator's namespace but the amount of process restarts it would have cost when mounting
them made the solution less attractive. However it would still be possible to do so._

# Development notes

The operator skeleton has been generated using the [operator-sdk](https://github.com/operator-framework):
* The `deploy` directory contains the corresponding kubernetes manifests.
* The `deploy/crds` directory is generated from `pkg/apis/credentialmanager/v1alpha1/*_types.go`.
* The `pkg/controller/*` directories contain the code that handles the corresponding CRD.

The CRD files in `deploy/crd` can be generated as follows:
```
operator-sdk generate k8s
operator-sdk generate crds
```
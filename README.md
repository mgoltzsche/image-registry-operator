secret operator
===

An operator that watches abstract secret representations (CRDs) and generates concrete for them.

For each ImagePullSecret resource a dockerconfig secret is maintained (and optionally rotated).
The registry URL may be loaded using another CRD.
The same credentials that have been written into the secret are also written into a shared secret
within the operator namespace that contains the htpasswd file from where users can be authenticated
by another service. This allows to use RBAC to specify image pull/push permissions and delegate
this authority to users within their namespace as well.

# Design decisions

## ImagePullSecret/ImagePullSecret

### Why not use existing ServiceAccount tokens as pull secrets?
A ServiceAccount token provides access to the K8s API and does not change (yet?).
These privileges should not be exposed to/mixed up with the registry:
* The registry is a separate system and its authentication is configured in a separate format anyway.
* Users may copy a pull secret to their local machine in order to get registry access locally during development.
  In case of a mobile device this can have serious security implications if the device is stolen and the secret is not rotated frequently.
  (However when the registry is integrated with an SSO system as well this scenario should not happen.)
* cesanta/docker_auth does not support Bearer token authentication for external or plugin authn (see below)

### Why rotate registry user credentials?
Because the registry is a critical infrastructure component and
users might copy pull/push secrets from the cluster to their (mobile) device which could get into wrong hands.

### Why use a CRD instead of a ServiceAccount annotation?
Push secrets should not be used as pull secrets and therefore also not be attached
to a ServiceAccount (to avoid giving a potentially compromised node image write access).
Thus there must be way to specify a secret without annotating a ServiceAccount: CRD.

#### Why have a CRD for a secret but not for a ServiceAccount?
To simplify optional usage:
In sub projects the ServiceAccount should be independent from the CRD/CR that could be deployed separately or not deployed at all.
A ServiceAccount can be set up with an imagePullSecret pointing to a secret not (yet) existing secret and used immediately -  
TODO: Verify that it also works when the pull secret is created after the first container start attempt referring to it.

### Why have separate ImageRegistryAccount?
* Authentication via secret check only would allow any account that can create/delete secrets to create/delete a registry account.
* The auth service should be granted access to resources within its own namespace, not resources of others.
* To allow static (not rotated) account creation (e.g. for external services) - not recommended though.

### Why use basic auth instead of a JWT?
* Registry access can be denied immediately by removing the CR (JWT would still be valid until it expires - however docker's JWT may still be valid anyway).
* With JWT secret renewal interval would need to be relatively short.
* Currently cesanta/docker_auth doesn't support Bearer Token (JWT) authentication for external or plugin authentication but basic auth only.
  (Docker proceeds with docker_auth's short-lived JWT afterwards anyway)

### Why use separate ImagePullSecret/ImagePushSecret CRDs instead of a single CRD with a push flag/subtype?
* Ensure pull secret format differs from push secret format as well as the pull password differs from the push password. (pull secrets are more widely distributed (to nodes) than push secrets and the latter have a higher security impact)
* Declare either a pull or a push secret when needed - not always both (imagine one pull secret and multiple push secrets per namespace or push secrets only)
* Allow users to delegate pull and push account management separately (RBAC)
* Avoid controller edge cases that would occur when changing a CR's mode from push to pull or vice versa (CR/secret password sync)
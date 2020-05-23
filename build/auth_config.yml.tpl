server:
  addr: "${AUTH_SERVER_ADDR}"

plugin_authn:
  plugin_path: /docker_auth/k8s-docker-authn.so

token:
  issuer: "${AUTH_TOKEN_ISSUER}"  # Must match issuer in the Registry config.
  expiration: ${AUTH_TOKEN_EXPIRATION}
  certificate: "${AUTH_TOKEN_CRT}"
  key: "${AUTH_TOKEN_KEY}"

acl:
  - match:
      origin: cr
      accessMode: push
    actions:
    - pull
    - push
    comment: ImagePushSecret users can push/pull
  - match:
      origin: cr
    actions:
    - pull
    comment: ImagePullSecret users can pull

#!/bin/sh

TEST_NAMESPACE=image-registry-test-$(date '+%Y%m%d-%H%M%S')

set -x

kubectl create namespace ${TEST_NAMESPACE}-registry &&
kubectl create namespace ${TEST_NAMESPACE}-user &&
kubectl apply -k ./deploy-overlays/cluster-scoped &&
kubectl apply -k ./deploy/registry-selfsigned -n ${TEST_NAMESPACE}-registry &&
kubectl wait --for condition=ready --timeout 120s -n ${TEST_NAMESPACE}-registry imageregistry/registry &&
(
kubectl apply -n ${TEST_NAMESPACE}-user -f - <<-EOF
apiVersion: registry.mgoltzsche.github.com/v1alpha1
kind: ImagePushSecret
metadata:
  name: example
spec:
  registryRef:
    name: registry
    namespace: ${TEST_NAMESPACE}-registry
EOF
) &&
kubectl wait --for condition=ready --timeout 45s -n ${TEST_NAMESPACE}-user imagepushsecret/example &&
kubectl get -n ${TEST_NAMESPACE}-user secret imagepushsecret-example &&
kubectl get -n ${TEST_NAMESPACE}-registry imageregistryaccount push.${TEST_NAMESPACE}-user.example.0
STATUS=$?
kubectl delete namespace ${TEST_NAMESPACE}-registry
kubectl delete namespace ${TEST_NAMESPACE}-user
kubectl delete -k ./deploy-overlays/cluster-scoped
exit $STATUS
#!/bin/sh

TEST_NAMESPACE=image-registry-test-$(date '+%Y%m%d-%H%M%S')

set -x

(
	set -ex
	kubectl create namespace ${TEST_NAMESPACE}-registry-issuer
	kubectl create namespace ${TEST_NAMESPACE}-registry-selfsigned
	kubectl create namespace ${TEST_NAMESPACE}-user
	kubectl apply -k ./deploy-overlays/cluster-scoped
	for REG in selfsigned issuer; do
		kubectl apply -k ./examples/registry-$REG -n ${TEST_NAMESPACE}-registry-$REG
		kubectl wait --for condition=ready --timeout 120s -n ${TEST_NAMESPACE}-registry-$REG imageregistry/registry
		kubectl apply -f - <<-EOF
		apiVersion: registry.mgoltzsche.github.com/v1alpha1
		kind: ImagePushSecret
		metadata:
		  name: example
		  namespace: ${TEST_NAMESPACE}-user
		spec:
		  registryRef:
		    name: registry
		    namespace: ${TEST_NAMESPACE}-registry-$REG
		EOF
		kubectl wait --for condition=ready --timeout 45s -n ${TEST_NAMESPACE}-user imagepushsecret/example
	done
	kubectl get -n ${TEST_NAMESPACE}-user secret imagepushsecret-example
	kubectl get -n ${TEST_NAMESPACE}-registry-selfsigned imageregistryaccount
	kubectl get -n ${TEST_NAMESPACE}-registry-selfsigned imageregistryaccount push.${TEST_NAMESPACE}-user.example.1
)
STATUS=$?
echo >&2
kubectl delete namespace ${TEST_NAMESPACE}-user
kubectl delete namespace ${TEST_NAMESPACE}-registry-selfsigned
kubectl delete namespace ${TEST_NAMESPACE}-registry-issuer
kubectl delete -k ./deploy-overlays/cluster-scoped
exit $STATUS
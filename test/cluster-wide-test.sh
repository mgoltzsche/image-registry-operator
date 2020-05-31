#!/bin/sh

TEST_NAMESPACE=registry-test-$(date '+%Y%m%d-%H%M%S')

set -x

(
	set -ex
	kubectl create namespace ${TEST_NAMESPACE}-issuer
	kubectl create namespace ${TEST_NAMESPACE}-self-signed
	kubectl create namespace ${TEST_NAMESPACE}-user
	kubectl apply -k deploy/cluster-wide
	for REG in self-signed issuer; do
		kubectl apply -k deploy/examples/registry-$REG -n ${TEST_NAMESPACE}-$REG
		kubectl wait --for condition=ready --timeout 120s -n ${TEST_NAMESPACE}-$REG imageregistry/registry
		kubectl apply -f - <<-EOF
		apiVersion: registry.mgoltzsche.github.com/v1alpha1
		kind: ImagePushSecret
		metadata:
		  name: example
		  namespace: ${TEST_NAMESPACE}-user
		spec:
		  registryRef:
		    name: registry
		    namespace: ${TEST_NAMESPACE}-$REG
		EOF
		kubectl wait --for condition=ready --timeout 45s -n ${TEST_NAMESPACE}-user imagepushsecret/example
	done
	kubectl get -n ${TEST_NAMESPACE}-user secret imagepushsecret-example
	kubectl get -n ${TEST_NAMESPACE}-self-signed imageregistryaccount
	kubectl get -n ${TEST_NAMESPACE}-self-signed imageregistryaccount push.${TEST_NAMESPACE}-user.example.1
)
STATUS=$?
echo >&2
kubectl delete namespace ${TEST_NAMESPACE}-user
kubectl delete namespace ${TEST_NAMESPACE}-self-signed
kubectl delete namespace ${TEST_NAMESPACE}-issuer
kubectl delete -k ./deploy/cluster-wide
exit $STATUS
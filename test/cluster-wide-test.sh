#!/bin/sh

cd "$(dirname $0)"/..

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
		[ "$CERTMAN_INSTALLED" ] || kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.15.1/cert-manager.yaml
		CERTMAN_INSTALLED=1
	done
	for KIND in ImagePullSecret ImagePushSecret; do
		kubectl apply -f - <<-EOF
		apiVersion: registry.mgoltzsche.github.com/v1alpha1
		kind: $KIND
		metadata:
		  name: example
		  namespace: ${TEST_NAMESPACE}-user
		spec:
		  registryRef:
		    name: registry
		    namespace: ${TEST_NAMESPACE}-self-signed
		EOF
		kubectl wait --for condition=ready --timeout 45s -n ${TEST_NAMESPACE}-user $KIND/example
	done
	kubectl get -n ${TEST_NAMESPACE}-user secret imagepushsecret-example
	kubectl get -n ${TEST_NAMESPACE}-self-signed imageregistryaccount
	kubectl get -n ${TEST_NAMESPACE}-self-signed imageregistryaccount push.${TEST_NAMESPACE}-user.example.1
	kubectl apply -n ${TEST_NAMESPACE}-user -f test/makisu-job.yaml
	kubectl wait -n ${TEST_NAMESPACE}-user job makisu-job --for condition=complete --timeout 120s || (
		kubectl -n ${TEST_NAMESPACE}-user logs $(kubectl get -n ${TEST_NAMESPACE}-user pod -o jsonpath='{.items[*].metadata.name}' | grep -Eo 'makisu-job-[^ ]+' | head -1)
		false
	)
	kubectl apply -n ${TEST_NAMESPACE}-user -f - <<-EOF
		apiVersion: v1
		kind: Pod
		metadata:
		  name: example-app
		spec:
		  containers:
		  - name: example
		    image: registry.${TEST_NAMESPACE}-self-signed.svc.cluster.local/example:latest
		    imagePullPolicy: Always
		  imagePullSecrets:
		  - name: imagepullsecret-example
	EOF
	kubectl wait -n ${TEST_NAMESPACE}-user pod example-app --for condition=ready --timeout 45s || (
		kubectl describe -n ${TEST_NAMESPACE}-user pod example-app
		false
	)
)
STATUS=$?
echo >&2
kubectl delete namespace ${TEST_NAMESPACE}-user
kubectl delete namespace ${TEST_NAMESPACE}-self-signed
kubectl delete namespace ${TEST_NAMESPACE}-issuer
exit $STATUS

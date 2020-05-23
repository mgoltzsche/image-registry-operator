#!/bin/sh

TEST_NAMESPACE=image-registry-operator-test-$(date '+%Y%m%d-%H%M%S')

set -x

kubectl create namespace ${TEST_NAMESPACE}-operator &&
kubectl create namespace ${TEST_NAMESPACE}-registry &&
kubectl apply -k ./deploy -n ${TEST_NAMESPACE}-operator &&
kubectl apply -k ./deploy/registry-selfsigned -n ${TEST_NAMESPACE}-operator &&
kubectl wait --for condition=ready --timeout 120s -n ${TEST_NAMESPACE}-operator imageregistry/registry
STATUS=$?
kubectl delete namespace ${TEST_NAMESPACE}-registry
kubectl delete namespace ${TEST_NAMESPACE}-operator
exit $STATUS
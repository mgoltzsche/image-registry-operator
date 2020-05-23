PKG=github.com/mgoltzsche/image-registry-operator
TEST_IMAGE=image-registry-operator-test
TEST_NAMESPACE=image-registry-operator-test-$(shell date '+%Y%m%d-%H%M%S')
define TESTDOCKERFILE
	FROM $(TEST_IMAGE)
	ENV K8S_VERSION=v1.18.2
	RUN apk add --update --no-cache git \
		&& curl -fsSLo /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/$${K8S_VERSION}/bin/linux/amd64/kubectl \
		&& chmod +x /usr/local/bin/kubectl
	#ENV OPERATOR_SDK_VERSION=v0.16.0
	#RUN curl -fsSLo /usr/local/bin/operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/$${OPERATOR_SDK_VERSION}/operator-sdk-$${OPERATOR_SDK_VERSION}-x86_64-linux-gnu \
	#		&& chmod +x /usr/local/bin/operator-sdk
endef
export TESTDOCKERFILE


all: operator docker_auth nginx

operator:
	docker build --force-rm -t image-registry-operator -f build/Dockerfile --target operator .

docker_auth:
	docker build --force-rm -t docker_auth -f build/Dockerfile-auth .

nginx:
	docker build --force-rm -t registry-nginx -f build/Dockerfile-nginx .

unit-tests:
	docker build --force-rm -f build/Dockerfile .

e2e-tests:
	make containerized-run-e2e-tests

containerized-%: test-image
	$(eval DOCKER ?= $(if $(shell docker -v),docker,podman))
	$(DOCKER) run --rm --net host \
		-v "`pwd`:/go/src/$(PKG)" \
		--mount "type=bind,src=$$KUBECONFIG,dst=/root/.kube/config" \
		--entrypoint /bin/sh $(TEST_IMAGE) -c "make $*"

test-image:
	docker build --force-rm -t $(TEST_IMAGE) -f build/Dockerfile --target=builddeps .
	echo "$$TESTDOCKERFILE" | docker build --force-rm -t $(TEST_IMAGE) -f - .

generate:
	#operator-sdk add api --api-version=registry.mgoltzsche.github.com/v1alpha1 --kind=ImageRegistryAccount
	#operator-sdk add controller --api-version=registry.mgoltzsche.github.com/v1alpha1 --kind=ImageRegistryAccount
	operator-sdk generate k8s
	operator-sdk generate crds

run-e2e-tests: operatorsdk-tests kubectl-tests

operatorsdk-tests:
	kubectl create namespace $(TEST_NAMESPACE)
	operator-sdk test local ./test/e2e --namespace $(TEST_NAMESPACE) --up-local; \
	STATUS=$$?; \
	kubectl delete namespace $(TEST_NAMESPACE); \
	exit $$STATUS

kubectl-tests:
	./test/test.sh

install-tools: download-deps
	cat tools.go | grep -E '^\s*_' | cut -d'"' -f2 | xargs -n1 go install

download-deps:
	go mod download

clean:
	rm -rf build/_output

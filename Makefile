PKG=github.com/mgoltzsche/credential-manager
TEST_IMAGE=credential-manager-test
define TESTDOCKERFILE
	FROM $(TEST_IMAGE)
	RUN apk add --update --no-cache curl make gcc libc-dev
	ENV K8S_VERSION=v1.17.3
	RUN curl -fsSLo /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/$${K8S_VERSION}/bin/linux/amd64/kubectl \
		&& chmod +x /usr/local/bin/kubectl
	ENV OPERATOR_SDK_VERSION=v0.16.0
	RUN curl -fsSLo /usr/local/bin/operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/$${OPERATOR_SDK_VERSION}/operator-sdk-$${OPERATOR_SDK_VERSION}-x86_64-linux-gnu \
		&& chmod +x /usr/local/bin/operator-sdk
endef
export TESTDOCKERFILE


all: operator authenticator

operator:
	docker build --force-rm -t credential-manager -f build/Dockerfile .

authenticator:
	docker build --force-rm -t registry-authenticator -f build/Dockerfile-auth .

containerized-%: test-image
	$(eval DOCKER ?= $(if $(shell docker -v),docker,podman))
	$(DOCKER) run --rm --net host \
		-v "`pwd`:/go/src/$(PKG)" \
		-v "$$KUBECONFIG:/root/.kube/config" \
		--entrypoint /bin/sh $(TEST_IMAGE) -c "make $*"

test-image:
	docker build --force-rm -t $(TEST_IMAGE) -f build/Dockerfile --target=builddeps .
	echo "$$TESTDOCKERFILE" | docker build --force-rm -t $(TEST_IMAGE) -f - .

generate:
	operator-sdk generate k8s
	operator-sdk generate crds

e2e-tests:
	kubectl create namespace operator-test
	operator-sdk test local ./test/e2e --namespace operator-test --up-local; \
	STATUS=$$?; \
	kubectl delete namespace operator-test; \
	exit $$STATUS

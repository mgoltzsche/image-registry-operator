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


all: operator docker_auth nginx makisu

operator:
	docker build --force-rm -t image-registry-operator -f build/Dockerfile --target operator .

docker_auth:
	docker build --force-rm -t docker_auth -f build/Dockerfile-auth .

nginx:
	docker build --force-rm -t registry-nginx -f build/Dockerfile-nginx .

makisu:
	docker build --force-rm -t makisu -f build/Dockerfile-makisu .

unit-tests:
	docker build --force-rm -f build/Dockerfile .

e2e-tests:
	make containerized-run-e2e-tests

containerized-%: test-image
	$(eval DOCKER ?= $(if $(shell docker -v),docker,podman))
	$(eval DOPTS := $(if $(wildcard $(HOME)/.minikube),-v $(HOME)/.minikube:$(HOME)/.minikube,))
	$(DOCKER) run --rm --net host \
		-v "`pwd`:/go/src/$(PKG)" \
		--mount "type=bind,src=$$KUBECONFIG,dst=/root/.kube/config" \
		$(DOPTS) \
		--entrypoint /bin/sh $(TEST_IMAGE) -c "make $*"

test-image:
	docker build --force-rm -t $(TEST_IMAGE) -f build/Dockerfile --target=builddeps .
	echo "$$TESTDOCKERFILE" | docker build --force-rm -t $(TEST_IMAGE) -f - .

generate:
	#operator-sdk add api --api-version=registry.mgoltzsche.github.com/v1alpha1 --kind=ImageBuildEnv
	#operator-sdk add controller --api-version=registry.mgoltzsche.github.com/v1alpha1 --kind=ImageBuildEnv
	operator-sdk generate k8s
	operator-sdk generate crds

run-e2e-tests: operatorsdk-tests-local kubectl-tests

operatorsdk-tests-local:
	kubectl create namespace $(TEST_NAMESPACE)-local
	# TODO: fix test namespace
	operator-sdk test local ./test/e2e --up-local --operator-namespace $(TEST_NAMESPACE)-local; \
	STATUS=$$?; \
	kubectl delete namespace $(TEST_NAMESPACE)-local; \
	exit $$STATUS

operatorsdk-tests:
	kubectl create namespace $(TEST_NAMESPACE)
	for M in service_account role role_binding operator; do \
		echo '---'; cat deploy/operator/$${M}.yaml; \
	done >/tmp/registryoperator-manifest-e2e.yaml
	operator-sdk test local ./test/e2e --operator-namespace $(TEST_NAMESPACE) --namespaced-manifest /tmp/registryoperator-manifest-e2e.yaml; \
	STATUS=$$?; \
	kubectl delete namespace $(TEST_NAMESPACE); \
	exit $$STATUS

kubectl-tests:
	./test/cluster-wide-test.sh

install-tools: download-deps
	cat tools.go | grep -E '^\s*_' | cut -d'"' -f2 | xargs -n1 go install

download-deps:
	go mod download

clean:
	rm -rf build/_output .kubeconfig

start-minikube:
	minikube start --kubernetes-version=1.20.5 --network-plugin=cni --enable-default-cni --container-runtime=cri-o --bootstrapper=kubeadm

delete-minikube:
	minikube delete

release-kustomization: KUSTOMIZATION_DIR=deploy/operator
release-kustomization:
	VERSION=$(VERSION) KUSTOMIZATION_DIR=$(KUSTOMIZATION_DIR) ./hack/release.sh

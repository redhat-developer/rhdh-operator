PROFILES := $(shell find config/manifests -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)

# Profile directory: subdirectory of ./config/profile
# In terms of Kustomize it is overlay directory
# It also contains default-config directory
# with set of Backstage Default Configuration YAML manifests
# to use other config - add a directory with config,
# use it as following commands: 'PROFILE=<dir-name> make test|integration-test|run|deploy|deployment-manifest'
PROFILE ?= rhdh
PROFILE_SHORT := $(shell echo $(PROFILE) | cut -d. -f1)

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
# Set a default VERSION if it is not defined
ifeq ($(origin VERSION), undefined)
VERSION ?= 0.7.0
DEFAULT_VERSION := true
else
DEFAULT_VERSION := false
endif

# IMAGE_TAG_VERSION is the image tag, which might be different from VERSION.
# For example, the RHDH profile uses 1.y as image tags if VERSION is 1.y.z
IMAGE_TAG_VERSION = $(VERSION)

ifeq ($(PROFILE), rhdh)
	# Profile-specific settings
	ifeq ($(DEFAULT_VERSION), true)
		# transforming: 0.y.z => 1.y.z. Only if VERSION was not explicitly overridden by the user
		MAJOR := $(shell echo $(VERSION) | cut -d. -f1)
		INCREMENTED_MAJOR := $(shell expr $(MAJOR) + 1)
		MINOR_PATCH := $(shell echo $(VERSION) | cut -d. -f2-)
		VERSION := $(INCREMENTED_MAJOR).$(MINOR_PATCH)
		IMAGE_TAG_VERSION := $(shell echo $(VERSION) | cut -d. -f1,2)
	endif

	# IMAGE_TAG_BASE ?= registry.redhat.io/rhdh/rhdh-rhel9-operator
	IMAGE_TAG_BASE ?= quay.io/rhdh/rhdh-rhel9-operator
	DEFAULT_CHANNEL ?= fast
	CHANNELS ?= fast,fast-\$${CI_X_VERSION}.\$${CI_Y_VERSION}
	BUNDLE_METADATA_PACKAGE_NAME ?= rhdh
else
	# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
    # This variable is used to construct full image tags for bundle and catalog images.
    #
    # For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
    # quay.io/rhdh-community/operator-bundle:$VERSION and quay.io/rhdh-community/operator-catalog:$VERSION.
    IMAGE_TAG_BASE ?= quay.io/rhdh-community/operator

	# DEFAULT_CHANNEL defines the default channel used in the bundle.
	# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
	# To re-generate a bundle for any other default channel without changing the default setup, you can:
	# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
	# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
	DEFAULT_CHANNEL ?= alpha

	# BUNDLE_METADATA_PACKAGE_NAME is the name of the package in the bundle
	BUNDLE_METADATA_PACKAGE_NAME ?= backstage-operator
endif

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS ?= $(DEFAULT_CHANNEL)
BUNDLE_CHANNELS ?= --channels=$(CHANNELS)
BUNDLE_DEFAULT_CHANNEL ?= --default-channel=$(DEFAULT_CHANNEL)
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(IMAGE_TAG_VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) --output-dir bundle/$(PROFILE) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.37.0
OPM_VERSION ?= v1.23.0
# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_TAG_VERSION)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
# CONTAINER_ENGINE kept for backward compatibility if defined.
ifneq ($(origin CONTAINER_TOOL), undefined)
CONTAINER_TOOL := $(CONTAINER_TOOL)
else
ifneq ($(origin CONTAINER_ENGINE), undefined)
CONTAINER_TOOL := $(CONTAINER_ENGINE)
else
CONTAINER_TOOL := docker
endif
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Source packages outside of tests
PKGS := $(shell go list ./... | grep -v /e2e)

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object paths="./..."

.PHONY: fmt
fmt: goimports ## Format the code using goimports
	find . -not -path '*/\.*' -name '*.go' -exec $(GOIMPORTS) -w {} \;

.PHONY: test
test: manifests generate fmt vet envtest $(LOCALBIN) ## Run tests. We need LOCALBIN=$(LOCALBIN) to get correct default-config path
	mkdir -p $(LOCALBIN)/default-config && rm -fr $(LOCALBIN)/default-config/* && cp -r config/profile/$(PROFILE)/default-config/* $(LOCALBIN)/default-config
	mkdir -p $(LOCALBIN)/plugin-deps && rm -fr $(LOCALBIN)/plugin-deps/* && cp -r config/profile/$(PROFILE)/plugin-deps/* $(LOCALBIN)/plugin-deps 2>/dev/null || :
	LOCALBIN=$(LOCALBIN) KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(PKGS) -coverprofile cover.out

.PHONY: integration-test
integration-test: ginkgo manifests generate fmt vet envtest $(LOCALBIN) ## Run integration_tests. We need LOCALBIN=$(LOCALBIN) to get correct default-config path
	mkdir -p $(LOCALBIN)/default-config && rm -fr $(LOCALBIN)/default-config/* && cp -r config/profile/$(PROFILE)/default-config/* $(LOCALBIN)/default-config
	mkdir -p $(LOCALBIN)/plugin-deps && rm -fr $(LOCALBIN)/plugin-deps/* && cp -r config/profile/$(PROFILE)/plugin-deps/* $(LOCALBIN)/plugin-deps 2>/dev/null || :
	LOCALBIN=$(LOCALBIN) KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -v -r $(ARGS) integration_tests

# After this time, Ginkgo will emit progress reports, so we can get visibility into long-running tests.
POLL_PROGRESS_INTERVAL := 120s
TIMEOUT ?= 14400s
GINKGO_FLAGS_ALL = $(GINKGO_TEST_ARGS) --randomize-all --poll-progress-after=$(POLL_PROGRESS_INTERVAL) --poll-progress-interval=$(POLL_PROGRESS_INTERVAL) -timeout $(TIMEOUT) --no-color
# Flags for tests that may be run in parallel
GINKGO_FLAGS=$(GINKGO_FLAGS_ALL) -nodes=$(TEST_EXEC_NODES)
# Flags to run one test per core.
GINKGO_FLAGS_AUTO = $(GINKGO_FLAGS_ALL) -p
ifdef TEST_EXEC_NODES
   TEST_EXEC_NODES := $(TEST_EXEC_NODES)
else
   TEST_EXEC_NODES := 1
endif

.PHONY: test-e2e
test-e2e: ginkgo ## Run end-to-end tests. See the 'tests/e2e/README.md' file for more details.
	# go test ./test/e2e/ -v -ginkgo.v
	$(GINKGO) $(GINKGO_FLAGS) --skip-file=e2e_upgrade_test.go tests/e2e

.PHONY: test-e2e-upgrade
test-e2e-upgrade: ginkgo ## Run end-to-end tests dedicated to the operator upgrade paths. See the 'tests/e2e/README.md' file for more details.
	# go test ./test/e2e/ -v -ginkgo.v
	$(GINKGO) $(GINKGO_FLAGS) --focus-file=e2e_upgrade_test.go tests/e2e

.PHONY: gosec
gosec: addgosec ## run the gosec scanner for non-test files in this repo
  	# we let the report content trigger a failure using the GitHub Security features.
	$(GOSEC) -no-fail -fmt $(GOSEC_FMT) -out $(GOSEC_OUTPUT_FILE)  ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run --timeout 15m

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix --timeout 15m

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet $(LOCALBIN) ## Run a controller from your host.
	mkdir -p $(LOCALBIN)/default-config/ &&	rm -fr $(LOCALBIN)/default-config/* && cp -r config/profile/$(PROFILE)/default-config/* $(LOCALBIN)/default-config/
	mkdir -p $(LOCALBIN)/plugin-deps/ &&	rm -fr $(LOCALBIN)/plugin-deps/* && cp -r config/profile/$(PROFILE)/plugin-deps/* $(LOCALBIN)/plugin-deps/ 2>/dev/null || :
	go run -C $(LOCALBIN) ../cmd/main.go

# by default images expire from quay registry after 14 days
# set a longer timeout (or set no label to keep images forever)
LABEL ?= quay.expires-after=14d
PLATFORM ?= linux/amd64

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: image-build
image-build: ## Build container image with the manager.
	$(CONTAINER_TOOL) build --platform $(PLATFORM) -t $(IMG) --label $(LABEL) .

.PHONY: image-push
image-push: ## Push container image with the manager.
	$(CONTAINER_TOOL) push $(IMG)

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} --label $(LABEL) -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm -f Dockerfile.cross

.PHONY: build-installers
build-installers: ## Generate a consolidated YAML with CRDs and deployment for all available profiles.
	@for profile in $(PROFILES); do \
  		$(MAKE) build-installer PROFILE=$$profile; \
  	done

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist/$(PROFILE)
	cd config/profile/$(PROFILE) && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/profile/$(PROFILE) > dist/$(PROFILE)/install.yaml
	@echo "Generated operator installer manifest: dist/$(PROFILE)/install.yaml"

.PHONY: deployment-manifest
deployment-manifest: build-installer ## Generate manifest to deploy operator. Deprecated. Use 'make build-installer' instead.
	cp -f dist/$(PROFILE)/install.yaml $(PROFILE)-operator-${VERSION}.yaml
	@echo "Generated operator script $(PROFILE)-operator-${VERSION}.yaml"


# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(IMAGE_TAG_VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

.PHONY: bundles
bundles: ## Generate bundle manifests and metadata, then validate generated files for all available profiles.
	@for profile in $(PROFILES); do \
  		$(MAKE) bundle PROFILE=$$profile; \
  	done

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q --interactive=false \
		--apis-dir ./api/ \
		--input-dir ./config/manifests/$(PROFILE)/ \
		--output-dir ./config/manifests/$(PROFILE)/
	cd config/profile/$(PROFILE) && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests/$(PROFILE) | $(OPERATOR_SDK) generate bundle --kustomize-dir config/manifests/$(PROFILE) $(BUNDLE_GEN_FLAGS)
	@mv -f bundle.Dockerfile ./bundle/$(PROFILE)/bundle.Dockerfile
	@sed -i 's/backstage-operator.v$(VERSION)/$(PROFILE_SHORT)-operator.v$(VERSION)/g' ./bundle/$(PROFILE)/manifests/backstage-operator.clusterserviceversion.yaml
	@sed -i 's/backstage-operator/$(BUNDLE_METADATA_PACKAGE_NAME)/g' ./bundle/$(PROFILE)/metadata/annotations.yaml
	@sed -i 's/backstage-operator/$(BUNDLE_METADATA_PACKAGE_NAME)/g' ./bundle/$(PROFILE)/bundle.Dockerfile
	$(OPERATOR_SDK) bundle validate ./bundle/$(PROFILE)

## to update the CSV with a new tagged version of the operator:
## yq '.spec.install.spec.deployments[0].spec.template.spec.containers[1].image|="quay.io/rhdh-community/operator:some-other-tag"' bundle/manifests/backstage-operator.clusterserviceversion.yaml
## or
## sed -r -e "s#(image: +)quay.io/.+operator.+#\1quay.io/rhdh-community/operator:some-other-tag#g" -i bundle/manifests/backstage-operator.clusterserviceversion.yaml
.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build --platform $(PLATFORM) -f bundle/$(PROFILE)/bundle.Dockerfile -t $(BUNDLE_IMG) --label $(LABEL) .

.PHONY: bundle-push
bundle-push: ## Push bundle image to registry
	$(MAKE) image-push IMG=$(BUNDLE_IMG)

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
# bundle-push is needed because 'opm index add' always pulls from the remote registry and never uses the local image.
# See https://github.com/operator-framework/operator-registry/issues/885
.PHONY: catalog-build
catalog-build: bundle-push opm ## Generate operator-catalog dockerfile using the operator-bundle image built and published above; then build catalog image
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT) --generate -d ./index.Dockerfile
	$(CONTAINER_TOOL) build --platform $(PLATFORM) -f index.Dockerfile -t $(CATALOG_IMG) --label $(LABEL) .

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) image-push IMG=$(CATALOG_IMG)

.PHONY:
release-build: bundle image-build bundle-build catalog-build ## Build operator, bundle + catalog images

.PHONY:
release-push: image-push bundle-push catalog-push ## Push operator, bundle + catalog images

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/profile/$(PROFILE) && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/profile/$(PROFILE) | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/profile/$(PROFILE) | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: plugin-infra
plugin-infra:
	@if [ -d "config/profile/$(PROFILE)/plugin-infra" ]; then \
		$(KUSTOMIZE) build config/profile/$(PROFILE)/plugin-infra | $(KUBECTL) apply -f -; \
	else \
		echo "Directory config/profile/$(PROFILE)/plugin-infra does not exist."; \
	fi

.PHONY: plugin-infra-undeploy
plugin-infra-undeploy:
	@if [ -d "config/profile/$(PROFILE)/plugin-infra" ]; then \
		$(KUSTOMIZE) build config/profile/$(PROFILE)/plugin-infra | $(KUBECTL) delete -f -; \
	else \
		echo "Directory config/profile/$(PROFILE)/plugin-infra does not exist."; \
	fi

##@ OLM Deployment

# It has to be the same namespace as ./config/default/kustomization.yaml -> namespace
OPERATOR_NAMESPACE ?= $(subst .,-, $(PROFILE))-operator
OLM_NAMESPACE ?= olm
OPENSHIFT_OLM_NAMESPACE = openshift-marketplace

.PHONY: deploy-olm
deploy-olm: ## Deploy the operator with OLM
	sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" config/samples/catalog-operator-group.yaml | \
		$(KUBECTL) -n ${OPERATOR_NAMESPACE} apply -f -
	sed "s/{{VERSION}}/$(subst /,\/,$(VERSION))/g" config/samples/catalog-subscription-template.yaml | \
		sed "s/{{DEFAULT_CHANNEL}}/$(subst /,\/,$(DEFAULT_CHANNEL))/g" | \
		sed "s/{{BUNDLE_METADATA_PACKAGE_NAME}}/$(subst /,\/,$(BUNDLE_METADATA_PACKAGE_NAME))/g" | \
		sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" | \
		sed "s/{{OLM_NAMESPACE}}/$(subst /,\/,$(OLM_NAMESPACE))/g" | \
		$(KUBECTL) -n ${OPERATOR_NAMESPACE} apply -f -

.PHONY: deploy-olm-openshift
deploy-olm-openshift: ## Deploy the operator with OLM
	sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" config/samples/catalog-operator-group.yaml | \
		$(KUBECTL) -n ${OPERATOR_NAMESPACE} apply -f -
	sed "s/{{VERSION}}/$(subst /,\/,$(VERSION))/g" config/samples/catalog-subscription-template.yaml | \
		sed "s/{{DEFAULT_CHANNEL}}/$(subst /,\/,$(DEFAULT_CHANNEL))/g" | \
		sed "s/{{BUNDLE_METADATA_PACKAGE_NAME}}/$(subst /,\/,$(BUNDLE_METADATA_PACKAGE_NAME))/g" | \
		sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" | \
		sed "s/{{OLM_NAMESPACE}}/$(subst /,\/,$(OPENSHIFT_OLM_NAMESPACE))/g" | \
		$(KUBECTL) -n ${OPERATOR_NAMESPACE} apply -f -

.PHONY: undeploy-olm
undeploy-olm: ## Un-deploy the operator with OLM
	-$(KUBECTL) -n ${OPERATOR_NAMESPACE} delete subscriptions.operators.coreos.com $(PROFILE_SHORT)-operator
	-$(KUBECTL) -n ${OPERATOR_NAMESPACE} delete operatorgroup $(PROFILE_SHORT)-operator-group
	-$(KUBECTL) -n ${OPERATOR_NAMESPACE} delete clusterserviceversion $(PROFILE_SHORT)-operator.v$(VERSION)

.PHONY: catalog-update
catalog-update: ## Update catalog source in the default namespace for catalogsource
	-$(KUBECTL) delete catalogsource $(PROFILE_SHORT)-operator -n $(OLM_NAMESPACE)
	sed "s/{{CATALOG_IMG}}/$(subst /,\/,$(CATALOG_IMG))/g" config/samples/catalog-source-template.yaml | \
		sed "s/{{PROFILE}}/$(subst /,\/,$(PROFILE))/g" | \
		sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" | \
		$(KUBECTL) apply -n $(OLM_NAMESPACE) -f -

.PHONY: catalog-update
catalog-update-openshift: ## Update catalog source in the default namespace for catalogsource
	-$(KUBECTL) delete catalogsource $(PROFILE_SHORT)-operator -n $(OPENSHIFT_OLM_NAMESPACE)
	sed "s/{{CATALOG_IMG}}/$(subst /,\/,$(CATALOG_IMG))/g" config/samples/catalog-source-template.yaml | \
		sed "s/{{PROFILE}}/$(subst /,\/,$(PROFILE))/g" | \
		sed "s/{{PROFILE_SHORT}}/$(subst /,\/,$(PROFILE_SHORT))/g" | \
		$(KUBECTL) apply -n $(OPENSHIFT_OLM_NAMESPACE) -f -

# Deploy on Openshift cluster using OLM (by default installed on Openshift)
.PHONY: deploy-openshift
deploy-openshift: release-build release-push catalog-update-openshift create-operator-namespace deploy-olm-openshift ## Deploy the operator on openshift cluster

.PHONY: install-olm
install-olm: operator-sdk ## Install the Operator Lifecycle Manager.
	$(OPSDK) olm install

.PHONY: create-operator-namespace
create-operator-namespace: ## Create the namespace where the operator will be deployed.
	-$(KUBECTL) create namespace ${OPERATOR_NAMESPACE}

.PHONY: deploy-k8s-olm
deploy-k8s-olm: release-build release-push catalog-update create-operator-namespace deploy-olm ## Deploy the operator on openshift cluster

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOIMPORTS ?= $(LOCALBIN)/goimports-$(GOIMPORTS_VERSION)
GOSEC ?= $(LOCALBIN)/gosec-$(GOSEC_VERSION)
GINKGO ?= $(LOCALBIN)/ginkgo-$(GINKGO_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.2
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.59.1
GOIMPORTS_VERSION ?= v0.16.1
GOSEC_VERSION ?= v2.20.0
GINKGO_VERSION ?= v2.22.2

## Gosec options - default format is sarif so we can integrate with Github code scanning
GOSEC_FMT ?= sarif  # for other options, see https://github.com/securego/gosec#output-formats
GOSEC_OUTPUT_FILE ?= gosec.sarif

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: goimports
goimports: $(GOIMPORTS) ## Download goimports locally if necessary.
$(GOIMPORTS): $(LOCALBIN)
	$(call go-install-tool,$(GOIMPORTS),golang.org/x/tools/cmd/goimports,${GOIMPORTS_VERSION})

.PHONY: addgosec
addgosec: $(GOSEC) ## Download gosec locally if necessary.
$(GOSEC): $(LOCALBIN)
	$(call go-install-tool,$(GOSEC),github.com/securego/gosec/v2/cmd/gosec,${GOSEC_VERSION})

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download Ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,${GINKGO_VERSION})

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk-$(OPERATOR_SDK_VERSION)
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
endif

.PHONY: opm
OPM ?= $(LOCALBIN)/opm-$(OPM_VERSION)
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
endif

##@ Misc.

show-img: ## Show the value of the IMG variable resolved.
	@echo -n $(IMG)

show-container-tool: ## Show the value of CONTAINER_TOOL variable resolved.
	@echo -n $(CONTAINER_TOOL)

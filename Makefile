# Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

REGISTRY                                   := eu.gcr.io/gardener-project/gardener
APISERVER_IMAGE_REPOSITORY                 := $(REGISTRY)/apiserver
CONTROLLER_MANAGER_IMAGE_REPOSITORY        := $(REGISTRY)/controller-manager
SCHEDULER_IMAGE_REPOSITORY                 := $(REGISTRY)/scheduler
ADMISSION_IMAGE_REPOSITORY                 := $(REGISTRY)/admission-controller
RESOURCE_MANAGER_IMAGE_REPOSITORY          := $(REGISTRY)/resource-manager
OPERATOR_IMAGE_REPOSITORY                  := $(REGISTRY)/operator
GARDENLET_IMAGE_REPOSITORY                 := $(REGISTRY)/gardenlet
EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY  := $(REGISTRY)/extensions/provider-local
PUSH_LATEST_TAG                            := false
VERSION                                    := $(shell cat VERSION)
EFFECTIVE_VERSION                          := $(VERSION)-$(shell git rev-parse HEAD)
REPO_ROOT                                  := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
GARDENER_LOCAL_KUBECONFIG                  := $(REPO_ROOT)/example/gardener-local/kind/local/kubeconfig
GARDENER_LOCAL2_KUBECONFIG                 := $(REPO_ROOT)/example/gardener-local/kind/local2/kubeconfig
GARDENER_LOCAL_HA_SINGLE_ZONE_KUBECONFIG   := $(REPO_ROOT)/example/gardener-local/kind/ha-single-zone/kubeconfig
GARDENER_LOCAL_HA_MULTI_ZONE_KUBECONFIG    := $(REPO_ROOT)/example/gardener-local/kind/ha-multi-zone/kubeconfig
GARDENER_LOCAL_OPERATOR_KUBECONFIG         := $(REPO_ROOT)/example/gardener-local/kind/operator/kubeconfig
LOCAL_GARDEN_LABEL                         := local-garden
REMOTE_GARDEN_LABEL                        := remote-garden
ACTIVATE_SEEDAUTHORIZER                    := false
SEED_NAME                                  := ""
DEV_SETUP_WITH_WEBHOOKS                    := false
KIND_ENV                                   := "skaffold"
PARALLEL_E2E_TESTS                         := 5
IPV6_SUFFIX                                := ""

ifneq ($(strip $(shell git status --porcelain 2>/dev/null)),)
	EFFECTIVE_VERSION := $(EFFECTIVE_VERSION)-dirty
endif

ifneq ($(USE_IPV6),)
	IPV6_SUFFIX:="-ipv6"
endif

SHELL=/usr/bin/env bash -o pipefail

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include hack/tools.mk

LOGCHECK_DIR := $(TOOLS_DIR)/logcheck
GOMEGACHECK_DIR := $(TOOLS_DIR)/gomegacheck

#########################################
# Rules for local development scenarios #
#########################################

.PHONY: dev-setup
dev-setup:
	@if [ "$(DEV_SETUP_WITH_WEBHOOKS)" = "true" ]; then ./hack/local-development/dev-setup --with-webhooks; else ./hack/local-development/dev-setup; fi

.PHONY: dev-setup-register-gardener
dev-setup-register-gardener:
	@./hack/local-development/dev-setup-register-gardener

.PHONY: local-garden-up
local-garden-up: $(HELM)
	@./hack/local-development/local-garden/start.sh $(LOCAL_GARDEN_LABEL) $(ACTIVATE_SEEDAUTHORIZER)

.PHONY: local-garden-down
local-garden-down:
	@./hack/local-development/local-garden/stop.sh $(LOCAL_GARDEN_LABEL)

.PHONY: remote-garden-up
remote-garden-up: $(HELM)
	@./hack/local-development/remote-garden/start.sh $(REMOTE_GARDEN_LABEL)

.PHONY: remote-garden-down
remote-garden-down:
	@./hack/local-development/remote-garden/stop.sh $(REMOTE_GARDEN_LABEL)

.PHONY: start-apiserver
start-apiserver:
	@./hack/local-development/start-apiserver

.PHONY: start-controller-manager
start-controller-manager:
	@./hack/local-development/start-controller-manager

.PHONY: start-scheduler
start-scheduler:
	@./hack/local-development/start-scheduler

.PHONY: start-admission-controller
start-admission-controller:
	@./hack/local-development/start-admission-controller

.PHONY: start-resource-manager
start-resource-manager:
	@./hack/local-development/start-resource-manager

.PHONY: start-operator
start-operator:
	@./hack/local-development/start-operator

.PHONY: start-gardenlet
start-gardenlet: $(HELM) $(YAML2JSON) $(YQ)
	@./hack/local-development/start-gardenlet

.PHONY: start-extension-provider-local
start-extension-provider-local:
	@./hack/local-development/start-extension-provider-local

#################################################################
# Rules related to binary build, Docker image build and release #
#################################################################

.PHONY: install
install:
	@EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) ./hack/install.sh ./...

.PHONY: docker-images
docker-images:
	@echo "Building docker images with version and tag $(EFFECTIVE_VERSION)"
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(APISERVER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(APISERVER_IMAGE_REPOSITORY):latest                -f Dockerfile --target apiserver .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)       -t $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):latest       -f Dockerfile --target controller-manager .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(SCHEDULER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(SCHEDULER_IMAGE_REPOSITORY):latest                -f Dockerfile --target scheduler .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(ADMISSION_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(ADMISSION_IMAGE_REPOSITORY):latest                -f Dockerfile --target admission-controller .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(RESOURCE_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)         -t $(RESOURCE_MANAGER_IMAGE_REPOSITORY):latest         -f Dockerfile --target resource-manager .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(OPERATOR_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                 -t $(OPERATOR_IMAGE_REPOSITORY):latest                 -f Dockerfile --target operator .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(GARDENLET_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(GARDENLET_IMAGE_REPOSITORY):latest                -f Dockerfile --target gardenlet .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION) -t $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):latest -f Dockerfile --target gardener-extension-provider-local .

.PHONY: docker-images-ppc
docker-images-ppc:
	@echo "Building docker images for IBM's POWER(ppc64le) with version and tag $(EFFECTIVE_VERSION)"
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(APISERVER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(APISERVER_IMAGE_REPOSITORY):latest                -f Dockerfile --target apiserver .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)       -t $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):latest       -f Dockerfile --target controller-manager .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(SCHEDULER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(SCHEDULER_IMAGE_REPOSITORY):latest                -f Dockerfile --target scheduler .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(ADMISSION_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(ADMISSION_IMAGE_REPOSITORY):latest                -f Dockerfile --target admission-controller .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(RESOURCE_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)         -t $(RESOURCE_MANAGER_IMAGE_REPOSITORY):latest         -f Dockerfile --target resource-manager .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(OPERATOR_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                 -t $(OPERATOR_IMAGE_REPOSITORY):latest                 -f Dockerfile --target operator .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(GARDENLET_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)                -t $(GARDENLET_IMAGE_REPOSITORY):latest                -f Dockerfile --target gardenlet .
	@docker build --build-arg EFFECTIVE_VERSION=$(EFFECTIVE_VERSION)  -t $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION) -t $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):latest -f Dockerfile --target gardener-extension-provider-local .

.PHONY: docker-push
docker-push:
	@if ! docker images $(APISERVER_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(APISERVER_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(CONTROLLER_MANAGER_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(CONTROLLER_MANAGER_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(SCHEDULER_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(SCHEDULER_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(ADMISSION_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(ADMISSION_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(RESOURCE_MANAGER_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(RESOURCE_MANAGER_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(GARDENLET_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(GARDENLET_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@docker push $(APISERVER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(APISERVER_IMAGE_REPOSITORY):latest; fi
	@docker push $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(CONTROLLER_MANAGER_IMAGE_REPOSITORY):latest; fi
	@docker push $(SCHEDULER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(SCHEDULER_IMAGE_REPOSITORY):latest; fi
	@docker push $(ADMISSION_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(ADMISSION_IMAGE_REPOSITORY):latest; fi
	@docker push $(RESOURCE_MANAGER_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(RESOURCE_MANAGER_IMAGE_REPOSITORY):latest; fi
	@docker push $(GARDENLET_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(GARDENLET_IMAGE_REPOSITORY):latest; fi
	@docker push $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)
	@if [[ "$(PUSH_LATEST_TAG)" == "true" ]]; then docker push $(EXTENSION_PROVIDER_LOCAL_IMAGE_REPOSITORY):latest; fi

#####################################################################
# Rules for verification, formatting, linting, testing and cleaning #
#####################################################################

.PHONY: revendor
revendor:
	@GO111MODULE=on go mod tidy
	@GO111MODULE=on go mod vendor
	@cd $(LOGCHECK_DIR); go mod tidy
	@cd $(GOMEGACHECK_DIR); go mod tidy

.PHONY: clean
clean:
	@hack/clean.sh ./cmd/... ./extensions/... ./pkg/... ./plugin/... ./test/...

.PHONY: check-generate
check-generate:
	@hack/check-generate.sh $(REPO_ROOT)

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT) $(HELM) $(IMPORT_BOSS) $(LOGCHECK) $(GOMEGACHECK) $(YQ)
	@hack/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/... ./extensions/... ./pkg/... ./plugin/... ./test/...
	@hack/check-imports.sh ./charts/... ./cmd/... ./extensions/... ./pkg/... ./plugin/... ./test/... ./third_party/...

	@echo "> Check $(LOGCHECK_DIR)"
	@cd $(LOGCHECK_DIR); $(abspath $(GOLANGCI_LINT)) run -c $(REPO_ROOT)/.golangci.yaml --timeout 10m ./...
	@cd $(LOGCHECK_DIR); go vet ./...
	@cd $(LOGCHECK_DIR); $(abspath $(GOIMPORTS)) -l .

	@echo "> Check $(GOMEGACHECK_DIR)"
	@cd $(GOMEGACHECK_DIR); $(abspath $(GOLANGCI_LINT)) run -c $(REPO_ROOT)/.golangci.yaml --timeout 10m ./...
	@cd $(GOMEGACHECK_DIR); go vet ./...
	@cd $(GOMEGACHECK_DIR); $(abspath $(GOIMPORTS)) -l .

	@hack/check-charts.sh ./charts
	@hack/check-skaffold-deps.sh

.PHONY: generate
generate: $(CONTROLLER_GEN) $(GEN_CRD_API_REFERENCE_DOCS) $(GOIMPORTS) $(GO_TO_PROTOBUF) $(HELM) $(MOCKGEN) $(OPENAPI_GEN) $(PROTOC_GEN_GOGO) $(YAML2JSON)
	@hack/update-protobuf.sh
	@hack/update-codegen.sh
	@hack/generate-parallel.sh charts cmd example extensions pkg plugin test
	@cd $(LOGCHECK_DIR); go generate ./...
	@cd $(GOMEGACHECK_DIR); go generate ./...
	@hack/generate-monitoring-docs.sh
	$(MAKE) format

.PHONY: generate-sequential
generate-sequential: $(CONTROLLER_GEN) $(GEN_CRD_API_REFERENCE_DOCS) $(GOIMPORTS) $(GO_TO_PROTOBUF) $(HELM) $(MOCKGEN) $(OPENAPI_GEN) $(PROTOC_GEN_GOGO) $(YAML2JSON)
	@hack/update-protobuf.sh
	@hack/update-codegen.sh
	@hack/generate.sh ./charts/... ./cmd/... ./example/... ./extensions/... ./pkg/... ./plugin/... ./test/...
	@cd $(LOGCHECK_DIR); go generate ./...
	@cd $(GOMEGACHECK_DIR); go generate ./...
	@hack/generate-monitoring-docs.sh
	$(MAKE) format

.PHONY: format
format: $(GOIMPORTS)
	@./hack/format.sh ./cmd ./extensions ./pkg ./plugin ./test ./hack
	@cd $(LOGCHECK_DIR); $(abspath $(GOIMPORTS)) -l -w .
	@cd $(GOMEGACHECK_DIR); $(abspath $(GOIMPORTS)) -l -w .

.PHONY: test
test: $(REPORT_COLLECTOR) $(PROMTOOL)
	@./hack/test.sh ./cmd/... ./extensions/pkg/... ./pkg/... ./plugin/...
	@cd $(LOGCHECK_DIR); go test -race -timeout=2m ./... | grep -v 'no test files'
	@cd $(GOMEGACHECK_DIR); go test -race -timeout=2m ./... | grep -v 'no test files'

.PHONY: test-integration
test-integration: $(REPORT_COLLECTOR) $(SETUP_ENVTEST)
	@./hack/test-integration.sh ./test/integration/...

.PHONY: test-cov
test-cov: $(PROMTOOL)
	@./hack/test-cover.sh ./cmd/... ./extensions/pkg/... ./pkg/... ./plugin/...

.PHONY: test-cov-clean
test-cov-clean:
	@./hack/test-cover-clean.sh

.PHONY: check-apidiff
check-apidiff: $(GO_APIDIFF)
	@./hack/check-apidiff.sh

.PHONY: check-vulnerabilities
check-vulnerabilities: $(GO_VULN_CHECK)
	$(GO_VULN_CHECK) ./...

.PHONY: test-prometheus
test-prometheus: $(PROMTOOL)
	@./hack/test-prometheus.sh

.PHONY: check-docforge
check-docforge: $(DOCFORGE)
	@./hack/check-docforge.sh

.PHONY: verify
verify: check format test test-integration test-prometheus

.PHONY: verify-extended
verify-extended: check-generate check format test-cov test-cov-clean test-integration test-prometheus

#####################################################################
# Rules for local environment                                       #
#####################################################################

kind-up kind-down gardener-up gardener-down register-local-env tear-down-local-env register-kind2-env tear-down-kind2-env test-e2e-local-simple test-e2e-local-migration test-e2e-local: export KUBECONFIG = $(GARDENER_LOCAL_KUBECONFIG)
kind2-up kind2-down gardenlet-kind2-up gardenlet-kind2-down: export KUBECONFIG = $(GARDENER_LOCAL2_KUBECONFIG)
kind-ha-single-zone-up kind-ha-single-zone-down gardener-ha-single-zone-up register-kind-ha-single-zone-env tear-down-kind-ha-single-zone-env ci-e2e-kind-ha-single-zone: export KUBECONFIG = $(GARDENER_LOCAL_HA_SINGLE_ZONE_KUBECONFIG)
kind-ha-multi-zone-up kind-ha-multi-zone-down gardener-ha-multi-zone-up register-kind-ha-multi-zone-env tear-down-kind-ha-multi-zone-env ci-e2e-kind-ha-multi-zone: export KUBECONFIG = $(GARDENER_LOCAL_HA_MULTI_ZONE_KUBECONFIG)
kind-operator-up kind-operator-down operator-up operator-down test-e2e-local-operator ci-e2e-kind-operator: export KUBECONFIG = $(GARDENER_LOCAL_OPERATOR_KUBECONFIG)

kind-up: $(KIND) $(KUBECTL) $(HELM)
	./hack/kind-up.sh --cluster-name gardener-local --environment $(KIND_ENV) --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind/base/kubeconfig --path-cluster-values $(REPO_ROOT)/example/gardener-local/kind/local/values.yaml
kind-down: $(KIND)
	./hack/kind-down.sh --cluster-name gardener-local --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind/base/kubeconfig

kind2-up: $(KIND) $(KUBECTL) $(HELM)
	./hack/kind-up.sh --cluster-name gardener-local2 --environment $(KIND_ENV) --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind2/base/kubeconfig --path-cluster-values $(REPO_ROOT)/example/gardener-local/kind/local2/values.yaml --skip-registry
kind2-down: $(KIND)
	./hack/kind-down.sh --cluster-name gardener-local2 --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind2/base/kubeconfig --keep-backupbuckets-dir

kind-ha-single-zone-up: $(KIND) $(KUBECTL) $(HELM)
	./hack/kind-up.sh --cluster-name gardener-local-ha-single-zone --environment $(KIND_ENV) --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind-ha-single-zone/base/kubeconfig --path-cluster-values $(REPO_ROOT)/example/gardener-local/kind/ha-single-zone/values.yaml
kind-ha-single-zone-down: $(KIND)
	./hack/kind-down.sh --cluster-name gardener-local-ha-single-zone --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind-ha-single-zone/base/kubeconfig

kind-ha-multi-zone-up: $(KIND) $(KUBECTL) $(HELM)
	./hack/kind-up.sh --cluster-name gardener-local-ha-multi-zone --environment $(KIND_ENV) --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind-ha-multi-zone/base/kubeconfig --path-cluster-values $(REPO_ROOT)/example/gardener-local/kind/ha-multi-zone/values.yaml
kind-ha-multi-zone-down: $(KIND)
	./hack/kind-down.sh --cluster-name gardener-local-ha-multi-zone --path-kubeconfig $(REPO_ROOT)/example/provider-local/seed-kind-ha-multi-zone/base/kubeconfig

kind-operator-up: $(KIND) $(KUBECTL) $(HELM)
	./hack/kind-up.sh --cluster-name gardener-operator-local --environment $(KIND_ENV) --path-kubeconfig $(REPO_ROOT)/example/gardener-local/kind/operator/kubeconfig --path-cluster-values $(REPO_ROOT)/example/gardener-local/kind/operator/values.yaml
	mkdir -p $(REPO_ROOT)/dev/local-backupbuckets/gardener-operator
kind-operator-down: $(KIND)
	./hack/kind-down.sh --cluster-name gardener-operator-local --path-kubeconfig $(REPO_ROOT)/example/gardener-local/kind/operator/kubeconfig
	# We need root privileges to clean the backup bucket directory, see https://github.com/gardener/gardener/issues/6752
	docker run --user root:root -v $(REPO_ROOT)/dev/local-backupbuckets:/dev/local-backupbuckets alpine rm -rf /dev/local-backupbuckets/gardener-operator

# speed-up skaffold deployments by building all images concurrently
export SKAFFOLD_BUILD_CONCURRENCY = 0
# use static label for skaffold to prevent rolling all gardener components on every `skaffold` invocation
gardener-up gardener-down gardener-ha-single-zone-up gardener-ha-single-zone-down gardener-ha-multi-zone-up gardener-ha-multi-zone-down gardenlet-kind2-up gardenlet-kind2-down: export SKAFFOLD_LABEL = skaffold.dev/run-id=gardener-local
# set ldflags for skaffold
gardener-up gardener-ha-single-zone-up gardener-ha-multi-zone-up gardenlet-kind2-up operator-up: export LD_FLAGS = $(shell $(REPO_ROOT)/hack/get-build-ld-flags.sh)

gardener-up: $(SKAFFOLD) $(HELM) $(KUBECTL)
	SKAFFOLD_DEFAULT_REPO=localhost:5001 SKAFFOLD_PUSH=true $(SKAFFOLD) run
gardener-down: $(SKAFFOLD) $(HELM) $(KUBECTL)
	./hack/gardener-down.sh
register-local-env: $(KUBECTL)
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/garden/local
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/seed-kind/local
tear-down-local-env: $(KUBECTL)
	$(KUBECTL) annotate project local confirmation.gardener.cloud/deletion=true
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/seed-kind/local
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/garden/local

gardenlet-kind2-up: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) deploy -m kind2-env -p kind2 --kubeconfig=$(GARDENER_LOCAL_KUBECONFIG)
	@# define GARDENER_LOCAL_KUBECONFIG so that it can be used by skaffold when checking whether the seed managed by this gardenlet is ready
	GARDENER_LOCAL_KUBECONFIG=$(GARDENER_LOCAL_KUBECONFIG) SKAFFOLD_DEFAULT_REPO=localhost:5001 SKAFFOLD_PUSH=true $(SKAFFOLD) run -m provider-local,gardenlet -p kind2
gardenlet-kind2-down: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) delete -m kind2-env -p kind2 --kubeconfig=$(GARDENER_LOCAL_KUBECONFIG)
	$(SKAFFOLD) delete -m gardenlet,kind2-env -p kind2
register-kind2-env: $(KUBECTL)
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/seed-kind2/local
tear-down-kind2-env: $(KUBECTL)
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/seed-kind2/local

gardener-ha-single-zone-up: $(SKAFFOLD) $(HELM) $(KUBECTL)
	SKAFFOLD_DEFAULT_REPO=localhost:5001 SKAFFOLD_PUSH=true $(SKAFFOLD) run -p ha-single-zone
gardener-ha-single-zone-down: $(SKAFFOLD) $(HELM) $(KUBECTL)
	./hack/gardener-down.sh --skaffold-profile ha-single-zone
register-kind-ha-single-zone-env: $(KUBECTL)
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/garden/local
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/seed-kind-ha-single-zone/local
tear-down-kind-ha-single-zone-env: $(KUBECTL)
	$(KUBECTL) annotate project local confirmation.gardener.cloud/deletion=true
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/seed-kind-ha-single-zone/local
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/garden/local

gardener-ha-multi-zone-up: $(SKAFFOLD) $(HELM) $(KUBECTL)
	SKAFFOLD_DEFAULT_REPO=localhost:5001 SKAFFOLD_PUSH=true $(SKAFFOLD) run -p ha-multi-zone
gardener-ha-multi-zone-down: $(SKAFFOLD) $(HELM) $(KUBECTL)
	./hack/gardener-down.sh --skaffold-profile ha-multi-zone
register-kind-ha-multi-zone-env: $(KUBECTL)
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/garden/local
	$(KUBECTL) apply -k $(REPO_ROOT)/example/provider-local/seed-kind-ha-multi-zone/local
tear-down-kind-ha-multi-zone-env: $(KUBECTL)
	$(KUBECTL) annotate project local confirmation.gardener.cloud/deletion=true
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/seed-kind-ha-multi-zone/local
	$(KUBECTL) delete -k $(REPO_ROOT)/example/provider-local/garden/local

operator-up: $(SKAFFOLD) $(HELM) $(KUBECTL)
	SKAFFOLD_DEFAULT_REPO=localhost:5001 SKAFFOLD_PUSH=true $(SKAFFOLD) run -f skaffold-operator.yaml
operator-down: $(SKAFFOLD) $(HELM) $(KUBECTL)
	$(KUBECTL) delete garden --all --ignore-not-found --wait --timeout 5m
	$(SKAFFOLD) delete -f skaffold-operator.yaml

test-e2e-local: $(GINKGO)
	./hack/test-e2e-local.sh --procs=$(PARALLEL_E2E_TESTS) --label-filter="default" ./test/e2e/gardener/...
test-e2e-local-simple: $(GINKGO)
	./hack/test-e2e-local.sh --procs=$(PARALLEL_E2E_TESTS) --label-filter "Shoot && simple" ./test/e2e/gardener/...
test-e2e-local-migration: $(GINKGO)
	./hack/test-e2e-local.sh --procs=$(PARALLEL_E2E_TESTS) --label-filter "Shoot && control-plane-migration" ./test/e2e/gardener/...
test-e2e-local-ha-single-zone: $(GINKGO)
	SHOOT_FAILURE_TOLERANCE_TYPE=node ./hack/test-e2e-local.sh --procs=$(PARALLEL_E2E_TESTS) --label-filter "simple || (high-availability && upgrade-to-node)" ./test/e2e/gardener/...
test-e2e-local-ha-multi-zone: $(GINKGO)
	SHOOT_FAILURE_TOLERANCE_TYPE=zone ./hack/test-e2e-local.sh --procs=$(PARALLEL_E2E_TESTS) --label-filter "simple || (high-availability && upgrade-to-zone)" ./test/e2e/gardener/...
test-e2e-local-operator: $(GINKGO)
	./hack/test-e2e-local.sh operator --procs=$(PARALLEL_E2E_TESTS) --label-filter="default" ./test/e2e/operator/...

ci-e2e-kind: $(KIND) $(YQ)
	./hack/ci-e2e-kind.sh
ci-e2e-kind-migration: $(KIND) $(YQ)
	./hack/ci-e2e-kind-migration.sh
ci-e2e-kind-ha-single-zone: $(KIND) $(YQ)
	./hack/ci-e2e-kind-ha-single-zone.sh
ci-e2e-kind-ha-multi-zone: $(KIND) $(YQ)
	./hack/ci-e2e-kind-ha-multi-zone.sh
ci-e2e-kind-operator: $(KIND) $(YQ)
	./hack/ci-e2e-kind-operator.sh

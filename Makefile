DOCKER_IMAGE := landlock-genprof-dev
PLUGIN_BIN := kubectl-landlock_genprof
INSTALL_DIR ?= $$(go env GOPATH)/bin
INSTALL_PATH := $(INSTALL_DIR)/$(PLUGIN_BIN)
INSTALL_MANIFEST := $(INSTALL_DIR)/.landlock-genprof-install
NS ?= default
PROPOSAL ?=
OUT_DIR ?= out/$(PROPOSAL)

# Injected into cmd/landlock-genprof's version/commit/date vars — falls
# back to "dev"/"none"/"unknown" (their zero-value defaults) outside a
# git checkout, e.g. a tarball build. --tags --always so an untagged
# checkout still gets a commit-based version instead of erroring.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

.PHONY: help init-vm bootstrap env-doctor test-env test-env-clean check-kernel ui-lima ui-lima-auth ui-lima-demo operations-center-demo operations-center-demo-test operations-center-demo-reset ui-lima-auth-test ui-lima-auth-release published-release-harness-test published-trusted-proxy-fixture-test published-rbac-ownership-test operations-center-frontend-build build test vet fmt docs-cli build-plugin install-plugin install uninstall verify-install check-public-assets docker-build docker-test docker-shell export-proposal apply-proposal demo-proposal demo-nginx apply-nginx envtest envtest-diagnostics test-all

help: ## List commands grouped by side effect and purpose
	@awk 'BEGIN { FS = ":.*## "; order[1]="Installation"; order[2]="Development environment"; order[3]="Tests and quality"; order[4]="Documentation and generation"; order[5]="UI and demos"; order[6]="Proposal operations"; order[7]="Other" } /^[a-zA-Z_-]+:.*## / { target=$$1; group="Other"; if (target ~ /^(install|uninstall|verify-install|build-plugin|install-plugin)$$/) group="Installation"; else if (target ~ /^(init-vm|bootstrap|dev-bootstrap|dev-doctor|env-doctor|dev-down|test-env|test-env-clean|check-kernel)$$/) group="Development environment"; else if (target ~ /^(build|test|test-unit|test-envtest|test-integration|test-e2e|test-security|test-all|envtest|envtest-diagnostics|vet|fmt|lint|docker-build|docker-test|e2e-)/) group="Tests and quality"; else if (target ~ /^(docs-cli|docs-build|generate|check-public-assets)$$/) group="Documentation and generation"; else if (target ~ /^(ui-|operations-center|published-)/) group="UI and demos"; else if (target ~ /^(export-proposal|apply-proposal|demo-|apply-nginx)$$/) group="Proposal operations"; text[group] = text[group] sprintf("%-24s %s\n", target, $$2); } END { for (i=1; i<=7; i++) { group=order[i]; if (text[group] != "") { printf "\n[%s]\n%s", group, text[group] } } }' $(MAKEFILE_LIST)

init-vm: ## Deprecated compatibility wrapper for the Core bootstrap
	./hack/init-vm.sh

bootstrap: ## Create the contributor Core kind+Cilium platform (Linux or macOS/Lima)
	./hack/bootstrap.sh --lane core

dev-bootstrap: bootstrap ## Compatibility alias for the contributor platform bootstrap

dev-doctor: env-doctor ## Compatibility alias for environment diagnostics

dev-down: test-env-clean ## Remove only the owned project layer; preserve the platform

env-doctor: ## Diagnose host, runtime, Core topology, and project-environment readiness
	./hack/env-doctor.sh

ui-lima: ## Validate macOS/Lima Core and launch the local Operations Center UI
	./hack/ui-lima.sh

ui-lima-auth: ## Reproducible production-like trusted-proxy authenticated UI qualification (disposable HMAC/proxy fixture; TEST FIXTURE, not a production proxy)
	./hack/ui-lima-auth.sh

ui-lima-demo: ## Interactive source-mode Operations Center demo; remains alive until Ctrl-C
	./hack/ui-lima-demo.sh

operations-center-demo: ## Bootstrap the disposable multi-context Operations Center demo
	./hack/operations-center-demo.sh

operations-center-demo-test: ## Verify the Operations Center demo URL manifest contract
	./hack/operations-center-demo-test.sh

operations-center-demo-reset: ## Remove only the owned Operations Center demo fixtures
	./hack/operations-center-demo-reset.sh

ui-lima-auth-test: ## Automated source-mode UI smoke with a disposable real workload and browser (does not publish)
	./hack/ui-lima-functional.sh

operations-center-frontend-build: ## Build the additive React Operations Center migration frontend
	cd web/operations-center && npm ci && npm run build

ui-lima-auth-release: ## Published-artifact-only Lima/IHM qualification (requires RELEASE_VERSION=vX.Y.Z; no local fallback)
	@test -n "$(RELEASE_VERSION)" || (echo "RELEASE_VERSION is required (example: make ui-lima-auth-release RELEASE_VERSION=v0.8.1)"; exit 2)
	RELEASE_VERSION="$(RELEASE_VERSION)" PUBLISHED_RELEASE_VALIDATE_ONLY="$(PUBLISHED_RELEASE_VALIDATE_ONLY)" PUBLISHED_PROXY_NAMESPACE="$(PUBLISHED_PROXY_NAMESPACE)" PUBLISHED_PROXY_SELECTOR="$(PUBLISHED_PROXY_SELECTOR)" PUBLISHED_PROXY_URL="$(PUBLISHED_PROXY_URL)" PUBLISHED_PROXY_IMAGE="$(PUBLISHED_PROXY_IMAGE)" ./hack/ui-lima-auth-release.sh

published-release-harness-test: ## Non-publishing tests for published-release reference and fallback guards
	./hack/published-release-harness-test.sh

published-trusted-proxy-fixture-test: ## Validate the disposable in-cluster trusted-proxy fixture
	./hack/published-trusted-proxy-fixture-test.sh

published-rbac-ownership-test: ## Validate Helm external/shared RBAC ownership mode
	./hack/published-rbac-ownership-test.sh

test-env: ## Install the project Core CRDs/RBAC and Inspektor Gadget (SPO/PodLock remain optional)
	./hack/test-env.sh

test-env-clean: ## Remove only owned project-layer resources; preserve cluster, VM, and host tools
	./hack/test-env-clean.sh

check-kernel: ## Vérifie que le kernel hôte supporte Landlock et eBPF
	./hack/check-kernel.sh

build: ## go build tracked source packages — macOS/Windows use the tracer stub
	@packages="$$(go list ./... | grep -v '/book/dist/')"; test -n "$$packages"; go build $$packages

UNIT_DIAGNOSTIC_TESTS := ^(TestObservationContributionEnvtestE1ToE7|TestReceiptConcurrencySameKeyConvergesOnOneEffect|TestContainerContributionCrashRecoveryMatrix|TestObservationAdapterConcurrentDifferentObservationsAccumulate)$$

test-unit: ## Run authoritative unit/package tests without generated book/dist packages
	@packages="$$(go list ./... | grep -v '/book/dist/')"; test -n "$$packages"; go test -cover -skip '$(UNIT_DIAGNOSTIC_TESTS)' $$packages

test: test-unit ## Compatibility alias for the unit test suite

vet: ## go vet tracked source packages
	@packages="$$(go list ./... | grep -v '/book/dist/')"; test -n "$$packages"; go vet $$packages

KNOWN_DIAGNOSTIC_TESTS := ^(TestObservationContributionEnvtestE1ToE7|TestReceiptConcurrencySameKeyConvergesOnOneEffect|TestContainerContributionCrashRecoveryMatrix|TestObservationAdapterConcurrentDifferentObservationsAccumulate)$$

envtest: ## Run authoritative envtest suite (known diagnostics are explicit below)
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use -p path 1.36.2)" \
	    go test -tags=envtest -count=1 -skip '$(KNOWN_DIAGNOSTIC_TESTS)' ./internal/proposal/... ./internal/history/... ./internal/attempt/... ./internal/observation/kubernetes/... ./internal/authz/...
	@# Workbench certification: production binary, real proposal, real loopback
	@# listener, real HTTP. -run keeps this to the E2E cases; the package's
	@# unit tests already run untagged in `make test`.
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use -p path 1.36.2)" \
	    go test -tags=envtest -count=1 -run 'TestWorkbenchE2E' ./cmd/landlock-genprof/...

test-envtest: envtest ## Run authoritative API-server semantics tests

test-integration: test-envtest ## Compatibility alias for API-server integration tests

test-e2e: test-e2e-core ## Run the live Kubernetes E2E suite (requires prepared infrastructure)

test-security: ## Run local SAST; does not install scanners automatically
	@command -v gosec >/dev/null 2>&1 || { echo "gosec is required; install it separately before make test-security" >&2; exit 2; }
	@gosec ./...

lint: fmt vet ## Run formatting and static analysis checks

docs-build: ## Build the mdBook documentation
	mdbook build book

generate: docs-cli ## Regenerate generated CLI reference documentation

envtest-diagnostics: ## Run only the explicitly accepted non-authoritative diagnostics
	./hack/run-diagnostics.sh

test-all: test envtest ## Run all tests (unit + envtest)

fmt: ## Vérifie le formatage (gofmt -l) sans rien modifier
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Fichiers non formatés :"; echo "$$unformatted"; exit 1; \
	fi

docs-cli: ## Régénère book/src/cli/ (référence CLI) depuis les commandes cobra réelles — non versionné (voir .gitignore), à refaire avant `mdbook serve`/`mdbook build` en local
	go run -tags gendocs ./cmd/landlock-genprof book/src/cli

build-plugin: ## Build le binaire nommé kubectl-landlock_genprof, avec version/commit/date réels injectés (voir `landlock-genprof version`) — kubectl transforme le "_" du nom de fichier en "-" dans la commande, d'où kubectl-landlock_genprof -> `kubectl landlock-genprof ...` (un tiret littéral dans kubectl-landlock-genprof serait lu comme deux sous-commandes séparées, "landlock genprof")
	go build -ldflags "$(LDFLAGS)" -o $(PLUGIN_BIN) ./cmd/landlock-genprof

install-plugin: build-plugin ## build-plugin + installe dans $$(go env GOPATH)/bin (doit être dans le PATH pour que kubectl le détecte, voir `kubectl plugin list`)
	mkdir -p "$$(go env GOPATH)/bin"
	mv $(PLUGIN_BIN) "$$(go env GOPATH)/bin/$(PLUGIN_BIN)"

install: build-plugin ## Install the kubectl plugin in the user-owned Go bin directory
	@set -eu; \
	dir="$(INSTALL_DIR)"; path="$(INSTALL_PATH)"; manifest="$(INSTALL_MANIFEST)"; \
	mkdir -p "$$dir"; \
	if [ -e "$$path" ] && [ "$${FORCE:-0}" != 1 ]; then \
		echo "Refusing to overwrite existing $$path; use FORCE=1 only after verifying it is yours." >&2; exit 2; \
	fi; \
	install -m 0755 "$(PLUGIN_BIN)" "$$path"; \
	sha="$$(shasum -a 256 "$$path" | awk '{print $$1}')"; \
	{ printf 'path=%s\n' "$$path"; printf 'sha256=%s\n' "$$sha"; } > "$$manifest"; \
	echo "Installed $$path"; \
	case ":$${PATH:-}:" in *:"$$dir":*) ;; *) echo "PATH_MISSING: add $$dir to PATH before invoking kubectl landlock-genprof" >&2 ;; esac

uninstall: ## Remove only a plugin installed by this target
	@set -eu; \
	path="$(INSTALL_PATH)"; manifest="$(INSTALL_MANIFEST)"; \
	if [ ! -f "$$manifest" ]; then echo "No managed installation found at $$path"; exit 0; fi; \
	managed_path="$$(sed -n 's/^path=//p' "$$manifest")"; expected="$$(sed -n 's/^sha256=//p' "$$manifest")"; \
	[ "$$managed_path" = "$$path" ] || { echo "Refusing to remove unexpected path $$managed_path" >&2; exit 2; }; \
	[ -f "$$path" ] || { echo "Managed binary is already absent"; rm -f "$$manifest"; exit 0; }; \
	actual="$$(shasum -a 256 "$$path" | awk '{print $$1}')"; \
	[ "$$actual" = "$$expected" ] || { echo "Refusing to remove modified $$path" >&2; exit 2; }; \
	rm -f "$$path" "$$manifest"; echo "Removed managed installation $$path"

verify-install: ## Verify the managed plugin, command discovery and PATH
	@set -eu; \
	path="$(INSTALL_PATH)"; dir="$(INSTALL_DIR)"; \
	[ -x "$$path" ] || { echo "INSTALL_MISSING: run make install INSTALL_DIR=$$dir" >&2; exit 3; }; \
	"$$path" version; "$$path" --help >/dev/null; \
	case ":$${PATH:-}:" in *:"$$dir":*) ;; *) echo "PATH_MISSING: $$dir is not on PATH" >&2; exit 4 ;; esac; \
	if command -v kubectl >/dev/null 2>&1; then kubectl landlock-genprof --help >/dev/null || { echo "KUBECTL_PLUGIN_FAILED" >&2; exit 5; }; else echo "KUBECTL_NOT_FOUND: direct plugin checks passed" >&2; fi

check-public-assets: ## Verify README release-download links and public HTTP reachability
	@./hack/check-public-assets.sh

docker-build: ## Construit l'image Dockerfile.dev (build/test Linux réel, y compris internal/tracer, sans la VM)
	docker build -f Dockerfile.dev -t $(DOCKER_IMAGE) .

docker-test: docker-build ## go build + go test dans le conteneur Linux (équivalent CI, sans cluster réel)
	docker run --rm $(DOCKER_IMAGE) sh -c "go build ./... && go vet ./... && go test -cover ./..."

docker-shell: docker-build ## Shell interactif dans le conteneur de dev
	docker run --rm -it $(DOCKER_IMAGE) bash

export-proposal: ## Exporte les artefacts d'une SecurityProfileProposal vers OUT_DIR (debug/information uniquement; non authoritative) (usage: make export-proposal PROPOSAL=<nom> [NS=default] [OUT_DIR=out/<nom>])
	@test -n "$(PROPOSAL)" || (echo "PROPOSAL est requis (ex: make export-proposal PROPOSAL=nginx-demo)"; exit 1)
	@mkdir -p "$(OUT_DIR)"
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.podLock}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/profile.yaml"
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.networkPolicy}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/networkpolicy.yaml"
	@if [ ! -s "$(OUT_DIR)/networkpolicy.yaml" ]; then rm -f "$(OUT_DIR)/networkpolicy.yaml"; fi
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.patchedManifest}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/patched.yaml"
	@if [ ! -s "$(OUT_DIR)/patched.yaml" ]; then rm -f "$(OUT_DIR)/patched.yaml"; fi
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.spoSeccompProfile}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/seccompprofile.yaml"
	@if [ ! -s "$(OUT_DIR)/seccompprofile.yaml" ]; then rm -f "$(OUT_DIR)/seccompprofile.yaml"; fi
	@echo "Artifacts exported to $(OUT_DIR) for inspection only."
	@echo "WARNING: Exported files are non-authoritative snapshots of mutable proposal.spec."
	@echo "Do NOT apply them for governed rollout. Use: kubectl landlock-genprof apply-proposal $(PROPOSAL) -n $(NS)"

apply-proposal: ## Applique une proposal via le chemin autoritatif (approval-bound)
	@test -n "$(PROPOSAL)" || (echo "PROPOSAL est requis (ex: make apply-proposal PROPOSAL=nginx-demo)"; exit 1)
	@kubectl landlock-genprof apply-proposal "$(PROPOSAL)" -n "$(NS)" --yes

demo-proposal: export-proposal ## Prepare la demo proposal-first: exporte, liste les artefacts, puis montre le label PodLock du manifest patché si present
	@echo "Artefacts de demo dans $(OUT_DIR):"
	@ls -1 "$(OUT_DIR)"
	@if [ -f "$(OUT_DIR)/patched.yaml" ]; then \
		echo; \
		echo "Label PodLock dans patched.yaml:"; \
		grep -n 'podlock.kubewarden.io/profile' "$(OUT_DIR)/patched.yaml" || true; \
	fi
	@echo
	@echo "Pour appliquer la proposal: make apply-proposal PROPOSAL=$(PROPOSAL) NS=$(NS) OUT_DIR=$(OUT_DIR)"

demo-nginx: ## Raccourci demo proposal-first pour nginx-demo/default
	@$(MAKE) demo-proposal PROPOSAL=nginx-demo NS=default OUT_DIR=out/nginx-demo

apply-nginx: ## Raccourci d'application de la proposal nginx-demo/default
	@$(MAKE) apply-proposal PROPOSAL=nginx-demo NS=default OUT_DIR=out/nginx-demo

# E2E infra targets
.PHONY: e2e-cluster-create e2e-install e2e-preflight e2e-golden e2e-cluster-destroy

e2e-cluster-create: ## Create a disposable kind cluster for E2E
	@bash -n test/e2e/cluster-create.sh >/dev/null 2>&1 || true
	@bash test/e2e/cluster-create.sh

e2e-install: ## Install CRDs and Inspektor Gadget into the E2E cluster
	@bash test/e2e/install-crds.sh
	@bash test/e2e/install-gadget.sh

e2e-preflight: ## Perform non-mutating preflight checks for the E2E environment
	@bash test/e2e/preflight.sh

# Note: e2e-golden runs the wrapper; the actual mutating demo requires manual consent
e2e-golden: ## Wrapper to run Golden E2E (must be run against kind-landlock-genprof-e2e context)
	@bash test/e2e/e2e-golden.sh

e2e-cluster-destroy: ## Destroy the disposable kind cluster
	@bash test/e2e/cluster-destroy.sh

.PHONY: test-e2e-core
# test-e2e-core: run smoke checks and the full Golden E2E (expects cluster + deps installed)
test-e2e-core: ## Run CORE E2E tests (smoke tracer, smoke networkpolicy, then Golden E2E)
	@bash -n test/e2e/smoke-tracer.sh >/dev/null 2>&1 || true
	@bash -n test/e2e/smoke-networkpolicy.sh >/dev/null 2>&1 || true
	@command -v kubectl >/dev/null 2>&1 || (echo "kubectl not found"; exit 2)
	@PLUGIN_PATH="$$(command -v kubectl-landlock_genprof || true)"; \
		[ -n "$$PLUGIN_PATH" ] || (echo "kubectl-landlock_genprof plugin not found in PATH"; exit 2); \
		echo "[check] plugin path=$$PLUGIN_PATH"
	@PATH_CLEAN="$$(printf '%s' "$$PATH" | tr ':' '\n' | awk 'NF && !seen[$$0]++ { print }' | while read -r p; do [ -d "$$p" ] && printf '%s:' "$$p"; done | sed 's/:$$//')"; \
		echo "[diag] kubectl plugin list"; \
		set +e; PATH="$$PATH_CLEAN" kubectl plugin list >/dev/null; rc=$$?; set -e; \
		if [ "$$rc" -ne 0 ]; then \
			echo "[diag] kubectl plugin list returned rc=$$rc; continuing because canonical plugin execution is checked separately"; \
		fi
	@kubectl landlock-genprof --help >/dev/null || (echo "kubectl landlock-genprof --help failed"; exit 2)
	@echo "[check] kubectl landlock-genprof --help: OK"
	@echo "Running smoke tracer"
	@bash test/e2e/smoke-tracer.sh
	@echo "Running smoke networkpolicy"
	@bash test/e2e/smoke-networkpolicy.sh
	@echo "Running Golden E2E (3-run)"
	@bash hack/demo-golden.sh

DOCKER_IMAGE := landlock-genprof-dev
PLUGIN_BIN := kubectl-landlock_genprof
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

.PHONY: help init-vm check-kernel build test vet fmt docs-cli build-plugin install-plugin docker-build docker-test docker-shell export-proposal apply-proposal demo-proposal demo-nginx apply-nginx envtest test-all

help: ## Liste les commandes disponibles
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "%-15s %s\n", $$1, $$2}'

init-vm: ## Installe kind/kubectl/Inspektor Gadget et déploie le pod de test (voir COMMENT_COMMENCER.md §2)
	./hack/init-vm.sh

check-kernel: ## Vérifie que le kernel hôte supporte Landlock et eBPF
	./hack/check-kernel.sh

build: ## go build ./... — sur macOS/Windows, internal/tracer.Trace() compile en stub (voir docs/architecture.md §3)
	go build ./...

test: ## go test avec couverture (informatif, pas de seuil bloquant)
	go test -cover ./...

vet: ## go vet ./...
	go vet ./...

envtest: ## Run envtest suite (CRD semantics + Workbench E2E, against a real API server)
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use -p path 1.36.2)" \
	    go test -tags=envtest -count=1 ./internal/proposal/... ./internal/history/...
	@# Workbench certification: production binary, real proposal, real loopback
	@# listener, real HTTP. -run keeps this to the E2E cases; the package's
	@# unit tests already run untagged in `make test`.
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use -p path 1.36.2)" \
	    go test -tags=envtest -count=1 -run 'TestWorkbenchE2E' ./cmd/landlock-genprof/...

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

DOCKER_IMAGE := landlock-genprof-dev
PLUGIN_BIN := kubectl-landlock-genprof
NS ?= default
PROPOSAL ?=
OUT_DIR ?= out/$(PROPOSAL)

.PHONY: help init-vm check-kernel build test vet fmt build-plugin install-plugin docker-build docker-test docker-shell export-proposal apply-proposal demo-proposal demo-nginx apply-nginx

help: ## Liste les commandes disponibles
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "%-15s %s\n", $$1, $$2}'

init-vm: ## Installe kind/kubectl/Inspektor Gadget et déploie le pod de test (voir HOW_TO_START.md §2)
	./hack/init-vm.sh

check-kernel: ## Vérifie que le kernel hôte supporte Landlock et eBPF
	./hack/check-kernel.sh

build: ## go build ./... — sur macOS/Windows, internal/tracer.Trace() compile en stub (voir docs/architecture.md §3)
	go build ./...

test: ## go test avec couverture (informatif, pas de seuil bloquant)
	go test -cover ./...

vet: ## go vet ./...
	go vet ./...

fmt: ## Vérifie le formatage (gofmt -l) sans rien modifier
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Fichiers non formatés :"; echo "$$unformatted"; exit 1; \
	fi

build-plugin: ## Build le binaire nommé kubectl-landlock-genprof — un plugin kubectl n'est qu'un exécutable kubectl-<nom> dans le PATH, invocable via `kubectl landlock-genprof ...`
	go build -o $(PLUGIN_BIN) ./cmd/landlock-genprof

install-plugin: build-plugin ## build-plugin + installe dans $$(go env GOPATH)/bin (doit être dans le PATH pour que kubectl le détecte, voir `kubectl plugin list`)
	mkdir -p "$$(go env GOPATH)/bin"
	mv $(PLUGIN_BIN) "$$(go env GOPATH)/bin/$(PLUGIN_BIN)"

docker-build: ## Construit l'image Dockerfile.dev (build/test Linux réel, y compris internal/tracer, sans la VM)
	docker build -f Dockerfile.dev -t $(DOCKER_IMAGE) .

docker-test: docker-build ## go build + go test dans le conteneur Linux (équivalent CI, sans cluster réel)
	docker run --rm $(DOCKER_IMAGE) sh -c "go build ./... && go vet ./... && go test -cover ./..."

docker-shell: docker-build ## Shell interactif dans le conteneur de dev
	docker run --rm -it $(DOCKER_IMAGE) bash

export-proposal: ## Exporte les artefacts d'une SecurityProfileProposal vers OUT_DIR (usage: make export-proposal PROPOSAL=<nom> [NS=default] [OUT_DIR=out/<nom>])
	@test -n "$(PROPOSAL)" || (echo "PROPOSAL est requis (ex: make export-proposal PROPOSAL=nginx-demo)"; exit 1)
	@mkdir -p "$(OUT_DIR)"
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.podLock}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/profile.yaml"
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.networkPolicy}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/networkpolicy.yaml"
	@if [ ! -s "$(OUT_DIR)/networkpolicy.yaml" ]; then rm -f "$(OUT_DIR)/networkpolicy.yaml"; fi
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.patchedManifest}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/patched.yaml"
	@if [ ! -s "$(OUT_DIR)/patched.yaml" ]; then rm -f "$(OUT_DIR)/patched.yaml"; fi
	@kubectl get securityprofileproposal "$(PROPOSAL)" -n "$(NS)" -o jsonpath='{.spec.spoSeccompProfile}' | awk '{gsub(/\\\\n/, "\n")}1' > "$(OUT_DIR)/seccompprofile.yaml"
	@if [ ! -s "$(OUT_DIR)/seccompprofile.yaml" ]; then rm -f "$(OUT_DIR)/seccompprofile.yaml"; fi
	@echo "Artefacts exportes dans $(OUT_DIR)"

apply-proposal: export-proposal ## Exporte puis applique les artefacts de la proposal (PodLock, NetworkPolicy/SPO si presents, workload patch en dernier)
	@kubectl apply -f "$(OUT_DIR)/profile.yaml"
	@if [ -f "$(OUT_DIR)/networkpolicy.yaml" ]; then kubectl apply -f "$(OUT_DIR)/networkpolicy.yaml"; fi
	@if [ -f "$(OUT_DIR)/seccompprofile.yaml" ]; then kubectl apply -f "$(OUT_DIR)/seccompprofile.yaml"; fi
	@if [ -f "$(OUT_DIR)/patched.yaml" ]; then kubectl apply -f "$(OUT_DIR)/patched.yaml"; fi
	@echo "Artefacts appliques depuis $(OUT_DIR)"

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

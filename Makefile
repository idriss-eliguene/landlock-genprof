DOCKER_IMAGE := landlock-genprof-dev

.PHONY: help init-vm check-kernel build test vet fmt docker-build docker-test docker-shell

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

docker-build: ## Construit l'image Dockerfile.dev (build/test Linux réel, y compris internal/tracer, sans la VM)
	docker build -f Dockerfile.dev -t $(DOCKER_IMAGE) .

docker-test: docker-build ## go build + go test dans le conteneur Linux (équivalent CI, sans cluster réel)
	docker run --rm $(DOCKER_IMAGE) sh -c "go build ./... && go vet ./... && go test -cover ./..."

docker-shell: docker-build ## Shell interactif dans le conteneur de dev
	docker run --rm -it $(DOCKER_IMAGE) bash

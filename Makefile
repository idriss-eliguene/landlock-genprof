.PHONY: help init-vm check-kernel

help: ## Liste les commandes disponibles
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "%-15s %s\n", $$1, $$2}'

init-vm: ## Installe kind/kubectl/Inspektor Gadget et déploie le pod de test (voir HOW_TO_START.md §2)
	./hack/init-vm.sh

check-kernel: ## Vérifie que le kernel hôte supporte Landlock et eBPF
	./hack/check-kernel.sh

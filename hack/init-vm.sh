#!/usr/bin/env bash
# Sets up the dev VM from scratch: kind, kubectl, Inspektor Gadget, and a
# test pod — everything needed before internal/tracer.Trace() can be
# exercised manually via `ig trace open` (see HOW_TO_START.md §5,
# Student A section).
#
# Idempotent: safe to re-run if a step fails partway (network hiccup,
# cluster not ready yet, ...) — already-done steps are skipped.
#
# Printed strings (echo) below are kept in French on purpose: this script
# is run by French-speaking students per HOW_TO_START.md.
set -euo pipefail

KIND_VERSION="v0.32.0"
KUBECTL_VERSION="v1.36.2"
IG_VERSION="v0.54.1"
CLUSTER_NAME="landlock-dev"

case "$(uname -m)" in
	x86_64) ARCH="amd64" ;;
	aarch64 | arm64) ARCH="arm64" ;;
	*)
		echo "Architecture non supportée : $(uname -m)"
		exit 1
		;;
esac
echo "Architecture détectée : ${ARCH}"

GOBIN="$(go env GOPATH)/bin"
if [[ ":$PATH:" != *":${GOBIN}:"* ]]; then
	echo "⚠️  ${GOBIN} n'est pas dans ton PATH (c'est là que 'go install' met ses"
	echo "    binaires, dont kind). Ajouté pour cette exécution du script."
	echo "    Pour que ça reste vrai dans tes prochains terminaux, ajoute à ~/.bashrc :"
	echo "    export PATH=\$PATH:${GOBIN}"
	export PATH="$PATH:${GOBIN}"
fi

echo
echo "== 1/6 : kind =="
if kind version >/dev/null 2>&1; then
	echo "kind déjà installé : $(kind version)"
else
	go install "sigs.k8s.io/kind@${KIND_VERSION}"
	echo "kind installé : $(kind version)"
fi

echo
echo "== 2/6 : kubectl =="
if kubectl version --client >/dev/null 2>&1; then
	echo "kubectl déjà installé : $(kubectl version --client --output=yaml | grep gitVersion)"
else
	curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl"
	sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
	rm kubectl
	echo "kubectl installé : $(kubectl version --client --output=yaml | grep gitVersion)"
fi

echo
echo "== 3/6 : cluster kind =="
if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
	echo "Cluster '${CLUSTER_NAME}' déjà présent."
else
	kind create cluster --name "$CLUSTER_NAME"
fi
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
kubectl get nodes

echo
echo "== 4/6 : Inspektor Gadget (ig + plugin kubectl-gadget) =="
if ig version >/dev/null 2>&1; then
	echo "ig déjà installé : $(ig version)"
else
	curl -sL "https://github.com/inspektor-gadget/inspektor-gadget/releases/download/${IG_VERSION}/ig-linux-${ARCH}-${IG_VERSION}.tar.gz" \
		| sudo tar -xzf - -C /usr/local/bin
	echo "ig installé : $(ig version)"
fi

if kubectl gadget version >/dev/null 2>&1; then
	echo "kubectl-gadget déjà installé."
else
	curl -sL "https://github.com/inspektor-gadget/inspektor-gadget/releases/download/${IG_VERSION}/kubectl-gadget-linux-${ARCH}-${IG_VERSION}.tar.gz" \
		| sudo tar -xzf - -C /usr/local/bin
fi

echo
echo "== 5/6 : déploiement d'Inspektor Gadget sur le cluster =="
kubectl gadget deploy
echo "Attente que les pods gadget soient prêts (jusqu'à 60s)..."
kubectl wait --for=condition=Ready pod -n gadget --all --timeout=60s || {
	echo "⚠️  Les pods gadget ne sont pas prêts après 60s — vérifie manuellement :"
	echo "    kubectl get pods -n gadget"
	echo "    kubectl logs -n gadget -l k8s-app=gadget"
	exit 1
}
kubectl get pods -n gadget

echo
echo "== 6/6 : pod de test (nginx-demo) =="
if kubectl get pod nginx-demo >/dev/null 2>&1; then
	echo "Pod nginx-demo déjà présent."
else
	kubectl run nginx-demo --image=nginx:alpine --port=80
fi
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=60s
kubectl get pod nginx-demo

echo
echo "✅ Infra prête. Premier test manuel :"
echo "    ig trace open --containername nginx-demo"
echo "  (dans un autre terminal : kubectl exec nginx-demo -- ls /etc)"

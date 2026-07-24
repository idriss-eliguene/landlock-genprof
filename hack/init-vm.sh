#!/usr/bin/env bash
# Sets up the dev VM from scratch: kind (with Cilium as CNI, not the
# default kindnet — kindnet doesn't enforce NetworkPolicy), Helm,
# kubectl, Inspektor Gadget, and a test pod — everything needed before
# internal/tracer.Trace() can be exercised manually via `kubectl gadget
# run trace_open:...` (see HOW_TO_START.md §5, Student A section).
#
# Does NOT install security-profiles-operator or PodLock — both are
# opt-in enforcement dependencies, only needed if you actually want a
# generated seccomp/Landlock profile enforced, not to run a trace itself.
# See docs/enforcement-prerequisites.md, including why PodLock in
# particular isn't set up here at all (kind isn't a supported node type
# for it, per PodLock's own docs).
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
HELM_VERSION="v4.2.3"
CILIUM_VERSION="1.19.6"
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
echo "== 3/7 : Helm =="
if helm version >/dev/null 2>&1; then
	echo "Helm déjà installé : $(helm version --short)"
else
	curl -LO "https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz"
	tar -xzf "helm-${HELM_VERSION}-linux-${ARCH}.tar.gz"
	sudo install -o root -g root -m 0755 "linux-${ARCH}/helm" /usr/local/bin/helm
	rm -rf "helm-${HELM_VERSION}-linux-${ARCH}.tar.gz" "linux-${ARCH}"
	echo "Helm installé : $(helm version --short)"
fi

echo
echo "== 4/7 : cluster kind, CNI Cilium (pas kindnet) =="
# kindnet (le CNI par défaut de kind) ne supporte pas NetworkPolicy — un
# `kubectl apply` de networkpolicy.yaml (--network-out) ne ferait donc
# strictement rien avec le CNI par défaut, silencieusement. Cilium
# remplace kindnet pour que le NetworkPolicy généré soit réellement
# appliqué. Voir HOW_TO_START.md pour le détail.
if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
	echo "Cluster '${CLUSTER_NAME}' déjà présent."
else
	cat <<EOF | kind create cluster --name "$CLUSTER_NAME" --config -
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
EOF
fi
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
kubectl get nodes

if command -v cilium >/dev/null 2>&1; then
	echo "CLI cilium déjà installée : $(cilium version --client 2>/dev/null | head -1)"
else
	# La CLI cilium suit son propre versioning (dépôt séparé de Cilium
	# lui-même) — contrairement à kind/kubectl/ig/Helm ci-dessus, on suit
	# ici la méthode officiellement recommandée par Cilium (lookup de la
	# version stable courante), pas une version figée.
	CILIUM_CLI_VERSION="$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)"
	curl -L --fail --remote-name-all "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${ARCH}.tar.gz"
	sudo tar -xzf "cilium-linux-${ARCH}.tar.gz" -C /usr/local/bin
	rm "cilium-linux-${ARCH}.tar.gz"
	echo "CLI cilium installée : ${CILIUM_CLI_VERSION}"
fi

if kubectl get daemonset -n kube-system cilium >/dev/null 2>&1; then
	echo "Cilium déjà déployé sur le cluster."
else
	helm repo add cilium https://helm.cilium.io/ >/dev/null
	helm repo update cilium >/dev/null
	helm install cilium cilium/cilium --version "$CILIUM_VERSION" \
		--namespace kube-system \
		--set image.pullPolicy=IfNotPresent \
		--set ipam.mode=kubernetes
fi
echo "Attente que Cilium soit prêt (jusqu'à 120s)..."
cilium status --wait --wait-duration 120s

echo
echo "== 5/7 : Inspektor Gadget (ig + plugin kubectl-gadget) =="
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
echo "== 6/7 : déploiement d'Inspektor Gadget sur le cluster =="
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
echo "== 7/7 : pod de test (nginx-demo) =="
if kubectl get pod nginx-demo >/dev/null 2>&1; then
	echo "Pod nginx-demo déjà présent."
else
	kubectl run nginx-demo --image=nginx:alpine --port=80
fi
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=60s
kubectl get pod nginx-demo

echo
echo "✅ Infra prête. Premier test manuel :"
# trace_open:latest, pas ${IG_VERSION} : les images de gadgets ont leur
# propre versioning, pas aligné sur les releases du CLI ig/kubectl-gadget
# (trace_open:v0.54.1 n'existe pas — voir HOW_TO_START.md §5).
echo "    kubectl gadget run trace_open:latest -n default -c nginx-demo"
echo "  (dans un autre terminal : kubectl exec nginx-demo -- ls /etc)"

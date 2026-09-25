#!/usr/bin/env bash
# Safe bootstrap wrapper around the existing three-trace Golden E2E.
# Normal mode is mutating and must be invoked explicitly by an operator.
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXPECTED_CONTEXT=""
CHECK_ONLY=0
NAMESPACE="landlock-genprof-e2e"
PROPOSAL="nginx-demo"
ARTIFACTS_DIR="${BOOTSTRAP_ARTIFACTS_DIR:-$ROOT_DIR/artifacts/proposal-bootstrap}"

usage() {
  cat >&2 <<'EOF'
Usage: hack/proposal-bootstrap.sh --expected-context <context> [--check-only]

Options:
  --expected-context <context>  Required exact Kubernetes context.
  --check-only                  Read-only prerequisites; never invokes Golden.
  --artifacts-dir <directory>   Evidence and Golden artifact directory.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --expected-context|--context) [ "$#" -ge 2 ] || { usage; exit 2; }; EXPECTED_CONTEXT="$2"; shift 2 ;;
    --check-only) CHECK_ONLY=1; shift ;;
    --artifacts-dir) [ "$#" -ge 2 ] || { usage; exit 2; }; ARTIFACTS_DIR="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "ERROR: unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

[ -n "$EXPECTED_CONTEXT" ] || { echo "ERROR: --expected-context is required; refusing implicit cluster selection" >&2; exit 2; }
command -v kubectl >/dev/null 2>&1 || { echo "ERROR: kubectl not found" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq not found" >&2; exit 1; }
command -v kubectl-landlock_genprof >/dev/null 2>&1 || { echo "ERROR: kubectl-landlock_genprof not found on PATH" >&2; exit 1; }

mkdir -p "$ARTIFACTS_DIR"
CURRENT_CONTEXT="$(kubectl config current-context 2>/dev/null || true)"
[ "$CURRENT_CONTEXT" = "$EXPECTED_CONTEXT" ] || {
  echo "ERROR: current context '$CURRENT_CONTEXT' != expected '$EXPECTED_CONTEXT'" >&2
  exit 1
}
kubectl landlock-genprof --help >/dev/null 2>&1 || { echo "ERROR: plugin help failed" >&2; exit 1; }
kubectl cluster-info >/dev/null

for crd in securityprofileproposals.landlockgenprof.io traininghistories.landlockgenprof.io applyattempts.landlockgenprof.io; do
  kubectl get crd "$crd" >/dev/null 2>&1 || { echo "ERROR: required CRD missing: $crd" >&2; exit 1; }
done
kubectl get ns gadget >/dev/null 2>&1 || { echo "ERROR: gadget namespace missing" >&2; exit 1; }
kubectl get daemonset -n gadget gadget >/dev/null 2>&1 || { echo "ERROR: Inspektor Gadget daemonset missing" >&2; exit 1; }

SPO_PRESENT=false
PODLOCK_PRESENT=false
if kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io >/dev/null 2>&1; then SPO_PRESENT=true; fi
if kubectl get crd landlockprofiles.podlock.kubewarden.io >/dev/null 2>&1; then PODLOCK_PRESENT=true; fi

if [ "$CHECK_ONLY" -eq 1 ]; then
  jq -n \
    --arg expected "$EXPECTED_CONTEXT" --arg current "$CURRENT_CONTEXT" \
    --arg namespace "$NAMESPACE" --arg proposal "$PROPOSAL" \
    --argjson spo "$SPO_PRESENT" --argjson podlock "$PODLOCK_PRESENT" \
    '{schemaVersion:"landlock-genprof/proposal-bootstrap/v1", mode:"check-only", expectedContext:$expected, currentContext:$current, namespace:$namespace, proposalName:$proposal, prerequisites:{kubectl:true, plugin:true, apiServer:true, requiredCRDs:true, inspektorGadget:true, spo:$spo, podlock:$podlock}, governance:{status:"NOT_RUN", reason:"check-only performs no proposal or apply operation"}, enforcement:{status:"NOT_VERIFIED", reason:"no runtime enforcement probe was executed"}, proposal:null, applyAttempts:[]}' \
    > "$ARTIFACTS_DIR/proposal-bootstrap.json"
  BOOTSTRAP_NAMESPACE="$NAMESPACE" BOOTSTRAP_PROPOSAL="$PROPOSAL" \
    "$ROOT_DIR/hack/validate-proposal-bootstrap-evidence.sh" "$ARTIFACTS_DIR/proposal-bootstrap.json"
  echo "check-only evidence: $ARTIFACTS_DIR/proposal-bootstrap.json"
  exit 0
fi

echo "[warning] execute mode mutates only the Golden E2E namespace and resources"
EXPECTED_CONTEXT="$EXPECTED_CONTEXT" GOLDEN_ARTIFACTS_DIR="$ARTIFACTS_DIR" \
  bash "$ROOT_DIR/hack/demo-golden.sh"

kubectl get securityprofileproposal "$PROPOSAL" -n "$NAMESPACE" -o json > "$ARTIFACTS_DIR/proposal.json"
kubectl get applyattempts -n "$NAMESPACE" -o json > "$ARTIFACTS_DIR/applyattempts.json"

PROPOSAL_UID="$(jq -er '.metadata.uid' "$ARTIFACTS_DIR/proposal.json")"
PROPOSAL_RV="$(jq -er '.metadata.resourceVersion' "$ARTIFACTS_DIR/proposal.json")"
APPROVED_DIGEST="$(jq -er '.status.approvedCandidateDigest' "$ARTIFACTS_DIR/proposal.json")"

jq -e --arg namespace "$NAMESPACE" --arg proposal "$PROPOSAL" --arg uid "$PROPOSAL_UID" --arg digest "$APPROVED_DIGEST" '
  [.items[] | select(.spec.proposalNamespace == $namespace and .spec.proposalName == $proposal)] as $matches
  | ($matches | length) > 0
  and all($matches[];
    .metadata.namespace == $namespace and
    .spec.proposalUID == $uid and
    .spec.approvedCandidateDigest == $digest and
    .spec.target.namespace == $namespace and
    (.status.state as $state | (["IN_PROGRESS", "APPLIED", "PARTIALLY_APPLIED", "FAILED", "OUTCOME_UNKNOWN"] | index($state)) != null))
' "$ARTIFACTS_DIR/applyattempts.json" >/dev/null || {
  echo "ERROR: ApplyAttempt provenance assertion failed" >&2
  exit 1
}

jq -n --arg expected "$EXPECTED_CONTEXT" --arg current "$CURRENT_CONTEXT" \
  --arg namespace "$NAMESPACE" --arg proposal "$PROPOSAL" \
  --arg uid "$PROPOSAL_UID" --arg rv "$PROPOSAL_RV" --arg digest "$APPROVED_DIGEST" \
  --slurpfile attemptsObject "$ARTIFACTS_DIR/applyattempts.json" \
  --argjson spo "$SPO_PRESENT" --argjson podlock "$PODLOCK_PRESENT" \
  '{schemaVersion:"landlock-genprof/proposal-bootstrap/v1", mode:"execute", expectedContext:$expected, currentContext:$current, namespace:$namespace, proposalName:$proposal, prerequisites:{kubectl:true, plugin:true, apiServer:true, requiredCRDs:true, inspektorGadget:true, spo:$spo, podlock:$podlock}, governance:{status:"PASS", proposalUID:$uid, proposalResourceVersion:$rv, approvedCandidateDigest:$digest}, enforcement:{status:"NOT_VERIFIED", reason:"SPO/PodLock resource readiness is not kernel enforcement evidence"}, proposal:{uid:$uid, resourceVersion:$rv, approvedCandidateDigest:$digest}, applyAttempts:[$attemptsObject[0].items[] | select(.spec.proposalNamespace == $namespace and .spec.proposalName == $proposal) | {namespace:.metadata.namespace, name:.metadata.name, uid:.metadata.uid, resourceVersion:.metadata.resourceVersion, proposal:{namespace:.spec.proposalNamespace, name:.spec.proposalName, uid:.spec.proposalUID, approvedCandidateDigest:.spec.approvedCandidateDigest}, target:{namespace:.spec.target.namespace, workload:.spec.target.workload, container:.spec.target.container}, operatorIdentity:(.spec.operatorIdentity // ""), state:(.status.state // "")}]} ' \
  > "$ARTIFACTS_DIR/proposal-bootstrap.json"

BOOTSTRAP_NAMESPACE="$NAMESPACE" BOOTSTRAP_PROPOSAL="$PROPOSAL" \
  "$ROOT_DIR/hack/validate-proposal-bootstrap-evidence.sh" "$ARTIFACTS_DIR/proposal-bootstrap.json"
echo "governance evidence: PASS"
echo "enforcement qualification: NOT VERIFIED"
echo "evidence: $ARTIFACTS_DIR/proposal-bootstrap.json"

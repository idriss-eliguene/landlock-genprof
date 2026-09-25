#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat > "$TMP_DIR/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${FAKE_KUBECTL_LOG:?}"
case "$*" in
  "config current-context") printf '%s\n' "fake-context" ;;
  "cluster-info"|"landlock-genprof --help"|"plugin list") ;;
  "get crd "*|"get ns gadget"|"get daemonset -n gadget gadget") ;;
  *) echo "unexpected fake kubectl request: $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$TMP_DIR/kubectl"
ln -s "$TMP_DIR/kubectl" "$TMP_DIR/kubectl-landlock_genprof"

FAKE_KUBECTL_LOG="$TMP_DIR/kubectl.log" PATH="$TMP_DIR:$PATH" \
  BOOTSTRAP_ARTIFACTS_DIR="$TMP_DIR/artifacts" \
  bash "$ROOT_DIR/hack/proposal-bootstrap.sh" --check-only --expected-context fake-context

jq -e '.mode == "check-only" and .governance.status == "NOT_RUN" and .enforcement.status == "NOT_VERIFIED"' \
  "$TMP_DIR/artifacts/proposal-bootstrap.json" >/dev/null
if grep -E '(^| )(apply|create|delete|patch)( |$)' "$TMP_DIR/kubectl.log" >/dev/null; then
  echo "check-only invoked a mutating kubectl operation" >&2
  exit 1
fi

VALID="$TMP_DIR/valid.json"
jq -n '{schemaVersion:"landlock-genprof/proposal-bootstrap/v1",mode:"execute",expectedContext:"fake-context",currentContext:"fake-context",prerequisites:{},governance:{status:"PASS"},enforcement:{status:"NOT_VERIFIED"},proposal:{uid:"p-uid",resourceVersion:"7",approvedCandidateDigest:"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},applyAttempts:[{namespace:"landlock-genprof-e2e",proposal:{namespace:"landlock-genprof-e2e",name:"nginx-demo",uid:"p-uid",approvedCandidateDigest:"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},target:{namespace:"landlock-genprof-e2e"},state:"APPLIED"}]}' > "$VALID"
BOOTSTRAP_NAMESPACE=landlock-genprof-e2e BOOTSTRAP_PROPOSAL=nginx-demo \
  bash "$ROOT_DIR/hack/validate-proposal-bootstrap-evidence.sh" "$VALID"

jq '.applyAttempts[0].proposal.uid = "wrong"' "$VALID" > "$TMP_DIR/invalid.json"
if BOOTSTRAP_NAMESPACE=landlock-genprof-e2e BOOTSTRAP_PROPOSAL=nginx-demo \
    bash "$ROOT_DIR/hack/validate-proposal-bootstrap-evidence.sh" "$TMP_DIR/invalid.json" >/dev/null 2>&1; then
  echo "invalid provenance was accepted" >&2
  exit 1
fi

echo "proposal bootstrap tests: PASS"

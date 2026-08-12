#!/usr/bin/env bash
set -euo pipefail

# Wrapper to run the Golden E2E against the dedicated kind context.
EXPECTED_CONTEXT=kind-landlock-genprof-e2e
CUR_CTX=$(kubectl config current-context 2>/dev/null || true)
if [ "$CUR_CTX" != "$EXPECTED_CONTEXT" ]; then
  echo "ERROR: current context '$CUR_CTX' != expected '$EXPECTED_CONTEXT' — aborting"
  exit 1
fi

# Ensure namespace exists
kubectl create ns landlock-genprof-e2e --dry-run=client -o yaml | kubectl apply -f -

# Run the demo script in non-check-only mode (but this wrapper does not auto-run; left for manual execution)
echo "To run the mutating Golden E2E, execute: hack/demo-golden.sh"

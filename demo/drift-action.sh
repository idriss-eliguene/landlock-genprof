#!/usr/bin/env bash
# drift-action.sh — demo-only workload behavior driver.
#
# Why this exists separately from demo/golden/run-actions.sh: that script is
# owned by the authoritative Golden E2E (hack/demo-golden.sh asserts exact
# seenInRuns counts produced by its actions). Adding a new action there to
# serve the demo would put a release gate at the mercy of presentation
# changes. This file drives the *additional* behavior the demo needs and
# nothing else.
#
# It performs real work inside the real workload with the same mechanism
# run-actions.sh uses — an in-container curl — and contains no product
# logic whatsoever. It does not know what a candidate, a digest or an
# approval is.
#
# Usage: ./drift-action.sh <action>

set -euo pipefail

NAMESPACE=${NAMESPACE:-landlock-genprof-e2e}
POD=${POD:-nginx-demo}
ACTION_CONTAINER=${ACTION_CONTAINER:-tools}
CURL_BIN=${CURL_BIN:-/usr/bin/curl}

# The path the workload starts writing after approval. A *filesystem* path
# is the deliberate choice: `diff` compares synthesized Landlock candidates
# rule by rule, keyed on path (see cmd/landlock-genprof/diff.go), so a new
# path is a change diff can actually show. A new egress port would change
# the proposal's NetworkPolicy artifact — and therefore the candidate
# digest — but would render as nothing in `diff`, leaving the demo with
# two different hashes and no visible explanation.
DRIFT_PATH=${DRIFT_PATH:-/srv/nginx/data/audit-export}

case ${1:-} in
  fs_new_path)
    # Write to a path the workload has never touched in any previous
    # training run. Same --create-dirs mechanism as run-actions.sh's own
    # filesystem actions, so the observed process (proc.comm == curl)
    # matches the traced binary.
    if ! kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- \
      "$CURL_BIN" -sS --create-dirs \
      "http://echo-8080-svc.${NAMESPACE}.svc.cluster.local:8080" \
      -o "$DRIFT_PATH"; then
      echo "ERROR: fs_new_path in-container curl failed" >&2
      exit 1
    fi
    ;;
  clean)
    # Remove the drifted path so a re-run starts from the same baseline.
    # Best-effort: the container image has no shell, so this uses curl to
    # overwrite rather than rm, and a failure here is not fatal — reset.sh
    # recreates the pod when a truly clean slate is needed.
    kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- \
      "$CURL_BIN" -sS file:///dev/null -o "$DRIFT_PATH" >/dev/null 2>&1 || true
    ;;
  path)
    printf '%s\n' "$DRIFT_PATH"
    ;;
  *)
    echo "unknown action: ${1:-}" >&2
    echo "usage: $0 {fs_new_path|clean|path}" >&2
    exit 2
    ;;
esac

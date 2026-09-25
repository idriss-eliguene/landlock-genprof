#!/usr/bin/env bash
# Validate machine-readable evidence emitted by proposal-bootstrap.sh.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <evidence.json>" >&2
  exit 2
fi

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq not found" >&2; exit 2; }

jq -e '
  .schemaVersion == "landlock-genprof/proposal-bootstrap/v1" and
  (.mode == "check-only" or .mode == "execute") and
  (.expectedContext | type == "string" and length > 0) and
  (.currentContext | type == "string") and
  (.prerequisites | type == "object") and
  (.governance.status == "PASS" or .governance.status == "NOT_RUN") and
  .enforcement.status == "NOT_VERIFIED" and
  (if .mode == "check-only" then
     .governance.status == "NOT_RUN" and (.applyAttempts | length) == 0
   else
     .governance.status == "PASS" and
     (.proposal.uid | type == "string" and length > 0) and
     (.proposal.resourceVersion | type == "string" and length > 0) and
     (.proposal.approvedCandidateDigest | test("^sha256:[0-9a-f]{64}$")) and
     (.applyAttempts | length > 0) and
     all(.applyAttempts[];
       .namespace == $namespace and
       .proposal.namespace == $namespace and
       .proposal.name == $proposal and
       .proposal.uid == $proposalUID and
       .proposal.approvedCandidateDigest == $digest and
       .target.namespace == $namespace and
       (.state as $state | (["IN_PROGRESS", "APPLIED", "PARTIALLY_APPLIED", "FAILED", "OUTCOME_UNKNOWN"] | index($state)) != null))
   end)
' --arg namespace "${BOOTSTRAP_NAMESPACE:-landlock-genprof-e2e}" \
   --arg proposal "${BOOTSTRAP_PROPOSAL:-nginx-demo}" \
   --arg proposalUID "$(jq -r '.proposal.uid // ""' "$1")" \
   --arg digest "$(jq -r '.proposal.approvedCandidateDigest // ""' "$1")" \
   "$1" >/dev/null

echo "proposal bootstrap evidence: valid"

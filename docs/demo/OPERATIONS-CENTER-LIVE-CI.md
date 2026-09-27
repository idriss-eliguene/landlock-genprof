# Operations Center live CI qualification

The `Operations Center live qualification` workflow runs on an isolated Ubuntu
GitHub-hosted runner. It provisions host-node k3s (not Kind, because SPO Gadget
PID attribution requires the Kubernetes node to share the runner host PID
namespace), installs the repository's pinned Gadget/SPO setup, and then runs
the actual React bundle, Go Operations Center server, observation executor,
trusted-proxy authentication fixture, and headless Chromium browser.

The test identity is a fixed CI fixture assertion, not a human login system.
Four Kubernetes users are isolated by namespace RBAC: operator, reviewer,
approver, and denied. Review and approval use separate users and configured
application groups. The test does not expose credentials to Chromium.

## Reproduce

Use the workflow's `workflow_dispatch` trigger on a commit under test.
The workflow itself records source SHA, tool versions, lifecycle stages and
uploads its sanitized artifacts. Do not run `hack/operations-center-live-ci.sh`
against a local/shared cluster: it expects the disposable k3s environment
created by the workflow and creates namespace-scoped test resources.

The browser journey covers live workload selection, Gadget-backed capture,
stop/finalization, persisted facts in Evidence Explorer, stale-Pod rejection,
current-Pod proposal generation, and review and approval by separate
authenticated identities. It verifies denied and cross-namespace requests
return authorization failures. Frontend tests separately preserve UNKNOWN and
incomplete-coverage presentation; the control-failure regression is explicitly
fixture-based and requires the verifier result to remain UNKNOWN.

Artifacts include Playwright traces/screenshots, backend/executor/proxy logs,
Gadget/SPO logs, Kubernetes events, stage outcomes, and source/component
versions. Kubernetes Secrets, kubeconfigs, tokens, and environment dumps are
excluded. The qualification namespace exists only in the disposable runner
cluster, which is uninstalled after artifact collection.

## Proof boundary

The current live Observation-to-proposal contract derives the candidate-v2
container-capabilities artifact; its Seccomp derivation status is
`NOT_AVAILABLE`. Therefore this proposal cannot produce the approved
SPO-backed SeccompProfile needed by the governed apply and bounded twin/control
experiment. The workflow records those stages as `NOT_RUN`; they must not be
inferred from SPO readiness or from separate SPO/verification tests.

The positive live result is limited to real Gadget observation through
separately authorized human review and approval. Approval is not application,
and neither this workflow nor its evidence claims that the original workload
enforces a Seccomp filter.

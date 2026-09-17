# 12 — Security UX constraints

These are invariants this redesign was required to preserve, and did. Each
line states what was verified and where.

## Credentials / kubeconfig never reach the browser

Verified by inspection: the client script (`workbench_ui.go`) never reads
or displays kubeconfig content, bearer tokens, or certificates. The only
credential-adjacent surface is the `Credential` chip, which shows the
**auth method name** (`credential.authMethod || "server-side"`), never a
secret value. `handleWorkbenchScript` rejects anything but `GET
/workbench.js`. Session binding uses opaque `X-Environment-Session` /
`X-Environment-Context-Version` / `X-Environment-Namespace` headers, not
credentials.

## No global "current cluster" leak

Each request that needs environment scope carries its session/context
headers explicitly (`environmentHeaders()` in `workbench_ui.go`); there is
no module-level "current cluster" the client silently reuses across
identities. Namespace/session binding happens per opened environment
session (`/api/v09/environments`), not as ambient global state.

## RBAC/SSAR remains the sole authority

The UI has no application-level role or persona concept. Every gated action
reads its permission from `/api/v08/capabilities` (server-derived from
SSAR). This redesign added no client-side role table, no hardcoded persona
switch, and no "if reviewer then show X" branching — see
[01-personas.md](01-personas.md) for why the three personas described
there are a design lens, not an implementation construct.

## CAS / resourceVersion semantics untouched

`proposalActions()` still requires a real `resourceVersion` for every
governance action (`hasVersion = Boolean(item.resourceVersion)`; missing it
forces a `STALE` disabled state). `governance()` still sends
`expectedResourceVersion` on every mutating call and still treats HTTP 409
as a distinct, non-retriable-by-the-UI outcome (`showError({}, true)`,
stale-state styling, explicit **Refresh** action, no silent replay). This
pass changed none of this logic — see [04-interaction-model.md](04-interaction-model.md)
for the parts that *were* touched (styling/labels only).

## Candidate digest / provenance / attribution semantics untouched

No change to how `candidateDigest`, `reviewContextDigest`, or attribution
state are computed or interpreted — those values are still read verbatim
from `proposalRead`/`observationRead` and shown via `technical()`. This
pass only changed *where* they sit in the visual hierarchy (secondary,
behind "Technical metadata"/"Technical observation details"), never their
values or meaning.

## Evidence UNKNOWN never renders as success

See [06-evidence-model.md](06-evidence-model.md) in full. The one-line
version: `stateClass()` defaults unmapped/`UNKNOWN` tokens to the neutral
`.unknown` class; only an explicit allow-list
(`HEALTHY|SUCCESS|AVAILABLE|APPLIED`) gets `.success`. This default-to-
unknown behavior was verified unchanged and is treated as a regression risk
for any future edit to `stateClass()`.

## Restricted-namespace access is neither hidden nor leaked

See Journey C in [02-user-journeys.md](02-user-journeys.md). Explicit
namespace entry exists for identities without list access; a namespace the
identity isn't authorized for fails through the same generic
authorization-failure path as any other denied read, so the UI cannot be
used to distinguish "doesn't exist" from "exists but you can't see it."

## Invariant checklist (self-assessed against this diff)

```
CREDENTIALS_BROWSER_EXPOSED=NO
KUBECONFIG_BROWSER_EXPOSED=NO
SENSITIVE_AUTH_DATA_EXPOSED=NO
GLOBAL_CURRENT_CLUSTER_PRESENT=NO
RBAC_BROADENED=NO
NETWORKPOLICY_WEAKENED=NO
SECCOMP_BASELINE_CHANGED=NO
TRUSTED_PROXY_CHANGED=NO
CANDIDATE_DIGEST_CHANGED=NO
PROVENANCE_SEMANTICS_CHANGED=NO
OBSERVATION_ATTRIBUTION_CHANGED=NO
GOVERNANCE_CAS_CHANGED=NO
```

Basis: the diff for this pass touches only
`cmd/landlock-genprof/workbench.go` (HTML/CSS template),
`cmd/landlock-genprof/workbench_ui.go` (client script), two Go test files
(`workbench_g7_test.go`, `workbench_g8_test.go`, expectation updates only),
`.gitignore`, and two pre-existing local `hack/*.sh` script edits unrelated
to product/security semantics (demo readiness bounding, screenshot
tooling). No file under `internal/`, `internal/proposal`, or the RBAC/CRD
manifests was touched. See [13-implementation-map.md](13-implementation-map.md)
for the full file list.

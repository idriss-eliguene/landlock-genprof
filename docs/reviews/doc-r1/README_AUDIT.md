# DOC-R1 README Audit

## First 60 seconds

| Question | Current answer | Assessment |
|---|---|---|
| What is landlock-genprof? | Evidence-driven least-privilege governance for Kubernetes. | Good, but competing historical framing follows quickly. |
| What problem does it solve? | Evidence/governance gap and hand-authored policy risk. | Good. |
| What does it observe? | Filesystem, network, applicable capabilities; SPO-derived syscalls are separated. | Good and semantically careful. |
| What does it derive? | SecurityProfileProposal and backend artifacts. | Good, but candidate-v1/v2 terminology needs explicit boundary. |
| What is governed? | Exact candidate digest and human authority. | Good. |
| What backends can it target? | PodLock/Landlock, NetworkPolicy, SPO/seccomp, capabilities/context. | Good with backend-specific qualification limits. |
| What is Operations Center? | Current paragraph says local read-only projection. | Materially incomplete/partly false for final authenticated React product. |
| What is demonstrated? | README links release/progress/demo evidence. | Good, but current M10.9 React baseline is not the front-door status. |
| What is not established? | Landlock kernel denial, capability enforcement, universal compatibility/minimality. | Good. |
| How do I try it? | Bootstrap, doctor, trace/review/approve/apply, old `ui` wording. | Usable, but browser path and current product actions need update. |

## Findings

- `README.md:20-26` correctly identifies Operations Center at `/`, but calls
  it read-only without distinguishing local legacy mode from authenticated
  governance mode.
- `README.md:129-151` omits Health and says governance/application actions
  remain CLI-only, conflicting with the certified React product surface.
- `README.md:159` links the current product to `book/src/workbench.md`, which
  is itself stale.
- The README is otherwise unusually strong on claim safety and backend
  qualification boundaries.

## Status

`README_STATUS=PARTIAL / NEEDS DOC-R2 REMEDIATION`

It is a strong technical front door but not yet an accurate final product
front door.

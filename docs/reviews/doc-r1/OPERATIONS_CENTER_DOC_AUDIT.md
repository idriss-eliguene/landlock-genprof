# DOC-R1 Operations Center Documentation Audit

## Status

`OPERATIONS_CENTER_DOC_STATUS=PARTIAL`

The repository contains substantial internal Operations Center UX and
semantic material, but the user-facing Book does not explain the final React
product. A new user can discover the CLI and historical Workbench, but cannot
follow one authoritative current Operations Center guide.

## Existing coverage

| Topic | Existing evidence | Status |
|---|---|---|
| What Operations Center is | `README.md`, product vision, migration doc | Partial; current mode/action semantics conflict. |
| Start/access | README and `book/src/workbench.md` | Stale; command exists but description is old. |
| Operational Context | `information-architecture.md`, environment-model, migration doc | Strong internal coverage, absent from Book. |
| ClusterIdentity / Namespace | information architecture and environment model | Strong internal coverage, no task-oriented user chapter. |
| Workloads | UX docs and screenshots | Partial; not connected to Book. |
| Observations | observation workflow and semantics | Strong semantics, older Workbench naming. |
| Evidence | evidence UX/model docs | Strong source-specific and UNKNOWN boundaries. |
| Proposals / Governance | governance workflow, migration doc, screenshots | Strong internal coverage; README incorrectly limits actions to CLI. |
| History | migration doc and UX docs | Present internally; bounded projection/audit boundary not in Book. |
| Attention | migration doc and UX docs | Two taxonomies and no severity/age are not consistently surfaced. |
| Health / SPHM | migration doc, UX v0.9 docs, final qualification record | Current semantics exist internally; no user-facing chapter. |
| UNKNOWN / NOT_ESTABLISHED | semantic docs and UI acceptance | Strong internal treatment, missing from onboarding. |
| Exact lineage | migration and UX docs | Strong internal treatment, absent from public workflow. |

## Missing user-facing explanation

1. Current L1 navigation and the canonical `/` route.
2. Server-owned Operational Context and authority transitions.
3. Difference between context locator, ClusterIdentity, namespace, and session.
4. Workload → Observation → Proposal → ApplyAttempt → RollbackAttempt custody.
5. Candidate-v2 versus CLI candidate-v1.
6. Separate CandidateDigest and ReviewContextDigest.
7. Health as a dimension ledger, not a score.
8. Two Attention taxonomies without severity, age, or resolution workflow.
9. Bounded History versus a complete audit log.
10. Applied, structurally qualified, and behaviorally verified as separate axes.

## Recommendation

Add one public Book chapter, “Operations Center,” linked from Start Here and
the main summary. Keep detailed internal UX/migration documents as engineering
references, but link them as implementation/semantics references rather than
the primary user path.

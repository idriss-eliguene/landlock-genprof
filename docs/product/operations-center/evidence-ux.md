# Evidence UX and semantics

Evidence is derived by `internal/observation/domain.DeriveEvidenceState`.

| State | Semantic meaning | Operator label | Proposal behavior |
|---|---|---|---|
| AVAILABLE | healthy backend, attached bounded window, flushed stream, completed attribution, zero exclusions, positive facts | Evidence captured | eligible subject to proposal rules |
| EMPTY | all proof conditions hold, but zero attributable facts | No attributable evidence | no useful candidate |
| UNKNOWN | a proof condition is absent, attribution is incomplete/failed, or exclusions exist | Evidence state unknown | fail closed; do not imply availability |

`Attribution=COMPLETED` says attribution reached its terminal state; it does
not by itself prove all evidence-quality conditions. `Execution=NOT_AVAILABLE`
is a projection/read-model absence of execution information, not a claim that
runtime execution succeeded. `frozen=true` means the persisted observation is
terminal and immutable. The combination `NOT_AVAILABLE / COMPLETED / UNKNOWN`
is therefore a legitimate incomplete/historical projection state: it is shown
as uncertainty, retained in technical details, and is not presented as
healthy or proposal-ready.

Capability facts are typed normalized facts, separate from the evidence state.
Candidate availability requires the canonical evidence/provenance rules;
proposal generation does not convert UNKNOWN into success.

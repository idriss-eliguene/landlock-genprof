# Architecture Decision Records

An ADR records one architectural decision this project has **actually
made** — what was decided, why, and what it costs — so a future
contributor doesn't have to reconstruct the reasoning from commit
messages or code comments alone.

## Why this project uses ADRs

Several real, consequential decisions currently live only in scattered
places — a comment in a package's own source, a paragraph inside a
larger design doc, an RBAC manifest's own header. An ADR gives one such
decision a single, indexed, discoverable home, without duplicating the
reasoning that already exists elsewhere: an ADR points to the code or
doc where a decision is explained in detail, it doesn't restate it.

## ADR vs. RFC

- **ADR — decided.** Records a decision this project has actually made,
  usually already implemented. Written after the fact, not as a
  proposal.
- **RFC — proposed.** A design under discussion, not yet decided — may
  end up accepted, rejected, or deferred. This project doesn't have an
  RFC directory yet. When one exists, an RFC that gets accepted becomes
  an ADR at that point, not before — the two are never the same
  document at different stages, they're different documents with a
  hand-off between them.

Don't write an ADR for something still being debated — that's an RFC's
job once this project has a place for those. Don't leave a real, shipped
decision undocumented either — that's the gap this index exists to
close.

## Status and immutability

Once an ADR is `Accepted`, it's a historical record — its content
doesn't change to reflect new information or a change of mind. If a
later decision changes course, it gets its **own** new ADR, and the
earlier one is marked `Superseded by ADR-NNNN` — never edited to
pretend the original reasoning was different than it actually was.

Statuses used here: `Proposed`, `Accepted`, `Superseded`, `Deprecated`.

## Index

| # | Title | Status | Date |
|---|---|---|---|
| [0001](0001-exit-code-contract.md) | Exit-code contract (0/1/2/3) | Accepted | 2026-08-07 |
| [0002](0002-generic-behavior-ir-boundary.md) | Generic Behavior IR as the exporter boundary | Accepted | 2026-08-08 |
| [0007](0007-governed-apply-ordering-and-enforcement-readiness.md) | Governed apply ordering and enforcement readiness | Accepted | 2026-08-19 |
| [0008](0008-spo-derived-policy-import-boundary.md) | SPO derived-policy import boundary | Accepted | 2026-08-19 |
| [0009](0009-spo-merged-seccomp-profile-provenance-and-target-separation.md) | SPO merged SeccompProfile provenance and target separation | Accepted | 2026-08-21 |

## Adding a new ADR

Copy [`0000-template.md`](0000-template.md), number it sequentially
(`0001`, `0002`, ...), fill it in, add a row to the index above.


## Architecture governance guide

See [docs/architecture/governance.md](../architecture/governance.md) for the
project's architecture governance, review methods, identity-sensitive change
policy, and ADR/RFC lifecycle guidance.

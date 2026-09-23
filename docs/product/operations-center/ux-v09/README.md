# Operations Center — UX v0.9 convergence

> **Historical implementation record.** This v0.9 UX package documents the
> pre-M10.9 server-rendered Workbench implementation and its qualification
> captures. The current supported UI is the React Operations Center at `/`.
> File paths and symbol names below are historical traceability references,
> not current runtime entry points.

This directory is the product/UX reference for the Operations Center redesign
that shipped on `feat/operations-center-v09-ux-convergence`, built on top of
the M4 milestone merged in PR #255. It supersedes the older, terser notes in
`docs/product/operations-center/*.md` as the primary source of truth for how
the Operations Center is organized and why; those files are kept for history
and are not maintained further.

This is not a green-field redesign. The M4 implementation already got most
of the hard product decisions right (workload-first observation cards,
explicit `EMPTY`/`UNKNOWN`/`AVAILABLE` evidence states, capability-gated
governance actions with CAS conflict handling). This convergence pass:

1. Audited that implementation against how an operator actually works
   (Cluster → Identity → Namespace → Workload → Observation → Evidence →
   Proposal → Governance → History), documented the real state model, and
   fixed the places where the UI still expressed backend state as static
   controls instead of a lifecycle.
2. Removed dead/duplicate presentation code discovered during that audit.
3. Fixed a real functional bug found during the visual review loop (the
   Identity selector silently failed to auto-bind on first load).
4. Consolidated the Governance surface into Proposals, because in the shipped
   code Governance was a near-empty page that only redirected to Proposals,
   while all the real review/approve/reject/apply logic already lived there.

## Start here

- [00 — Product vision](00-product-vision.md)
- [01 — Personas](01-personas.md)
- [02 — User journeys](02-user-journeys.md)
- [03 — Information architecture](03-information-architecture.md)
- [04 — Interaction model](04-interaction-model.md)
- [05 — Observation state model](05-observation-state-model.md)
- [06 — Evidence model](06-evidence-model.md)
- [07 — Design system](07-design-system.md)
- [08 — Component inventory](08-component-inventory.md)
- [09 — Page specifications](09-page-specifications.md)
- [10 — Accessibility](10-accessibility.md)
- [11 — Microcopy](11-microcopy.md)
- [12 — Security UX constraints](12-security-ux-constraints.md)
- [13 — Implementation map](13-implementation-map.md)
- [14 — UX acceptance criteria](14-ux-acceptance-criteria.md)
- [15 — Known limitations](15-known-limitations.md)
- [Screenshots](screenshots/README.md)

## Ground truth

Everything in this directory is derived from the actual shipped code, not
aspiration:

- HTML shell + CSS: `cmd/landlock-genprof/workbench.go` (`workbenchClusterPageTemplate`)
- Client script: `cmd/landlock-genprof/workbench_ui.go` (`workbenchScript`)
- Read model / DTOs: `cmd/landlock-genprof/workbench_read_model.go`
- Observation/Evidence domain semantics: `internal/observation/domain/observation.go`
- Governance/CAS semantics: `cmd/landlock-genprof/observation_api.go`,
  proposal package (`internal/proposal`)
- Real-environment E2E driver: `test/ui/workbench-smoke.js`,
  `hack/ui-lima-demo.sh`

Screenshots in `screenshots/` are real captures from `hack/ui-lima-demo.sh`
against a live kind cluster, not mockups.

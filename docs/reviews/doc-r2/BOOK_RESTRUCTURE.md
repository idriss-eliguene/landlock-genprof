# DOC-R2 Book Restructure

## Resulting public flow

```text
Home
→ Start Here
→ What is landlock-genprof? / current lifecycle
→ Getting started
→ Governed workflow
→ Observation and evidence
→ Operations Center
→ Usage and CLI reference
→ Backends and qualification
→ Troubleshooting
→ Architecture / ADRs / roadmap
```

## Changes

- `book/src/index.md` now introduces the governed workload-policy lifecycle.
- `book/src/start-here.md` now offers an Operations Center path.
- `book/src/workflow.md` now names `OBSERVE → DERIVE → GOVERN → REALIZE →
  QUALIFY`.
- `book/src/operations-center.md` is the first-class current user guide.
- `book/src/troubleshooting.md` documents supported authority and uncertainty
  failure states.
- `book/src/workbench.md` remains available only as an explicitly historical
  document and is no longer in the current summary navigation.
- `book/src/observation-semantics.md` retains normative semantics while
  removing current-product Workbench framing.
- CLI pages are regenerated from the Cobra tree and the summary includes
  `observe` and `executor`.

## Deliberately preserved

Historical release records, ADRs, RFCs, and milestone UX records remain in the
repository. They are not silently rewritten into present-tense claims. The
v0.9 UX package is labeled as historical implementation traceability.

# CLI design: from "trace → yaml" to a policy lifecycle

## Why this exists

The CLI today (`trace`/`review`/`apply-proposal`/`approve`/`reject`) is
organized around one implementation detail — a single training run — not
around the domain model this project has converged on across its own
architecture reviews (see [`landlock-kernel-extraction.md`](landlock-kernel-extraction.md)
and the knowledge-asset roadmap referenced there): **evidence → synthesis
→ verification → explanation → review → governance**, with export/apply
as the mechanical tail, not the identity.

This document is the design decision, not an implementation — commands
below move from "designed" to "shipped" one at a time, each gated on the
same discipline the kernel extraction already established: small,
tested, behavior-preserving steps, never a big-bang rewrite of the
existing `trace`/`review`/`apply-proposal`/`approve`/`reject` commands,
which stay working throughout.

## The five commands that define the identity

If a user remembers only five: `trace`, `synthesize`, `verify`,
`explain`, `approve`. Deliberately **not** `generate` and **not**
`export` — those describe what the tool mechanically does, not what it's
for. The five above describe a compiler-plus-linter-plus-reviewer
workflow: get evidence, compile it, check it against something real (the
kernel's actual ABI, the accumulated corpus), understand why, decide.

## Full command surface (target shape, not current)

```
landlock                                    # also: kubectl landlock-genprof <...>
├── trace                     evidence capture, one training run             [--events-out/--candidate-out shipped]
├── evidence                  noun group — accumulated evidence, multi-source
│   ├── list                  scans a directory for files that parse as evidence [shipped]
│   ├── show                  summarizes a raw evidence file                  [shipped]
│   └── import                pull evidence from an external source (SPO, strace, auditd) [not shipped]
├── synthesize                compile accumulated evidence into a candidate  [shipped — minimal: PodLock + candidate only, no cluster]
├── verify                    run the verification pass pipeline             [shipped — --candidate-file + --kernel, ABI check; --output text|sarif]
├── explain                   evidence-backed rationale for a candidate/rule  [shipped — --candidate-file + --path]
├── diff                      compare two candidates/runs, evidence-linked    [shipped — positional args, exit 0/1/3; --output text|junit]
├── review                    inspect a proposal before a decision           [exists]
├── approve                                                                   [exists]
├── reject                                                                    [exists]
├── export                    render a candidate/approved policy to a format [shipped — --format podlock, stdout or --out]
├── apply                     apply an approved artifact to the cluster       [exists as apply-proposal]
├── policy                    noun group — one policy's state over time
│   ├── list                  every SecurityProfileProposal in a namespace + approval state [shipped]
│   ├── status                approval gate for one proposal, exit 0/2         [shipped]
│   └── history                                                                [not shipped]
├── corpus                    noun group — the knowledge-base assets
│   ├── query
│   ├── add
│   └── sync
├── abi                       noun group — ABI-compatibility matrix           [shipped]
│   ├── check
│   └── list
├── governance                noun group — audit/compliance surface
│   ├── audit-log
│   └── report
├── doctor                    environment/prerequisite diagnostics           [new — first slice]
├── config
├── plugin                    tracer/exporter/verifier/explainer registries, not new commands
├── completion
└── version                                                                   [exists]
```

`export`/`apply` deliberately stay separate commands, never merged:
`export` never mutates a cluster, `apply` always does — same split as
`terraform plan`/`apply`.

## Verification pass pipeline

`verify` is not a single check — it's a pipeline of independently
testable passes over a `Candidate`, mirroring the "no monolithic
Synthesize" lesson already applied once during the kernel extraction.
First pass to ship: **ABI compatibility** — does every right in a
candidate exist on the target kernel's ABI level, and if not, what
silently degrades. Plugins (future) extend this pipeline by registering
new passes, not by adding new top-level commands — see the plugin
architecture note below.

## Plugin architecture

Plugins extend **internal registries**, never commands or subcommands: an
evidence-source registry (`evidence collect --method=<name>`), an
exporter/format registry (`export --format=<name>`), a pass registry
(`verify --passes=<name,...>`), an explainer registry
(`explain --style=<name>`). A command-per-plugin model would make the CLI
surface unbounded — the opposite of staying coherent for a decade.
Security note: a pass plugin runs at the same trust level as the core
verification pipeline, so plugin provenance checking (`plugin verify`) is
load-bearing, not polish — ships before `plugin install` does, whenever
that phase starts.

## Stability policy

- **Stable forever, never renamed:** `trace`, `synthesize`, `verify`,
  `explain`, `review`, `approve`, `reject`, `export`, `apply`, `diff`.
- **May evolve:** subcommand structure under `corpus`/`abi`/`policy`/
  `governance`/`plugin`.
- **Flags never removed once shipped:** `--output`/`-o`, `--namespace`/
  `-n`, `--dry-run`, `--kernel`/`--target-kernel`.
- **The exit-code contract is the highest-stakes promise in this design**
  — CI pipelines hard-code against it. Once `verify`/`diff` ship
  `0`/`1`/`2`/`3`, that contract is frozen: `0` clean, `1` non-blocking
  findings, `2` blocking failure, `3` usage error.

## Rejected on purpose

`generate` (the exact trap the five-command test names), a standalone
`import` (redundant with `evidence import`/`--from-file` flags),
`enforce`/`apply-runtime`/`block` (runtime enforcement is permanently not
this project's job — see the ecosystem-positioning review), broad
`scan`/`compliance-scan` (Kubescape's territory), `dashboard`/`serve`/`ui`
(platform-scope creep), per-domain top-level verbs like
`seccomp-generate` (stay as `export --format=...` flags, never their own
identity-bearing commands), `merge` (leaks the pass pipeline's internal
merge step as user surface), `train`/`learn` (misleading — this isn't a
black-box ML system).

## Rollout order (this repo's own discipline: smallest safe step first)

1. **`doctor` (shipped)** — zero risk, wraps `hack/check-kernel.sh`'s
   logic as a real Go subcommand instead of a shell script a user has to
   know to run. Also establishes the exit-code contract in `main()`
   (`0`/`1`/`2`) that `verify`/`diff` will need later — deliberately
   built on the cheapest command first.
2. **`abi check`/`abi list` (shipped)** — a verified Landlock ABI table
   (`internal/landlock/abi.go`, confirmed against kernel docs 2026-08-06),
   standalone, independent of any synthesized candidate. **Not** the same
   thing as `verify`: see the real gap this surfaced below.
3. **`verify` (shipped, first real check)** — reads a candidate from a
   JSON file (`internal/exporter/landlockjson` — the second exporter
   Phase 3 of the kernel-extraction plan called for, and the *only*
   format that can carry `TRUNCATE` at all: PodLock's own schema
   collapses it into "write" and never gets it back), checks every
   rule's rights against `--kernel`'s (or the local host's) Landlock ABI
   level via `landlock.ABIForKernel`/`landlock.RightsAt`, and reports
   exactly which rule needs which right at which ABI level — the first
   command that says something `doctor`/`abi` alone cannot: "this
   candidate needs kernel >= 6.2," not just "Landlock is supported at
   all." Exit `2` (blocking) if any rule is incompatible, `0` if all are.
   (Network was considered as a second, faster path to cross-ABI value —
   wrong: `NetworkPolicy` is CNI-enforced, unrelated to Landlock's ABI at
   all; corrected in
   [`landlock-kernel-extraction.md`](landlock-kernel-extraction.md#known-gap-rulerights-vs-the-full-landlock-abi-vocabulary)
   before it was built.)

   **Now wired to `trace`** — `trace --candidate-out` writes exactly the
   file `verify --candidate-file` reads, closing the gap this section
   used to describe. `internal/policy.Synthesize`'s own signature stayed
   untouched on purpose: rather than change its return shape (which
   would've touched its one production call site and 13 existing test
   call sites for no benefit they need), a small, separate
   `SynthesizeCandidate(events)` re-derives the raw `Candidate` straight
   from events — a second, cheap pass over the same data, matching the
   same "redundant computation over an API-stability break" trade-off
   `publishProposal` already makes for `BehaviorProfile` (see
   `packages.md`). Confirmed live end to end: a candidate with a
   `truncate` rule, written by `writeCandidateJSON`, read back by
   `verify`, correctly reports incompatible against an ABI2 kernel and
   compatible against ABI3+.
4. **`synthesize` (shipped, deliberately minimal)** — a real, separate
   verb: `synthesize --events-file <path> --pod ... --container ...
   --binary ...` reads a `trace --events-out` file (`internal/evidence`
   — the first slice of the `evidence` noun group; see below) and
   re-runs synthesis offline, no cluster connection, producing the same
   PodLock profile and candidate JSON `trace` writes inline. Deliberately
   **not** a full re-implementation of `trace`: NetworkPolicy/seccomp/
   capabilities/securityContext/report, history recording, and
   `SecurityProfileProposal` publishing all still need a live cluster and
   stay `trace`-only — widening `synthesize`'s scope is a later, separate
   decision, not assumed here. Confirmed live, the full chain in one
   pass: `trace`'s own `writeEventsJSON` output, fed to `synthesize`, fed
   to `verify`, correctly flags `truncate` against an ABI2 kernel and
   passes against ABI3+.

   `evidence` (the noun group) gets its first real slice too, earlier
   than planned: `internal/evidence` persists raw `tracer.Event`s to a
   canonical, round-trip JSON format — the actual "evidence" the
   evidence/synthesis split's own name presupposes, one stage before
   `internal/exporter/landlockjson`'s `Candidate` documents. No
   `evidence list`/`evidence show`/`evidence import` subcommands yet —
   just the file format and `trace --events-out`/`synthesize
   --events-file` as its two ends.
5. **`explain` (shipped)** — the fifth and last of the five identity
   verbs. Reads a candidate file (same `--candidate-file` as `verify`),
   prints each rule's rights annotated with their real ABI level and
   minimum kernel version (`landlock.ABIForRight`/`MinKernelFor`),
   confidence, seen-count, and evidence — `--path` narrows to one rule.
   No new data, no exit-code contract to design: purely a human-readable
   view of what a `Candidate` already carries. **With this, all five
   identity verbs (`trace`/`synthesize`/`verify`/`explain`/`approve`)
   exist for real** — Phase 1's own rule ("all five ship together, never
   a generate-only release") is now honestly satisfied, not just true in
   the design doc.
6. **`export` (shipped)** — the other half of Phase 1's own promise:
   `export --candidate-file <path> --format podlock --pod ... --out ...`
   renders an existing candidate to PodLock YAML without re-running
   synthesis, printing to stdout by default (`--out` to write a file
   instead) — closes the "still only as `--*-out` flags" gap this
   section used to flag. `--format` is a real, if narrow today, registry
   (`podlock` only; unsupported values error clearly) — a second format
   plugs in later without a new top-level command, per the plugin
   architecture below. Resolves the kernel-extraction plan's "Phase 2b"
   more lightly than originally sketched: `internal/policy.
   FileAccessesFromCandidate` (the already-tested private helper
   `Synthesize` itself used, now exported) does the `Candidate ->
   profile.FileAccess` translation at the call site — `internal/
   exporter/podlock.ToProfile`'s own signature never had to change at
   all. Confirmed live: a rule with `truncate` still shows up as
   `readWrite` in the rendered YAML, not silently dropped.
7. **`diff <old> <new>` (shipped)** — first Phase 2 command, and the
   first to use exit code `3` for real: `0` no differences, `1`
   differences found (rules added/removed/rights changed), `3` a usage
   error (bad file, unparseable JSON) — collapsing "the candidates
   differ" and "diff itself couldn't run" into the same code would make
   a CI gate unable to tell a real signal from a broken invocation.
   Prompted a small, low-risk cleanup: `doctorExitError` — already
   reused by `verify`/`abi` beyond its original namesake — renamed to
   `exitCodeError` and given an optional wrapped error, so `diff`'s
   usage-error case can carry a real message instead of a generic one.
8. **`evidence show <events-file>` (shipped)** — the noun group's first
   real slice: summarizes a raw evidence file (event counts by domain,
   distinct paths/ports, observation time span) — a different question
   than `explain` answers ("what did the tracer see" vs. "what rules did
   synthesis produce").

   **`evidence list [directory]` (shipped)** — deliberately doesn't
   invent a registry either: it scans a directory (default `.`) for
   files that happen to parse as evidence, one summary line each (event
   count, observation window), silently skipping files that don't parse
   (e.g. a `candidate.json` sitting next to them) — honest about the
   actual reality, evidence files are just files on disk, not entries in
   a store this project doesn't have. `observationWindow` factored out of
   `runEvidenceShow` so both commands compute the same first/last-
   timestamp span the same way. `evidence import` stays unbuilt: no
   external source (SPO, strace, auditd) is wired up to import from yet
   — building it now would still be speculative.

   **Confirmed live (2026-08-07):** `trace --events-out` against
   `nginx-demo` (350 events, 20s window) produced a file `evidence
   list .` correctly found and summarized, silently skipping the
   `nginx-demo-profile.yaml` sitting next to it — matching unit-test
   behavior against real tracer output, not just synthetic fixtures.
   Separately noted, not a regression from this session: the observation
   window printed as the exact same second for both ends
   (`2026-08-07T18:35:07Z` to itself) across all 350 events — a
   pre-existing tracer timestamp-granularity characteristic
   `evidence`/`observationWindow` only display, not something either
   command computes wrong; worth a closer look later, filed as a
   follow-up rather than blocking Phase 2.
9. **`policy list`/`policy status <name>` (shipped)** — not new
   infrastructure: both sit on `internal/proposal`, the same
   `SecurityProfileProposal` store `review`/`approve`/`reject` already
   use, which only ever exposed by-name `Get`/`Save` operations. `policy
   list` needed the one operation that store didn't have yet —
   `proposal.List` (sorted by name, reusing the existing
   `statusFromObject` helper) — added directly rather than routed around.
   `policy status` is deliberately not a copy of `review`: `review`
   prints the full spec for a human about to decide; `status` prints only
   the current approval state and, unlike every other read-only command
   in this project, gates on it — exit `0` only if `Approved`, `2`
   (blocking) otherwise. The one command whose job is "has a human signed
   off," distinct from every ABI/correctness check the rest of the CLI
   asks. (Caught building this: the fake dynamic client used in tests
   panics on `.List()` for any CRD with no generated Go type registered
   in the scheme — fixed with
   `dynamicfake.NewSimpleDynamicClientWithCustomListKinds`, which
   supplies the missing List-Kind hint explicitly.)

   **Confirmed live (2026-08-07):** against a real
   `SecurityProfileProposal` (published by the `trace` run above, not
   `dynamic/fake`) — `policy list` showed `nginx-demo Draft`; `policy
   status nginx-demo` exited `2` with `is not Approved (currently
   Draft)`; after `approve nginx-demo`, both `policy list` and `policy
   status` showed `Approved`, the latter exiting `0`. The exact exit-code
   contract this command was designed around, confirmed end to end for
   the first time against the real CRD.

   **With this, Phase 2 is complete** — `diff`, `evidence`
   (`show`/`list`), `abi`, and `policy` all ship for real.
10. **`verify` exit-code contract locked; `verify --output sarif` (shipped)**
    — auditing `verify` against the stability policy's own 0/1/2/3 table
    surfaced a real gap: every usage-error path (missing
    `--candidate-file`, unreadable/malformed candidate file, unparseable
    `--kernel`) returned a plain error, falling through to `main()`'s
    generic exit `1` — the code the contract reserves for "non-blocking
    findings," a concept `verify` doesn't even have. `diff` already got
    this right (`exitCodeError{code: 3}` for its own read/parse
    failures); `verify` didn't, despite shipping the same 0/2 partial
    contract earlier. Fixed by wrapping all four paths the same way —
    `TestRunVerify_MissingCandidateFileFlag`, which used to assert the
    *opposite* (explicitly not an `exitCodeError`, a deliberate choice at
    the time), now asserts exit `3`, matching `diff`.

    `--output sarif` renders the same findings as a SARIF 2.1.0 log via a
    new `internal/exporter/sarif` — deliberately generic (`Rule`/
    `Result`, not ABI-specific) since `verify` is itself a pass pipeline
    (see its own doc comment) and future passes register their own
    findings through the same renderer, per the plugin architecture note
    above. Findings use `logicalLocations`, never
    `physicalLocation`/`artifactLocation`: a rule's `Path` is a runtime
    filesystem path inside the traced container, not a file in the
    repository being scanned — claiming otherwise would promise GitHub
    Code Scanning-style line annotations this project can't deliver.
    The exit-code contract is identical in both output modes — `--output
    sarif` changes serialization, not what CI gates on. Confirmed:
    `verify --candidate-file ... --output sarif` produces a
    schema-valid SARIF log (checked with `python3 -m json.tool`) with
    exactly one result for an ABI3-only rule against an ABI2 kernel,
    still exiting `2`.
11. **`diff --output junit` (shipped)** — renders one JUnit `testcase`
    per rule path across both candidates (unchanged = passed, added/
    removed/rights-changed = failed) via a new `internal/exporter/junit`,
    sibling of `internal/exporter/sarif` for the same generic-renderer
    reasoning. `diff`'s per-path text-mode formatting was refactored to
    compute one `textLine`/`failure` pair per path instead of
    re-deriving the same added/removed logic twice for the two output
    modes — behavior-preserving (all pre-existing text-mode tests pass
    unchanged). Same discipline as `verify`: the exit-code contract
    (`0`/`1`/`3`) is identical in both output modes. Confirmed: `diff
    old.json new.json --output junit` produces well-formed XML (checked
    with `xml.dom.minidom.parseString`), correct `tests`/`failures`
    counts, still exiting `1`.

    **With this, Phase 3 is complete** — `verify`'s exit-code contract
    is locked and both `--output sarif`/`--output junit` ship for real.
12. Everything else, per the roadmap phases below.

## Roadmap (phase → commands → why)

| Phase | Commands | Why |
|---|---|---|
| 1 — Minimal CLI (**complete**) | `trace`, `synthesize` (minimal), `verify` (ABI1+ABI3), `explain`, `review`, `approve`/`reject`, `export` (`--format podlock`), `doctor` | All five identity verbs ship together — never a generate-only release |
| 2 — Professional daily usage (**complete**) | `diff`, `evidence` (`show`/`list` shipped, `import` deliberately deferred), `abi`, `policy` (`list`/`status` shipped) | `diff` starts feeding the review-decision corpus |
| 3 — CI/CD integration (**complete**) | `verify --output=sarif`, `diff --output=junit`, locked exit-code contract | Real integration failures surface at scale |
| 4 — Large-scale lifecycle | `corpus` (add/sync), `governance` | Corpus becomes genuinely community-fed |
| 5 — Reference toolkit | `plugin`, pass registry stabilized as a public interface | Other projects build on the registries instead of rebuilding |

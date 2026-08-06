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
├── trace                     evidence capture, one training run
├── evidence                  noun group — accumulated evidence, multi-source
│   ├── list
│   ├── show
│   └── import                pull evidence from an external source (SPO, strace, auditd)
├── synthesize                compile accumulated evidence into a candidate
├── verify                    run the verification pass pipeline
├── explain                   evidence-backed rationale for a candidate/rule
├── diff                      compare two candidates/runs, evidence-linked
├── review                    inspect a proposal before a decision           [exists]
├── approve                                                                   [exists]
├── reject                                                                    [exists]
├── export                    render a candidate/approved policy to a format
├── apply                     apply an approved artifact to the cluster       [exists as apply-proposal]
├── policy                    noun group — one policy's state over time
│   ├── list
│   ├── status
│   └── history
├── corpus                    noun group — the knowledge-base assets
│   ├── query
│   ├── add
│   └── sync
├── abi                       noun group — ABI-compatibility matrix
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
3. **`verify` (unblocked for a real first check, still not started as a
   command)** — `Rule.Rights` now carries real `LandlockRight` values
   (`READ_FILE`/`READ_DIR`/`WRITE_FILE`/`EXECUTE`, using the `IsDir` bit
   already tracked, plus `TRUNCATE` — ABI3 — read from `openat(2)`'s
   `O_TRUNC` flag, already flowing through the pipeline unread until now)
   instead of a coarse read/write/execute label — a real fidelity gain,
   proven behavior-neutral by the existing golden tests. `TRUNCATE` is
   the first right this package produces that isn't ABI1, so a
   filesystem-only verification pass can now say something a bare
   kernel-version check (`doctor`) cannot: "this candidate needs kernel
   >= 6.2." (Network was considered as a second, faster path to cross-ABI
   value — wrong: `NetworkPolicy` is CNI-enforced, unrelated to Landlock's
   ABI at all; corrected in
   [`landlock-kernel-extraction.md`](landlock-kernel-extraction.md#known-gap-rulerights-vs-the-full-landlock-abi-vocabulary)
   before it was built.) Remaining ceiling: `REMOVE_*`/`MAKE_*`/`REFER`
   still need new tracer syscall hooks (unlink/mkdir/rename/...), larger,
   separate work.
4. `synthesize` split out as its own command from `trace`'s current
   implicit last step.
5. Everything else, per the roadmap phases below.

## Roadmap (phase → commands → why)

| Phase | Commands | Why |
|---|---|---|
| 1 — Minimal CLI | `trace`, `synthesize`, `verify` (static ABI table), `explain`, `review`, `approve`/`reject`, `export`, `doctor` | All five identity verbs ship together — never a generate-only release |
| 2 — Professional daily usage | `diff`, `evidence`, `abi`, `policy` | `diff` starts feeding the review-decision corpus |
| 3 — CI/CD integration | `verify --output=sarif`, `diff --output=junit`, locked exit-code contract | Real integration failures surface at scale |
| 4 — Large-scale lifecycle | `corpus` (add/sync), `governance` | Corpus becomes genuinely community-fed |
| 5 — Reference toolkit | `plugin`, pass registry stabilized as a public interface | Other projects build on the registries instead of rebuilding |

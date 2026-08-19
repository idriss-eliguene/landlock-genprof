# Contributing to landlock-genprof

Thanks for considering a contribution. This project generates least-privilege
Kubernetes security profiles (Landlock, seccomp, NetworkPolicy, capabilities)
from observed workload behavior, then requires human review before anything
is ever applied — see [`README.md`](README.md) for the full pitch, and
[`docs/product-definition-v1.md`](docs/product-definition-v1.md),
[`docs/product-design-v1.md`](docs/product-design-v1.md), and
[`docs/product-roadmap-v1.md`](docs/product-roadmap-v1.md) for where the
product is headed. See [`GOVERNANCE.md`](GOVERNANCE.md) for how decisions
get made, [`MAINTAINERS.md`](MAINTAINERS.md) for who makes them, and
[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) for the expected conduct.

## Before you start

- Skim [`docs/architecture.md`](docs/architecture.md) for how the pieces fit
  together (tracer → IR → exporters → CLI → cluster objects) and
  [`docs/roadmap.md`](docs/roadmap.md) for what's already built and why, in
  the order it was built.
- For anything non-trivial (a new exporter, a new flag, a behavior change),
  open an issue or a draft PR describing the approach before writing a lot of
  code — this project has a strong "confirm the nuance before building"
  habit (see how many `docs/roadmap.md` entries start with "confirmed via
  real source/live testing"); it's cheaper to align early.
- Small, focused PRs over large ones. One exporter, one bug fix, one flag —
  not a grab-bag.

## Development setup

- Go, per [`go.mod`](go.mod). `go build ./...` works on macOS/Windows too —
  `internal/tracer.Trace()` compiles to a stub there (the Inspektor Gadget
  Go SDK is Linux-only), so cross-platform contributors can still build and
  work on everything except the tracer itself.
- For anything touching `internal/tracer` or needing a real cluster (RBAC,
  CRDs, live `trace` runs), you need a Linux box with kernel ≥ 6.8 and a
  `kind` cluster with Inspektor Gadget — see
  [`HOW_TO_START.md`](HOW_TO_START.md) for the full VM/cluster
  setup (French version: [`COMMENT_COMMENCER.md`](COMMENT_COMMENCER.md)),
  or `make init-vm`/`make check-kernel`.
- No cluster available? `make docker-test` runs the real Linux build/test
  (including `internal/tracer`) in `Dockerfile.dev`, without needing a VM or
  cluster — the closest local equivalent to CI for the parts that don't need
  a live cluster.

## Before opening a PR

```bash
go build ./...
GOOS=linux go build ./...   # internal/tracer only compiles for real on Linux
gofmt -l .                  # must print nothing
go vet ./...
go test ./...
```

All of these are exactly what `.github/workflows/ci.yml`'s `build-and-test`
job runs — matching it locally before pushing saves a round trip. The
`security` job (`gosec`, Trivy) runs too; both `build-and-test` and
`security` are required checks on `master`. Run `gosec ./...` locally
(`go install github.com/securego/gosec/v2/cmd/gosec@latest`) before
pushing anything that touches conversions, file paths, or subprocess
calls — it's fast and catches this class of bug before CI does.

## Code conventions

- **No comments explaining *what* the code does** — names should carry that.
  Comments exist for the *why*: a non-obvious constraint, a real bug a test
  caught, a decision made after checking real upstream source instead of
  guessing. Skim any file under `internal/` for the tone — comments here
  routinely cite the exact source/version confirmed, or the specific test
  that caught a bug, rather than asserting from theory.
- **Confirm, don't guess, against real schemas.** When generating a
  manifest for another project's CRD (PodLock, security-profiles-operator,
  ...), verify field names/behavior against that project's actual source or
  docs — several bugs in this codebase's history came from an initial guess
  that turned out wrong (see `docs/roadmap.md`'s entries on `pkg/podlock`
  and `pkg/spo`).
- **Only report what was actually observed.** Exporters never infer "safe
  defaults" (e.g. `runAsNonRoot`, `privileged`) for something that wasn't
  seen during a training run — see `docs/policy-synthesis.md`.
- **Never auto-apply anything.** The CLI stops at writing YAML / publishing
  a review object; it never calls `kubectl apply` itself. Any new feature
  that touches the cluster should stay read-only unless there's a very
  strong, explicit reason otherwise (see how `--restart`'s write access is
  deliberately isolated into its own opt-in RBAC manifest,
  `docs/threat-model.md` §1).

## Commit messages

This repo uses Conventional-Commits-style subjects:
`type(scope): imperative summary`, e.g.
`fix(k8s): strip nodeName from patched bare-pod manifests`,
`feat(exporter): add the seccomp backend`,
`docs: record live confirmation of the restart fix`. Explain *why* in the
body when it's not obvious from the subject — `git log` is itself part of
this project's documentation trail.

## Releases

Automated via [release-please](https://github.com/googleapis/release-please)
(`.github/workflows/release-please.yml`), which reads the Conventional
Commits above to keep a standing "Release vX.Y.Z" PR up to date with the
next version number and changelog. Merging that PR creates the real tag,
which triggers `.github/workflows/release.yml` (cross-platform binaries
via goreleaser, the Helm chart pushed to GHCR as an OCI artifact — see
[`INSTALL.md`](INSTALL.md)).

**Only counts commits that land via a merged PR** — the workflow trigger
is `pull_request: types: [closed]` filtered to `merged == true`, not a
plain push to `master`. A commit pushed directly to `master` (bypassing
review) won't appear in the next release PR until an actual PR gets
merged. Deliberate: a release should only ever account for reviewed
work, not whatever happened to reach `master` however it got there.

**The decision is driven by the PR title, not the commits inside it.**
Squash-merge is the only merge method allowed on this repo, with the
squash commit's subject forced to the PR title
(`squash_merge_commit_title: PR_TITLE`) — so master only ever gains one
commit per PR, and that commit's message is exactly the PR title. Give
the PR title itself a Conventional Commits subject (`fix(doc): ...`,
`feat(exporter): ...`) — individual in-PR commit messages can be looser
(the local `.githooks/commit-msg` hook still checks each one, but that's
about commit hygiene during review, not what release-please reads after
merge). `.github/workflows/pr-title-lint.yml` enforces this on the PR
title itself before merge, same type list as the local hook.

**The release PR also bumps the "current version" mentions in
`INSTALL.md`, `README.md`, `demo/script.md`, and `book/src/index.md`** —
configured via `extra-files` in `release-please-config.json`. Those
files carry `<!-- x-release-please-start-version -->` /
`<!-- x-release-please-end -->` (or single-line
`<!-- x-release-please-version -->`) HTML-comment markers; anything
between a start/end pair gets its old-version string swapped for the new
one automatically. If you add another doc mentioning the current tag,
either wrap it in the same markers or add it to `extra-files` — don't
hand-edit the version there, it'll just get overwritten by the next
release PR. This exists specifically so the tag never again points at a
commit whose own docs haven't caught up yet.

### Release certification

Passing CI is not release authorization. Before a release is authorized,
**Core E2E, SPO Interop E2E and SPO D-MIN E2E must each have passed on the
exact RC SHA** —
the commit the tag points at, not an ancestor and not "the branch was green
last week". The rule and its rationale are in
[`docs/PROGRESS.md`](docs/PROGRESS.md); this is how to satisfy it.

SPO Interop E2E is deliberately not a required per-PR check, so this step is
where it is enforced.

1. Identify the RC SHA:
   `git rev-list -n1 <tag>` — that value is what everything below is
   checked against.
2. Get a run on that SHA. Merging the release PR produces the RC commit on
   `master` and a tag at the same commit, and both trigger the workflow, so
   normally the run already exists. Otherwise dispatch it:
   `gh workflow run spo-e2e.yml --ref <tag>`. Dispatch takes a branch or
   tag name — a bare commit SHA is not a valid ref, which is exactly why
   step 3 is not optional.
3. Verify the run actually ran on the RC SHA rather than on whatever the ref
   resolved to at the time:
   `gh run view <run-id> --json headSha,conclusion`.
   Both `headSha == RC SHA` and `conclusion == success` are required.
4. Repeat 1–3 for Core E2E and for SPO D-MIN E2E (`spo-dmin-e2e.yml`, which
   runs on a real-node k3s cluster rather than kind).
5. Only then authorize the release (`gh workflow run release.yml -f
   tag=<tag>`, which is the manual path release-please's tags require —
   see the anti-recursion note in `.github/workflows/release.yml`).

If SPO Interop E2E cannot pass on the RC SHA, the release may still proceed
only by dropping the SPO interoperability claim from that release's notes.
Shipping the claim without the evidence is not an option.

## Testing expectations

- New behavior needs a test. This codebase has repeatedly caught real bugs
  this way (a `status: {}` leak, a stale RBAC assumption, a missing
  `nodeName` strip) — write the test that would have caught the bug you're
  fixing, not just one that exercises the happy path.
- `internal/k8s` tests use `k8s.io/client-go/kubernetes/fake`; CRD-backed
  packages (`internal/proposal`, `internal/history`) use
  `k8s.io/client-go/dynamic/fake`. Reuse existing fixture helpers in the
  matching `_test.go` file before writing new ones.
- No live cluster in CI — anything that needs one is a manual, documented
  VM verification step (see the "confirmed live" entries throughout
  `docs/roadmap.md` and `docs/e2e-demo.md`), not an automated test.

## Licensing

Dual-licensed, contributor's and recipient's choice:
[Apache-2.0](LICENSE-APACHE) or [MIT](LICENSE-MIT) — see
[`COPYRIGHT`](COPYRIGHT). By contributing, you agree your changes are
licensed under the same terms.

## Sign off your commits (DCO)

Every commit must carry a `Signed-off-by` trailer certifying you wrote it
(or otherwise have the right to submit it under this project's license) —
the [Developer Certificate of Origin](DCO.md), the same mechanism the
Linux kernel and most CNCF projects use. It's about provenance, not a
transfer of your copyright: you keep it.

```bash
git commit -s -m "fix(k8s): strip nodeName from patched bare-pod manifests"
```

`-s` appends the trailer automatically, using your configured `user.name`/
`user.email`:

```
Signed-off-by: Jane Doe <jane@example.com>
```

Missing sign-off on an existing commit: `git commit --amend -s` (last
commit) or `git rebase --signoff <base>` (a range).

## Where to start

Look for issues labeled `good first issue`. If nothing's labeled yet, open
an issue describing what you'd like to work on — a small, well-scoped
exporter gap or a missing test is always a safe place to start; see
[`docs/roadmap.md`](docs/roadmap.md)'s milestones for what's built and what
isn't yet.

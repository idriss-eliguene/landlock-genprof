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
  [`HOW_TO_START.md`](HOW_TO_START.md) (French) for the full VM/cluster
  setup, or `make init-vm`/`make check-kernel`.
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

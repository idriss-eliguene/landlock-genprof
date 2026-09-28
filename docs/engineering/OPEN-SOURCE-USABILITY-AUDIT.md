# LANDLOCK-GENPROF Open-Source Usability, Adoption and Contributor Experience Audit

Audit date: 2026-09-24  
Audited baseline: `origin/master` at `c2b2d8c332456e30cd13f88be4c3d32ca23babe7`  
Current release tag: `v0.10.0` at the same commit  
Audit worktree: detached, isolated from PR #265  
Concurrent PR #265: open, head `a56193be7daa7d088719fa69ad585155206e4d52`, mergeable but blocked by required review

## Executive summary

The project has unusually strong product-boundary documentation, explicit
security caveats, a reproducible mdBook, an extensive Makefile, and a good
unit/envtest/frontend test base. A technically experienced Go/Kubernetes user
can build the CLI and run the documentation and frontend suites from a clean
checkout.

It is not currently release-ready as a first-time-user product because the
published release and demonstration entry points are broken:

* GitHub release `v0.10.0` exists but has zero assets.
* Both README demonstration MP4 links return HTTP 404.
* The demo release currently contains `landlock-genprof-george-1.25x.mp4`,
  while the README links to `landlock-genprof-operations-center-george.mp4`.
* The documented `make test` target fails in a clean checkout because it
  discovers ignored generated `book/dist` Go packages whose relative paths do
  not work from that directory. Two additional tests require local socket
  creation and fail under the audit sandbox.
* A new contributor must choose among several overlapping bootstrap lanes and
  infer which dependencies are mandatory for CLI-only, frontend, envtest,
  kind, SPO, Cilium, and real-node testing.

The highest-value corrections are release-asset publication/link validation,
a narrow unit-test target that excludes generated documentation, and a single
decision-oriented contributor quick start with explicit platform lanes.

## Personas and audit scope

| Persona | Needed outcome | Result |
|---|---|---|
| CLI user with an existing cluster | Install, diagnose, generate and inspect a proposal | PARTIAL |
| Kubernetes platform operator | Install chart/RBAC/CRDs and understand optional integrations | PARTIAL |
| Operations Center evaluator | Run the UI with trusted proxy, secrets and executor identity | BLOCKED without substantial setup |
| Go contributor | Build, unit-test and make a focused change | PASS for targeted work; `make test` FAILS |
| Frontend contributor | Install npm dependencies, lint, test and build independently | PASS |
| Security/integration contributor | Run envtest and disposable kind/E2E lanes | PASS/BLOCKED depending on environment |

## Installation and first-use journey

### Public entry points

`README.md`, `INSTALL.md`, `HOW_TO_START.md`, `docs/test-environment.md`, the
mdBook, and the Helm chart README are all present. Local-link scanning found no
missing relative links in README, INSTALL, or CONTRIBUTING.

The purpose and limits are unusually clear: the project separates observed
evidence, generated candidates, approval, application, and behavioral
verification. The README explicitly avoids claiming kernel enforcement.

### Documented methods

1. `go install ...@v0.10.0` followed by raw manifests.
2. GitHub release binary followed by raw manifests.
3. Source build with `make install-plugin`.
4. Local or OCI Helm chart.
5. Disposable kind+Cilium/Lima bootstrap with `hack/bootstrap.sh`,
   `make env-doctor`, and `make test-env`.

The methods are technically comprehensive but not presented as a short
decision tree. The user must understand that Inspektor Gadget is mandatory for
runtime tracing, while SPO, PodLock, Cilium, and the Operations Center are
separate integrations. The current docs say this, but the distinction is
spread across multiple documents.

### Reproduction evidence

| Attempt | Command | Result |
|---|---|---|
| Build CLI | `GOCACHE=/tmp/lg-audit-cache go build -o /tmp/landlock-genprof-audit ./cmd/landlock-genprof` | PASS |
| Root help | `/tmp/landlock-genprof-audit --help` | PASS; 22 top-level commands listed |
| Version | `/tmp/landlock-genprof-audit version` | PASS, but source build reports `dev (commit none, built unknown)` without Makefile ldflags |
| Invalid command | `... definitely-not-a-command` | PASS; exit 1 and useful error |
| Kernel check | `make check-kernel` on Darwin arm64 | PASS with warnings; cannot establish Linux node/eBPF behavior |
| Existing-cluster diagnostic | `make env-doctor` | FAIL/BLOCKED: Docker unavailable, API unreachable, six project CRDs missing |
| Project layer | `make test-env` | NOT_TESTED in this audit; it mutates a cluster and no disposable cluster was allocated |
| First live trace | documented `kubectl landlock-genprof trace ...` | NOT_TESTED; requires Inspektor Gadget and a configured cluster |

The documented first useful result is therefore not attainable from a clean
macOS host without the bootstrap lane or an already prepared cluster. The
docs explain the dependency, but do not provide a lightweight offline example
that generates and inspects a candidate without Kubernetes.

### Installation defects

* **P0/P1 release defect:** the recommended published `v0.10.0` binary path
  cannot work because the release has no assets. INSTALL.md still says the
  release is being prepared, which is stale after the tag was published.
* **P1 public demo defect:** README video links are 404. The actual demo
  release exposes a different filename and speed variant.
* **P2:** there is no `make uninstall` or `make verify-install`; CLI plugin
  installation is a `mv` into `$(go env GOPATH)/bin`, and PATH verification is
  left to the user.
* **P2:** there is no one-command, non-Kubernetes smoke path for the core
  proposal-generation experience.

## Kubernetes deployment journey

The deployment topology is documented accurately but is operationally dense:

* CLI identity and project CRDs/RBAC are the minimal layer.
* Inspektor Gadget must be installed separately for tracing.
* SPO and PodLock are optional backends with different prerequisites.
* Operations Center needs two pre-created Secrets, an external trusted proxy,
  an executor kubeconfig, explicit impersonation allowlists, and carefully
  configured NetworkPolicy selectors/CIDRs.
* The chart's profile-realizer identity is a distinct cluster-scoped boundary;
  it must not be conflated with namespace-local human/team access.

The chart README correctly documents secret handling, one-replica/Recreate
behavior, external proxy requirements, CRD upgrade behavior, RBAC ownership,
and uninstall retention of CRDs. This is a PASS for correctness, but a P1/P2
adoption burden because the Operations Center example is not a minimal
copy-paste deployment and does not provide a supported local proxy recipe in
the chart README itself.

No existing production or shared cluster was modified during this audit.
The previously provisioned disposable security-gate clusters had already
been removed before this audit began.

## Makefile architecture

`make help` is present and exposes the public command surface. It is useful,
but descriptions mix French and English and the target set is not grouped by
user goal.

### Inventory

| Area | Targets |
|---|---|
| Bootstrap/diagnostics | `init-vm`, `bootstrap`, `env-doctor`, `check-kernel` |
| UI/demo | `ui-lima`, `ui-lima-auth`, `ui-lima-demo`, `ui-lima-auth-test`, `operations-center-demo`, `operations-center-demo-test`, `operations-center-demo-reset`, `ui-lima-auth-release` |
| Project environment | `test-env`, `test-env-clean` |
| Go checks | `build`, `test`, `vet`, `fmt`, `envtest`, `envtest-diagnostics`, `test-all` |
| Docs/generation | `docs-cli` |
| CLI installation | `build-plugin`, `install-plugin` |
| Containers | `docker-build`, `docker-test`, `docker-shell` |
| Proposal operations | `export-proposal`, `apply-proposal`, `demo-proposal`, `demo-nginx`, `apply-nginx` |
| E2E | `e2e-cluster-create`, `e2e-install`, `e2e-preflight`, `e2e-golden`, `e2e-cluster-destroy`, `test-e2e-core` |

### Side effects and risks

* `bootstrap`, `test-env`, UI/demo targets, and E2E targets mutate or create
  Kubernetes/Lima resources.
* `test-env-clean`, `operations-center-demo-reset`, and
  `e2e-cluster-destroy` are destructive, although their documentation limits
  their ownership scope.
* `install-plugin` moves rather than copies the binary and has no inverse
  target.
* `docker-test` builds an image and needs Docker; it is not an offline test.
* `test-all` combines unit and envtest but does not represent the full CI/E2E
  matrix.

### Proposed backwards-compatible grouping

Keep existing names as aliases, but add grouped discoverability:

```text
make install              # documented CLI/plugin installation
make uninstall            # remove only the installed plugin
make verify-install       # version/help/plugin discovery
make dev-bootstrap        # alias/document the Core platform lane
make dev-doctor
make dev-down
make test-unit
make test-envtest
make test-integration
make test-e2e
make test-security
make docs-build
make lint
make generate
```

The first implementation should be documentation and target aliases, not a
renaming migration. `test-unit` must explicitly exclude `book/dist` and other
generated trees, or invoke package paths selected from tracked source.

## Contributor onboarding

CONTRIBUTING.md is strong on security boundaries, DCO, conventional commits,
test expectations, and the distinction between fake clients and real API
servers. It points to architecture and roadmap documents and mentions
`good first issue`.

Gaps:

* There is no `.github/ISSUE_TEMPLATE` or visible first-contribution issue
  path in the repository. The guidance says to look for `good first issue`,
  but does not identify a current issue or label convention.
* The contributor path has no single clean bootstrap transcript and no
  dependency table showing which targets require Linux, Docker, kind, Cilium,
  Lima, Inspektor Gadget, SPO, browser tooling, or network access.
* `go test ./...` and `make test` are presented as equivalent CI checks, but
  `make test` adds coverage and has a different failure surface.
* The frontend is independently testable, but CONTRIBUTING does not explain
  the `web/operations-center` npm commands.
* No `SECURITY.md` was found. Security reporting is not discoverable from a
  standard GitHub security-policy entry point.

## Development environment

| Component | Evidence/status |
|---|---|
| Go | `go.mod` requires Go 1.26.0; local Go 1.26.5 worked |
| macOS build | PASS for Go build; tracer uses a stub outside Linux |
| Linux tracer | Documented as requiring Linux kernel >=6.8 and Inspektor Gadget |
| kind/Lima | Documented, pinned versions in `hack/versions.env`; environment lane is heavyweight |
| envtest | PASS with `make envtest` and controller-runtime assets 1.36.2 |
| frontend | PASS after `npm ci`: lint, 41 tests, production build |
| Docker | BLOCKED in audit sandbox: Docker socket permission denied |
| SPO | Optional for core install; required for SPO-specific paths |
| Cilium | Required by the canonical Core network-policy lane, not by unit tests |

The repository supports isolated CLI package work, envtest work, frontend
work, and documentation work. It does not yet make those lanes obvious to a
new contributor.

## Testing matrix

| Suite | Exact command | Dependencies / side effects | Result |
|---|---|---|---|
| Go package tests | `go test ./...` | Go/cache; no cluster intended | PASS in isolated master worktree |
| Coverage tests | `make test` / `go test -cover ./...` | Go; discovers ignored `book/dist` | FAIL: relative chart README path and sandbox socket test |
| Build | `make build` | Go | PASS |
| Vet | `make vet` | Go | PASS |
| Formatting | `make fmt` | Go | FAIL: two tracked test files reported unformatted |
| Authoritative envtest | `make envtest` | Downloads envtest assets; local API server | PASS |
| Diagnostics | `make envtest-diagnostics` | Explicitly non-authoritative tests | NOT_TESTED |
| Helm | `helm lint deploy/helm/landlock-genprof` | Helm only | PASS; icon recommendation warning |
| Docs | `mdbook build book` | mdBook + Mermaid preprocessor | PASS; version warning |
| Docs tests | `mdbook test book` | mdBook | PASS |
| Frontend lint | `cd web/operations-center && npm run lint` | Node/npm | PASS |
| Frontend tests | `npm test -- --run` | Node/npm | PASS, 11 files/41 tests |
| Frontend production build | `npm run build` | Node/npm | PASS |
| Docker equivalent | `make docker-test` | Docker daemon, network | BLOCKED: daemon socket permission denied |
| Host doctor | `make env-doctor` | Docker, kubeconfig, cluster, project CRDs | FAIL/BLOCKED: API/Docker unavailable and six CRDs missing |
| Kernel check | `make check-kernel` | Host kernel; Linux evidence preferred | PASS with Darwin/eBPF warnings |
| Core E2E | `make test-e2e-core` | kind+Cilium, Gadget, plugin, live workload | NOT_TESTED |
| SPO/real-node E2E | workflow-driven | privileged Linux/k3s/real node | NOT_TESTED locally |

The `book/dist` issue is a reproducible test-discovery design defect: the
directory is ignored generated output but contains copied Go test packages.
Running from that package makes `helm/landlock-genprof/README.md` resolve
relative to `book/dist/deploy`, not the repository root. This should be fixed
by package selection/exclusion, not by editing generated output.

The two `make fmt` findings are existing source-format findings:
`internal/history/receipt_concurrency_test.go` and
`internal/observation/domain/observation_test.go`.

## Documentation consistency

PASS findings:

* README, INSTALL, chart README, and mdBook consistently distinguish generated,
  applied, and behaviorally verified policy.
* Local relative links in README, INSTALL, and CONTRIBUTING resolve.
* Helm CRD upgrade/uninstall retention behavior is documented.
* The chart documents that the trusted proxy is external and that the browser
  is not governance authority.

FAIL/P1/P2 findings:

* Release documentation is stale: INSTALL says v0.10.0 is still being
  prepared while GitHub shows a published tag with no assets.
* README’s product-demo links are stale and both return 404.
* README still has historical v0.8.1 emphasis above the current v0.10.0
  release context, which may confuse a new evaluator about supported artifact
  versions.
* The CLI reference is generated and mdBook builds cleanly, but the top-level
  install flow does not link to an executable verification command such as
  `landlock-genprof version` with expected output.
* No standard GitHub `SECURITY.md` or issue templates were found.

## Prioritized remediation backlog

### P1 — major adoption blockers

1. Publish verified v0.10.0 binary/checksum assets or correct the documented
   release install path. Add a release-asset smoke check to CI.
2. Replace both README video URLs with the actual public asset URL and add a
   link check that validates the release asset name.
3. Fix `make test` package discovery so a clean contributor checkout does not
   fail in ignored generated `book/dist` packages.
4. Add a single `docs/quickstart.md` or equivalent decision tree for CLI-only,
   existing-cluster, disposable kind, and Operations Center lanes.

### P2 — significant usability improvements

1. Add `test-unit`, `test-envtest`, `test-integration`, `test-e2e`,
   `test-security`, `docs-build`, and `verify-install` aliases while retaining
   old targets.
2. Add `make install`, `make uninstall`, and `make verify-install` with safe,
   user-owned paths and explicit PATH diagnostics.
3. Add frontend commands to CONTRIBUTING and document `npm ci`/lint/test/build.
4. Add `SECURITY.md`, issue templates, and a current `good first issue` path.
5. Add a small offline example or fixture-driven first-use path that does not
   require a live cluster.
6. Add explicit environment/privilege/network/duration/cleanup metadata to the
   testing documentation.

### P3 — polish

1. Standardize `make help` language and group output.
2. Add expected durations and troubleshooting links to make targets.
3. Add release/version context near the README top and retire stale historical
   wording from the active install path.
4. Add a docs link checker for both local links and release URLs.

## Proposed implementation phases

1. **Release correctness:** publish/verify v0.10.0 assets, repair README demo
   link, add release and documentation URL smoke tests.
2. **Test taxonomy:** exclude generated directories, add focused Make aliases,
   and document the authoritative versus diagnostic suites.
3. **Contributor quick start:** one platform decision tree, dependency table,
   first useful offline result, and frontend lane.
4. **Kubernetes adoption:** provide a minimal CLI-only chart/manifests path and
   a separate, explicit Operations Center/trusted-proxy deployment guide.
5. **Community hygiene:** add security policy, issue templates, first-issue
   guidance, and release upgrade/uninstall verification.

## Finding classification summary

* **P0:** none observed.
* **P1:** empty v0.10.0 release assets; broken README demo links; `make test`
  failure for a clean checkout; fragmented first-time setup.
* **P2:** missing install verification/uninstall targets; frontend lane absent
  from contributor guide; no security policy/issue templates; heavy first-use
  path without offline example.
* **P3:** mixed Makefile language, stale historical emphasis, missing expected
  durations and URL smoke checks.

## Remaining unknowns

* A clean Linux VM with Docker/kind/Cilium was not provisioned for this audit;
  Core E2E, Inspektor Gadget tracing, SPO paths, and real-node behavior are
  NOT_TESTED here.
* The published v0.10.0 release has no binary assets, so cross-platform
  installation cannot be validated against a downloadable artifact.
* Operations Center browser qualification was not rerun because its required
  cluster, proxy, secrets, and image publication environment were unavailable.

## Overall assessment

The engineering and security model is substantially clearer than the average
Kubernetes project, and the source-level testability is good. Adoption is held
back primarily by release/publication correctness and by the absence of a
single, low-friction path through the multiple supported environments.

The recommended first improvements are release asset/link repair, a clean
unit-test target, and a concise contributor decision tree. These can be
delivered without changing security-sensitive product code.

---

## 2026-09-28 gap-analysis update

Audit date: 2026-09-28
Baseline before this pass: `HEAD` at `a755bb8` (20 commits after the audit
above, including PR #275 "add version-targeted contributor bootstrap" and
PR #268 "add contributor quickstart and security policy")
Scope: this pass re-verifies the 2026-09-24 backlog against the current
checkout and applies small, safe documentation/CI fixes. It intentionally
did **not** provision a new disposable Kind/Lima environment (`make dev-up`)
— this host already has live, non-disposable-looking infrastructure
(Lima VM `landlock-genprof-core`, running; Kind clusters `landlock-genprof`,
`landlock-genprof-core`, `landlock-genprof-dev-f25d5480a64f`,
`landlock-genprof-proposal-bootstrap`) that is not this pass's to touch or
duplicate against. A full `dev-up`/`dev-status`/`dev-test`/`dev-e2e`/`dev-down`
fresh-environment cycle, and any Linux amd64 or truly clean-clone test, remain
unverified by this pass — see "Not verified this pass" below.

### Unverified premise

This pass was briefed on a story of a recent external contributor on Apple
Silicon/Lima/Ubuntu ARM64/Kind/Inspektor Gadget whose capture "exposed an
upstream Seccomp architecture defect." No issue, PR, commit, or doc in this
repository was found to corroborate that specific incident (checked `gh issue
list`, `gh pr list`, and grepped docs for arm64/seccomp-architecture wording).
The one currently open external PR (#283, contributor `rdout2`) is an
unrelated unit-test contribution. This finding is recorded as **unverified**
rather than assumed true, per the audit's own evidentiary standard.

### 2026-09-24 backlog re-verified against current `HEAD`

| # | 2026-09-24 finding | Status now | Evidence |
|---|---|---|---|
| P1-1 | Published `v0.10.0` release has zero binary assets | **Still open** | `gh release view v0.10.0 --json assets` → `"assets":[]`, confirmed 2026-09-28. `INSTALL.md` now states this honestly ("The published v0.10.0 release currently has no binary assets. Do not invent download URLs...") instead of the stale "being prepared" text — the *documentation* defect is fixed, the *release* defect is not. Publishing assets is a release action outside this pass's safe-fix scope. |
| P1-2 | README demo video links 404 / filename mismatch | **Fixed** | README now links `landlock-genprof-george-1.25x.mp4` in the `demo-operations-center-george` release; `curl -I -L` on that URL returns `200`. |
| P1-3 | `make test` fails in a clean checkout on generated `book/dist` packages | **Fixed** | `test-unit`/`build`/`vet` now do `go list ./... \| grep -v '/book/dist/'`; `make test-unit` passes cleanly (all packages `ok`, verified this pass). |
| P1-4 | No single contributor decision tree | **Fixed** | `docs/CONTRIBUTOR-QUICKSTART.md` (5 lanes: CLI-only, existing cluster, disposable kind+Cilium, frontend, security/integration) and `docs/engineering/DEVELOPMENT-ENVIRONMENT.md` (the `dev-doctor`/`dev-up`/`dev-status`/`dev-test`/`dev-e2e`/`dev-down` lifecycle) now exist and are linked from `CONTRIBUTING.md`. |
| P2 | Missing `test-unit`/`test-envtest`/`test-integration`/`test-e2e`/`test-security`/`docs-build`/`verify-install` aliases | **Fixed** | All present in `Makefile`, confirmed via `make help`. |
| P2 | No `make install`/`uninstall`/`verify-install` | **Fixed** | Present; `install`/`uninstall` are ownership-gated (manifest + sha256 check before removal). |
| P2 | Frontend npm commands absent from CONTRIBUTING | **Fixed** | `CONTRIBUTING.md` documents `npm ci`/`lint`/`test`/`build` under `web/operations-center`. |
| P2 | No `SECURITY.md` / issue templates / current good-first-issue path | **Fixed** | `SECURITY.md` exists; `.github/ISSUE_TEMPLATE/{bug-report,feature-request}.yml` exist; three open issues carry `good first issue` (#92, #94, #137). |
| P2 | No offline example / fixture-driven first-use path | **Largely fixed** | `examples/nginx-generated-*` (profile, proposal, networkpolicy, seccomp, seccompprofile, capabilities, securitycontext, report) are referenced throughout README per-flag, so a newcomer can see real output shape with no cluster. Not an interactive offline generator, but the stated need (see real output without infra) is met. |
| P2 | No environment/privilege/network/duration/cleanup metadata in testing docs | **Largely fixed** | `CONTRIBUTOR-QUICKSTART.md` gives Host/Dependencies/cleanup per lane; `DEVELOPMENT-ENVIRONMENT.md` has a dedicated "State, safety and cleanup" section and troubleshooting table. Expected *durations* per target are still absent (P3, not addressed this pass). |
| P3 | `make help` mixes French and English | **Fixed this pass** | Translated the remaining French `## ...` help text and adjacent runtime echo strings in `check-kernel`, `fmt`, `docs-cli`, `build-plugin`, `install-plugin`, `docker-build`, `docker-test`, `docker-shell`, `export-proposal`, `apply-proposal`, `demo-proposal`, `demo-nginx`, `apply-nginx`. `make help` output is now fully English; re-ran `make test-unit` after editing to confirm no behavior changed (help text and echo strings only). |
| P3 | `make help` not grouped by purpose | **Fixed** | Output is now grouped into `[Installation]`, `[Development environment]`, `[Tests and quality]`, `[Documentation and generation]`, `[UI and demos]`, `[Proposal operations]`, `[Other]`. |
| P3 | Stale v0.8.1 emphasis above current release at README top | **Fixed** | README now opens directly with the product pitch and version-agnostic framing; no historical version emphasis found at the top. |
| P1-rec/P3 | No release-asset / docs link smoke check in CI | **Fixed this pass** | `hack/check-public-assets.sh` existed on disk (added since the 2026-09-24 audit) but was not wired into any `.github/workflows/*.yml` — confirmed via `grep -rl check-public-assets .github/workflows/` returning nothing before this pass. Added a `public-assets-check` job to `.github/workflows/ci.yml` (read-only HTTP check, runs on push/PR/dispatch, no secrets, does not touch `release.yml`/`release-please.yml`/`release-recovery.yml`). Verified the script passes standalone (`./hack/check-public-assets.sh` → exit 0) and that the edited `ci.yml` is valid YAML. |

### Validation run this pass (non-mutating, no new cluster/VM created)

| Check | Command | Result |
|---|---|---|
| Contributor doctor | `make dev-doctor` | `DEV_DOCTOR=PASS` (Docker context `lima-landlock-genprof-core`, 12 CPU/24GiB/32GiB free reported) |
| Unit tests | `make test-unit` | PASS, all packages `ok`, `book/dist` correctly excluded |
| Formatting | `gofmt -l .` (excluding `book/dist`) | Clean, no output |
| Vet | `make vet` | Clean, no output |
| mdBook build | `mdbook build book` | PASS (pre-existing mdbook-mermaid version-mismatch warning only) |
| mdBook tests | `mdbook test book` | PASS |
| Helm lint | `helm lint deploy/helm/landlock-genprof` | PASS (pre-existing icon-recommendation notice only) |
| Frontend lint | `npm run lint` (`tsc --noEmit`) | PASS |
| Frontend tests | `npm test -- --run` | PASS, 17 files / 60 tests |
| Frontend build | `npm run build` | PASS |
| Public asset link check | `./hack/check-public-assets.sh` | PASS (exit 0) |
| Working tree cleanliness | `git status --short` after all of the above | Only the pre-existing untracked docs and this pass's two intentional edits — no generated `book/dist`/frontend `dist` pollution, confirming `.gitignore` coverage |

### Not verified this pass (explicitly out of scope by user decision)

- A true fresh-clone test in an isolated directory (this pass validated the
  existing checkout, not a `git clone` into a clean location).
- `make dev-up`/`dev-status`/`dev-test`/`dev-e2e`/`dev-down` end-to-end —
  would create a new disposable Kind+Cilium+Gadget cluster; deferred to avoid
  adding infrastructure alongside what's already running on this host.
- Native Linux amd64/arm64 onboarding.
- `make envtest`, `make test-e2e-core`, and any SPO/PodLock live-cluster
  qualification (previously PASS/BLOCKED per the 2026-09-24 audit; not
  rerun here since they either need a live cluster or were already
  authoritatively covered by CI).

### Maintainer decisions required

1. **Publish `v0.10.0` binary assets** (or explicitly retire the "download a
   binary" install path in favor of `go install`/Helm only). This is the one
   remaining P1 from 2026-09-24 and is a release action, not a docs/bootstrap
   fix — out of scope for this pass by design (constraints: do not alter
   release credentials or workflows).
2. **Expected-duration annotations** on `dev-up`/`test-e2e-core`/etc. were
   deprioritized this pass as P3 polish; confirm whether that's still wanted.
3. **The ARM64/Lima/Seccomp-architecture-defect contributor story** in this
   pass's brief could not be corroborated in-repo (see "Unverified premise"
   above) — if it refers to a real incident, point this pass at the issue/PR/
   log so it can be folded into the audit with real evidence instead of being
   dropped.

No product code, release tag, or release workflow was touched by this pass.
Changes are limited to `Makefile` (help-text/echo translation only, no
logic changes) and `.github/workflows/ci.yml` (new read-only CI job).

---

## Lima bootstrap review (same 2026-09-28 pass)

Scope: the Lima VM/Docker-context/Kind integration behind `make bootstrap`
and the `dev-*` lifecycle (`hack/bootstrap.sh`, `hack/dev-env.sh`,
`hack/dev-env-test.sh`). Static review plus non-destructive, read-only
inspection of the existing `landlock-genprof-core` Lima VM (already running
on this host from prior work) — no Lima VM, Docker context, or Kubernetes
cluster was started, stopped, reset, deleted, or reconfigured by this
review.

### 1. Inventory

There is no standalone Lima YAML template checked into this repository.
`hack/bootstrap.sh`'s `setup_lima()` calls `limactl start --name
landlock-genprof-core --arch <native> template://docker-rootful`, one of
`limactl`'s own built-in templates (rootful Docker on a stock Ubuntu LTS
guest). Architecture selection is host-native (`aarch64` on Apple Silicon,
`x86_64` on Intel/amd64) via `HOST_ARCH`/`LIMA_ARCH` — confirmed correct by
inspection and by the live VM's actual `vz`/`aarch64` config.

Other pieces: `hack/dev-env.sh` (the version-targeted `dev-*` wrapper around
`hack/bootstrap.sh`), `hack/dev-env-test.sh` (its regression tests, stub-based,
non-mutating), `hack/init-vm.sh` (a thin, clean deprecation wrapper delegating
to `hack/bootstrap.sh --lane core`), `hack/versions.env` (pinned
kubectl/kind/Helm/Cilium/Gadget versions, `KIND_NODE_IMAGE` digest-pinned),
and the `ui-lima-*` scripts (UI-specific Lima flows, out of scope for this
pass — no defect looked for or found there).

### 2/3. Bootstrap correctness and resource isolation — largely strong, one confirmed defect

Read-only inspection of the existing VM confirmed: kernel `7.0.0-31-generic`
(comfortably above the documented ≥6.8 Landlock/eBPF baseline), Ubuntu 26.04
LTS, Docker `29.8.0`, 4 CPU/8GiB/100GiB allocation — all consistent with
`limactl`'s current auto-sizing and the `docker-rootful` template's own
provisioning script (confirmed by reading `~/.lima/landlock-genprof-core/lima.yaml`,
the resolved config for the already-running VM — a read of existing state,
not a change to it).

`hack/bootstrap.sh` and `hack/dev-env.sh` are unusually careful about resource
isolation: `setup_cluster()`/`up()`/`down()` refuse to adopt or delete a
same-named Kind cluster without an exact ownership-file + control-plane
container-ID match (`verify_ownership`/`control_plane_id`); `hack/dev-env.sh`
never runs `docker context use` (it explicitly refuses to switch Docker
contexts); state directories are validated against symlink escape,
cross-instance leakage, and ambiguous file ownership before any cleanup
(`validate_state_root`, `prepare_state_cleanup`). `hack/dev-env-test.sh`
already exercises most of this defensively (interrupted-cleanup recovery,
mismatched-ownership refusal, cross-instance isolation, symlink-escape
rejection) — this is genuinely strong engineering, confirmed by reading the
code and by the passing regression suite.

**Confirmed defect (documentation + script-reminder gap, not a security
hole):** on a truly fresh macOS clone, `hack/bootstrap.sh`'s `setup_lima()`
creates the `lima-landlock-genprof-core` Docker context with `docker context
create`, which registers the context but does **not** make it the shell's
active one. The `dev-*` commands (`dev-doctor`, `dev-up`, `dev-status`,
`dev-test`, `dev-e2e`, `dev-down`) — by design — check the *persisted*
`docker context show` and refuse to switch it themselves (a deliberate,
correct safety choice, not the defect). Nothing in the repository ever runs
`docker context use lima-landlock-genprof-core`. The result: every `dev-*`
command fails on first use with `macOS development requires Docker context
lima-landlock-genprof-core; refusing to switch contexts` until the
contributor discovers and runs that command themselves — a step that, before
this pass, appeared only reactively in `DEVELOPMENT-ENVIRONMENT.md`'s
Troubleshooting section, never in the Quick Start / Lane 3 happy path a
contributor would follow first. Confirmed further: Lima's own
`docker-rootful` template prints this exact `docker context use` instruction
as part of its normal `limactl start` output — but `hack/bootstrap.sh`
suppresses that message on the VM-reuse branch (`limactl start "$vm"
>/dev/null`), so a contributor re-running `make bootstrap` on subsequent
sessions never even sees Lima's own hint.

This is friction/confusion-class (P1/P2), not a silent or destructive
failure: `dev-env.sh`'s refusal is fail-closed with an already-actionable
error message naming the exact required context. It costs one avoidable
debug round-trip on first-time macOS setup, which is precisely the
"first 15 minutes" a newcomer's experience is judged on.

### 4. Contributor experience

Aside from the above, the `dev-*` commands are genuinely diagnosable:
`dev-doctor` is read-only and reports tools/resources/context/version pins
before anything mutates; `dev-status` inspects without mutation; `dev-down`
is ownership-gated and explicitly limited to the cluster/state it created
(confirmed by code and by the passing `hack/dev-env-test.sh` cross-instance
isolation tests); error messages throughout name the exact missing
tool/version/context rather than failing generically.

### 5/6. Fixes implemented this pass

1. **`hack/bootstrap.sh`**: `setup_lima()` now prints an explicit
   `docker context use lima-landlock-genprof-core` reminder whenever the
   shell's persisted context doesn't already match — on **both** the
   VM-creation and VM-reuse branches (previously only visible, inconsistently,
   via Lima's own suppressed-on-reuse message). Purely additive logging; no
   behavior change to what gets created, started, or switched.
2. **`docs/engineering/DEVELOPMENT-ENVIRONMENT.md`**: Quick Start now opens
   with the explicit macOS first-time sequence (`make bootstrap` then
   `docker context use lima-landlock-genprof-core`) before `dev-doctor`. The
   "macOS with Lima" section's previously-ambiguous "`dev-up` may start the
   existing Lima VM" claim is corrected to state precisely what `dev-up` does
   and does not do about the Docker context.
3. **`docs/CONTRIBUTOR-QUICKSTART.md`**: Lane 3 now includes the same
   `docker context use lima-landlock-genprof-core` step with a one-line
   explanation, between `./hack/bootstrap.sh` and `make dev-doctor`.
4. **`hack/dev-env-test.sh`**: two new regression assertions, stub-based and
   non-mutating (no real `limactl`/`docker`/`kind` invoked) — the reminder
   fires on the VM-reuse branch when the persisted context doesn't match, and
   stays silent when it already does. This locks in fix #1 against
   regression.
5. **`.github/workflows/ci.yml`**: wired `hack/dev-env-test.sh` into the main
   `build-and-test` job. It existed on disk (and is exactly the regression
   suite for the ownership/safety logic reviewed above) but — like
   `hack/check-public-assets.sh` before this pass — was never referenced from
   any Makefile target or CI workflow, so a regression in cluster-ownership,
   ambiguous-adoption, or symlink-escape handling would not have been caught
   automatically. Ran clean locally before wiring it in.

Two more `hack/*-test.sh` scripts were found unwired during this same sweep
(`hack/proposal-bootstrap_test.sh`, `hack/exact-sha-workflow-trigger-test.sh`,
`hack/release-gate-check-test.sh`, `hack/test-executor-lifecycle.sh`) — noted
here as a maintainer decision, not fixed in this pass to keep the diff
focused on the Lima/dev-env review's own evidence.

### Not changed

No Lima VM, Docker context, Kind cluster, or kubeconfig on this host was
started, stopped, reset, deleted, or reconfigured. No new disposable
infrastructure was provisioned — a genuine fresh-clone `make bootstrap` run
(to observe Lima's first-creation output directly, rather than infer it from
the template source and code review) would create a **new**, separately-named
Lima VM and Kind cluster; this pass did not request approval for that and did
not run it. If wanted, the proposed resources would be: one Lima VM (~4
CPU/8GiB/20GiB, native arch, `template://docker-rootful`), one Kind cluster,
Cilium and Inspektor Gadget images (network pull required), cleanup via
`limactl stop && limactl delete` and `kind delete cluster` — fully
disposable, isolated by name from `landlock-genprof-core` and the other
clusters already on this host.

### Validation (this section)

`bash -n hack/bootstrap.sh` and `bash -n hack/dev-env-test.sh` (syntax),
`bash hack/dev-env-test.sh` (full regression suite, including the two new
assertions) — PASS. `make test-unit`, `make vet`, `gofmt -l .` — re-run
clean after all edits in this pass (Lima review + the earlier safe fixes
combined). `.github/workflows/ci.yml` re-validated as well-formed YAML after
adding the new step.

# Release readiness review

Date: 2026-09-23

## Summary

**Release readiness: NOT READY.** The source checkout and most local code,
documentation, Helm, frontend, and Go validation checks pass. The published
demonstration release is not the requested artifact: its only asset is a
1.25x video, while the expected 1.35x video is not present. The v0.10.0
release also has no binary assets and its OCI Helm chart was not found.
Runtime Kubernetes and end-to-end Operations Center validation were blocked by
the absence of an available disposable cluster.

No product version, tag, release, approved video, or unrelated local file was
modified by this review.

## Baseline

| Item | Result |
| --- | --- |
| Repository | `idriss-eliguene/landlock-genprof` |
| Review branch | `fix/release-readiness-installation-review` |
| Base | `origin/master` at `edd1e6416dcc0a8adc3d51b8dd6d11deb5796b8d` |
| Working tree | Documentation changes plus this report; unrelated `DEMO_B1_REPORT.md` preserved |
| Target release | Existing `v0.10.0`; no new release created |

## Published demonstration video

**FAIL / BLOCKED.** The public release
[`demo-operations-center-george`](https://github.com/idriss-eliguene/landlock-genprof/releases/tag/demo-operations-center-george)
has one asset, `landlock-genprof-george-1.25x.mp4`. Its unauthenticated
download succeeded, but its SHA-256 is
`36926f8d4cbfceb59478ed91bd9f3ff0da538714e52453955d5c3d5006050cd7` and its
duration is `250.933333` seconds. It is not the expected 1.35x artifact.

The locally available expected artifact was independently verified:

| Property | Value |
| --- | --- |
| File | `~/Movies/landlock-genprof-george-1.35x.mp4` |
| SHA-256 | `7e06ff73213a5b8e186c8ecea32a0efdf10eb01e280949eebd10c348436b1dea` |
| Size | `56,138,780` bytes |
| Duration | `232.366667` seconds |
| Video | H.264, 1920x1080, 30 fps |
| Audio | AAC, 48 kHz stereo |

The README's current MP4 URL points to a nonexistent filename and returns
HTTP 404. It was intentionally not changed to an assumed URL. Upload attempts
were diagnosed rather than repeated blindly: GitHub API authentication and
connectivity work, but the large upload read timed out; direct upload attempts
to the upload service returned HTTP 401. The correct asset therefore remains
unpublished and the README link cannot safely be corrected in this PR.

The separate demo worktree contains the versioned production sources needed
to rebuild the film; those sources are not in this product repository. The
manual demo workflow only produces diagnostics and does not build the MP4.

## Installation review

| Documented method | Actual result | Classification |
| --- | --- | --- |
| `go install github.com/idriss-eliguene/landlock-genprof/cmd/landlock-genprof@v0.10.0` in isolated `GOBIN` | Installed successfully; executable ran | PASS |
| Source build with `go build ./cmd/landlock-genprof` | Built successfully | PASS |
| `version` after ordinary build/install | Reports `dev (commit none, built unknown)` without release ldflags, as documented | PASS |
| Download a v0.10.0 pre-built binary | Release has no binary assets | FAIL |
| `helm install ... oci://ghcr.io/... --version 0.10.0` | OCI chart not found | FAIL |
| Local Helm chart rendering and lint | `helm lint` and `helm template` pass | PASS |
| Clean disposable Kubernetes installation | No available disposable cluster; existing context was unreachable | BLOCKED |

The GoReleaser configuration declares Linux, macOS, and Windows builds for
amd64 and arm64, but only a local macOS arm64 build was exercised. Runtime
support must not be inferred from the build matrix; the trace implementation
has Linux-specific behavior.

## CLI review

The current root command inventory, obtained from source and generated help,
is:

`abi`, `apply-proposal`, `approve`, `completion`, `custody-epoch`, `diff`,
`doctor`, `evidence`, `executor`, `explain`, `export`, `help`, `observe`,
`policy`, `reject`, `review`, `rollback`, `synthesize`, `trace`, `ui`,
`verify`, and `version`, with nested inspection commands under `abi`,
`custody-epoch`, `evidence`, and `policy`.

Root help and subcommand help passed. Invalid command handling returns exit 1;
required argument validation for `trace`, `review`, and `approve` paths returns
exit 1 with actionable errors. `doctor` correctly refuses local kernel
detection on macOS unless a kernel target is supplied. `apply-proposal` help
documents the approved-digest requirement, confirmation, and stale/changed
candidate fail-closed behavior.

The Go unit and environment-test suites exercised configuration, proposal
identity, digest-bound approval, apply attempts, and failure paths. One
10-run characterization of concurrent history accumulation failed once, so
that behavior is recorded as a reproducibility concern rather than being
declared fully clean.

**Classification: PASS with a known flaky concurrency defect to investigate.**

## Kubernetes and security contracts

Helm lint, Helm rendering, and manifest inspection passed. Service accounts,
RBAC, controller resources, namespaces, and health configuration are present.
Live installation, logs, readiness, upgrade, uninstallation, and optional
Inspektor Gadget/Security Profiles Operator/NetworkPolicy integration were
**BLOCKED** because no disposable cluster was available and the existing kind
context was unreachable from this environment.

The implementation and tests preserve digest-bound approval and stale approval
rejection. The review found no basis to claim kernel-level Landlock
enforcement. Documentation and reporting must continue to distinguish
`GENERATED`, `APPLIED`, `BEHAVIORALLY_VERIFIED`, `UNKNOWN`, and
`NOT_ESTABLISHED`.

## Operations Center

`npm run lint`, `npm test` (11 files, 41 tests), and `npm run build` all pass.
The frontend's attempt ledger uses actual proposal-to-ApplyAttempt data and
renders that no attempt is associated when none exists; it does not invent an
attempt for a proposal.

Browser/API integration, workload/proposal interaction, responsive browser
inspection, and console-error checks were **BLOCKED** because the backend
cluster was unavailable. Frontend visualization is not evidence of kernel
enforcement.

## Documentation and CI audit

`mdbook build book` and `mdbook test book` pass. `helm lint` passes. Go vet
passes. `go test ./...`, `go test -race ./...`, and `make envtest` pass in a
clean source archive; the concurrent history test described above is a known
intermittent failure under repeated characterization.

The installation guide was corrected to remove stale v0.8.1/current-source
wording, correct the v0.10.0 `go install` example, and accurately state that
v0.10.0 currently has no binary assets and no discoverable OCI chart.

The workflow audit found:

- `ci.yml`, race/static-analysis jobs, frontend checks, documentation checks,
  environment tests, and release workflows are present.
- `release.yml` publishes GoReleaser binaries, checksums, container images,
  and a Helm package when a release is actually run.
- The manual demo workflow is diagnostics-only; it does not build or publish
  the George MP4.
- A reproducible product-video workflow cannot be added from this repository
  because the required R6.3 source assets are only in the separate local demo
  worktree.
- `actionlint` found only pre-existing warnings in unrelated workflows; no
  workflow was changed by this review.

**Documentation audit: PASS after the focused INSTALL.md correction.**
**Release packaging: FAIL for the currently published v0.10.0 artifacts.**

## Validation record

| Command/check | Result |
| --- | --- |
| `go test ./...` (clean source archive) | PASS |
| `go test -race ./...` (clean source archive) | PASS |
| `go vet ./...` (clean source archive) | PASS |
| `make envtest` | PASS |
| `npm run lint` | PASS |
| `npm test` | PASS, 41 tests |
| `npm run build` | PASS |
| `mdbook build book` | PASS, version warning only |
| `mdbook test book` | PASS |
| `helm lint deploy/helm/landlock-genprof` | PASS |
| `helm template landlock-genprof deploy/helm/landlock-genprof` | PASS |
| Live Kubernetes deployment | BLOCKED |
| UI integration against backend | BLOCKED |
| Correct 1.35x public video download | BLOCKED; asset upload failed |

## Final assessment

The codebase is suitable for continued integration testing, but the release is
**not ready for a new-user release claim**. The mandatory blockers are:

1. Publish and unauthenticatedly verify the expected 1.35x video, then update
   the README to its actual GitHub asset URL.
2. Publish or correct the documented binary and OCI Helm release artifacts.
3. Re-run Kubernetes and browser integration checks in a disposable cluster.
4. Investigate the intermittent concurrent history accumulation failure.

This review does not merge a PR, create a release, move a tag, or modify the
approved George master.

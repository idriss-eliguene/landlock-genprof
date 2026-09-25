# Release Qualification — Frozen Master Snapshot

## Result

This qualification covers a non-publishing snapshot from master commit `4b3be78deb12ab15c78d9f787b33bebebf93ddd9`, which includes merged PR #271. It does not use the older `v0.10.0` tag as its source and did not create a tag, release, or publication.

Overall status: **QUALIFIED FOR OWNER REVIEW WITH A REPRODUCIBILITY BLOCKER**.

The build, tests, package contents, checksums, and installation smoke tests passed. Two independent GoReleaser builds were not byte-identical because the configured build date is wall-clock time. An official release should not claim reproducible artifacts until that is corrected or explicitly accepted by the owner.

## Source custody and environment

| Item | Value |
| --- | --- |
| Source | `origin/master` |
| Frozen SHA | `4b3be78deb12ab15c78d9f787b33bebebf93ddd9` |
| Included PR | #271, merged commit `4b3be78` |
| Checkout | isolated detached audit worktree |
| Go | `go1.26.5 darwin/arm64` |
| GoReleaser | `2.12.0`, built with Go 1.25.0 |
| Git | `2.39.5` |
| Make | GNU Make `3.81` |
| Host | macOS 15.2, Darwin 24.2.0, arm64 |
| Docker | `28.4.0` |
| kubectl | client `v1.36.4`, Kustomize `v5.8.1` |
| Helm | `v3.21.4` |

The central repository and existing worktrees were preserved. The isolated qualification checkout was clean before generated build output; generated `dist/` and the locally built plugin were removed after testing.

## Release pipeline audit

GoReleaser v2 configuration builds `./cmd/landlock-genprof` with `CGO_ENABLED=0`, stripped symbols, and version/commit/date ldflags. The supported matrix is:

| GOOS | GOARCH | Format |
| --- | --- | --- |
| linux | amd64, arm64 | tar.gz |
| darwin | amd64, arm64 | tar.gz |
| windows | amd64, arm64 | zip |

The CI GoReleaser check is manually dispatchable and uses Go 1.26.5. The release workflow is tag/manual-dispatch based, gates the tag commit onto master, invokes `goreleaser release --clean`, publishes GitHub assets, and then handles container images. Release Please prepares the release PR and only its merged release branch is allowed to publish.

`goreleaser check`: **PASS**.

## Build and package evidence

Command used, with publication disabled:

```sh
goreleaser release --snapshot --clean --skip=publish
```

The snapshot version embedded in the binaries was `0.10.0-SNAPSHOT-4b3be78`; the commit metadata was the full frozen SHA. Six archives and `checksums.txt` were generated.

| Artifact | Size | Build A SHA-256 | Build B SHA-256 |
| --- | ---: | --- | --- |
| `landlock-genprof_darwin_amd64.tar.gz` | 11,773,224 | `aaaccacb5e032102f9c977092972e6657058935af847aba9f88f6511f4780b71` | `765c1365406d6e7f0d8a40fb614e16b92e94323184e146c9a99f7c6de1124a7c` |
| `landlock-genprof_darwin_arm64.tar.gz` | 10,797,857 | `1482775f04985a5fa8570ce1db06191fa787f7833f7b983188ee0eb21d4e70ae` | `3729c25c2f72a1aaf7e92d8e8c0d1b4d6bed0ac5956dbb3270e7ce0c3b9a96c5` |
| `landlock-genprof_linux_amd64.tar.gz` | 17,662,035 | `27147ff710b9bb22d86696abb7dcad20de46059659e1c38aa4fd990de58a9142` | `13ccc00ae637fd12debce32dcbde5778b616826710e195af1bafd727b6589c62` |
| `landlock-genprof_linux_arm64.tar.gz` | 15,885,015 | `9624c25a11913688bfbb0213f4bbe716827576631653f719412c5ff0a8575936` | `5c83ef7c5f06b40e0e235551fd8734628bdd3375faf1c5c8ad1b1a804e971` |
| `landlock-genprof_windows_amd64.zip` | 11,814,811 | `c1d55d58ac22a41c851b33ac92175378c5a57266eb70cef8e2165546ad7dc6bd` | `3de993022c02d38e7bc02b667fc73524d4a73609a364f0cfbbb0227b7d48bb1a` |
| `landlock-genprof_windows_arm64.zip` | 10,441,695 | `51e24f044c5b11c42c042ed6dac43228ec1258e5b7229701ae8f5313528a5745` | `9b669f72c5f59a2e24a2278867b1fb220573de2cd9523e445420a3f0c598faf8` |

All six independent checksum comparisons against `checksums.txt`: **PASS**. Each archive contained exactly the expected binary, changelog, README files, and license files. `file` confirmed Mach-O amd64/arm64, ELF amd64/arm64, and PE amd64/arm64 outputs. The native macOS arm64 binary reported the expected snapshot version and commit.

## Reproducibility

Two builds used the same frozen SHA, GoReleaser version, Go 1.26.5, separate Go caches, and the same `SOURCE_DATE_EPOCH`. Results:

| Comparison | Result | Evidence |
| --- | --- | --- |
| Six target binaries | **DIFFER** | Go metadata showed `main.date=2026-09-25T05:38:40Z` vs `05:39:47Z` |
| Six archives | **DIFFER** | Archive hashes changed with binary and archive timestamps |
| Checksums file | **DIFFER** | It correctly reflected each build's artifacts |

The cause is confirmed in `.goreleaser.yaml`: `main.date={{.Date}}`. GoReleaser used wall-clock time despite `SOURCE_DATE_EPOCH`; archive member timestamps also varied. This is a supply-chain/reproducibility finding, not a source or toolchain mismatch. No official release should be advertised as reproducible until timestamp handling is made deterministic and requalified.

## Installation and CLI qualification

| Test | Result | Notes |
| --- | --- | --- |
| Native archive extraction | **PASS** | Darwin arm64 archive extracted to a temporary directory |
| `version` output | **PASS** | Snapshot version, full commit, and build date reported |
| `--help` output | **PASS** | Commands and usage rendered |
| Isolated `kubectl plugin list` | **PASS** | Temporary `kubectl-landlock_genprof` discovered |
| `kubectl landlock-genprof version` | **PASS** | Plugin invoked through kubectl |
| `make install` | **PASS** | Explicit temporary user-owned `INSTALL_DIR` |
| `make verify-install` | **PASS** | Version/help/PATH/plugin checks passed |
| checksum manifest | **PASS** | Valid 64-character SHA-256 with `LC_ALL=C` |
| `make uninstall` | **PASS** | Only managed plugin and manifest removed |

An initial run inherited `C.UTF-8`, and Perl `shasum` failed in the execution environment, leaving an empty checksum field. This was reproduced as an environment/locale issue and passed with `LC_ALL=C`; it should remain a documented prerequisite or be hardened in a future maintenance change.

These tests qualify CLI compatibility and plugin packaging only. They do not prove Landlock kernel behavior, eBPF operation, seccomp enforcement, or Kubernetes realization.

## Repository validation

| Command | Result |
| --- | --- |
| `goreleaser check` | **PASS** |
| `git diff --check` | **PASS** |
| `make test-unit` | **PASS** |
| `make test-envtest` | **PASS** — API semantics and workbench/ownership E2E |
| `make lint` | **PASS** — format and vet |
| `make docs-build` | **PASS** — mdBook built; preprocessor version warning only |

No source, dependency, release metadata, or product files were changed by this qualification. GoReleaser's `go mod tidy` hook temporarily pruned `go.sum`; that generated audit mutation was restored and is not part of the evidence commit.

## Security and remaining risks

- **PASS:** Release workflow gates the tagged source onto master before publication.
- **PASS:** Cross-platform artifacts are statically built with `CGO_ENABLED=0`.
- **PASS:** Checksums are generated and independently verified for all six archives.
- **BLOCKED:** Byte-for-byte reproducibility is not established because build timestamps vary.
- **NOT_TESTED:** Published release download verification, because no release was created.
- **NOT_TESTED:** Linux kernel Landlock/eBPF enforcement, because this qualification ran on macOS and is a distribution audit.
- **NOT_TESTED:** Container image publication and digest verification, because publication was prohibited.

## Recommendation

Do not publish a future official release from this qualification without owner approval and a decision on the reproducibility finding. The immediate next release strategy should be: fix or explicitly govern deterministic timestamp injection; rerun the two-build hash comparison; rerun the release workflow's exact-tag gate and public asset verification; then publish only from the approved release-please tag on master. Preserve this report, both artifact sets, logs, and hashes as the qualification record.

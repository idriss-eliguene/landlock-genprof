# Release Qualification — Deterministic Master Snapshot

## Result

This qualification preserves two historical baselines and adds the post-merge master result. The historical qualification used master commit `4b3be78deb12ab15c78d9f787b33bebebf93ddd9`, which includes merged PR #271. The corrective qualification used source commit `34a9518510f04e5626afb8b0f779164997d2838a`. The post-merge qualification uses current master `2223fae589891614c33458a3bf1500973b9c7b90`, the merge commit for PR #272. None used the older `v0.10.0` tag as build source, and none created a tag, release, or publication.

Overall status: **POST-MERGE MASTER QUALIFICATION PASS — PUBLICATION STILL REQUIRES OWNER APPROVAL**.

The historical build, tests, package contents, checksums, and installation smoke tests passed, but its two independent builds differed because build date and archive mtimes were wall-clock values. The corrective build fixed those sources of nondeterminism. The post-merge master qualification reproduced the fix: two independent builds match for every executable, archive, and checksum manifest.

## Source custody and environment

| Item | Value |
| --- | --- |
| Historical source | `origin/master` at `4b3be78deb12ab15c78d9f787b33bebebf93ddd9` |
| Corrective source | `fix/deterministic-release-builds` at `34a9518510f04e5626afb8b0f779164997d2838a` |
| Post-merge source | `origin/master` at `2223fae589891614c33458a3bf1500973b9c7b90` |
| Included PR | #271, merged commit `4b3be78` |
| Deterministic-build PR | #272, merged commit `2223fae` |
| Checkout | existing main repository on `master` |
| Go | `go1.26.5 darwin/arm64` |
| GoReleaser | `2.12.0`, built with Go 1.25.0 |
| Git | `2.39.5` |
| Make | GNU Make `3.81` |
| Host | macOS 15.2, Darwin 24.2.0, arm64 |
| Docker | `28.4.0` |
| kubectl | client `v1.36.4`, Kustomize `v5.8.1` |
| Helm | `v3.21.4` |

The central repository and existing worktrees were preserved. The corrective isolated checkout was clean before generated build output; generated `dist/` and the locally built plugin were removed after testing.

## Release pipeline audit

GoReleaser v2 configuration builds `./cmd/landlock-genprof` with `CGO_ENABLED=0`, stripped symbols, and version/commit/date ldflags. The supported matrix is:

| GOOS | GOARCH | Format |
| --- | --- | --- |
| linux | amd64, arm64 | tar.gz |
| darwin | amd64, arm64 | tar.gz |
| windows | amd64, arm64 | zip |

The CI GoReleaser check is manually dispatchable and uses Go 1.26.5. The release workflow is tag/manual-dispatch based, gates the tag commit onto master, derives `SOURCE_DATE_EPOCH` from the checked-out commit, invokes `goreleaser release --clean`, publishes GitHub assets, and then handles container images. The recovery workflow now applies the same timestamp contract to its product checkout. Release Please prepares the release PR and only its merged release branch is allowed to publish. The corrective configuration preserves this workflow and the six-target matrix.

`goreleaser check`: **PASS**.

## Corrective build and package evidence

Command used, with publication disabled:

```sh
goreleaser release --snapshot --clean --skip=publish
```

The corrective snapshot version embedded in the binaries was `0.10.0-SNAPSHOT-34a9518`; the commit metadata was the full corrective SHA and `main.date` was `2026-09-25T06:00:06Z`, the commit timestamp. Six archives and `checksums.txt` were generated.

| Artifact | Size | Build A SHA-256 | Build B SHA-256 |
| --- | ---: | --- | --- |
| `landlock-genprof_darwin_amd64.tar.gz` | 11,773,266 | `3581a28fddf0c82b56333235ad108fa98a73320e6aa4868669949b892efe6f58` | `3581a28fddf0c82b56333235ad108fa98a73320e6aa4868669949b892efe6f58` |
| `landlock-genprof_darwin_arm64.tar.gz` | 10,797,845 | `e36378194fc44224869e7c2fb2cd1534ad97bfcf7cc1bab977e1d7441e23c5a2` | `e36378194fc44224869e7c2fb2cd1534ad97bfcf7cc1bab977e1d7441e23c5a2` |
| `landlock-genprof_linux_amd64.tar.gz` | 17,661,995 | `63c7017351b4fb8d3d958e238fafe3798f3801526f0f931383d1034b011a452a` | `63c7017351b4fb8d3d958e238fafe3798f3801526f0f931383d1034b011a452a` |
| `landlock-genprof_linux_arm64.tar.gz` | 15,885,002 | `aad87462726000c6be0325b1bfa0742ab44265b7ec5a52b9eb4e65b3f9bebf32` | `aad87462726000c6be0325b1bfa0742ab44265b7ec5a52b9eb4e65b3f9bebf32` |
| `landlock-genprof_windows_amd64.zip` | 11,814,849 | `055ffed1427427e5bddc29898de3e304aea86330492ea0412806adc3c2e65ee6` | `055ffed1427427e5bddc29898de3e304aea86330492ea0412806adc3c2e65ee6` |
| `landlock-genprof_windows_arm64.zip` | 10,441,675 | `50be88f713c5550f053a8d6c3967fbaba17c6e9de70dc364bbe83ee4eab40a42` | `50be88f713c5550f053a8d6c3967fbaba17c6e9de70dc364bbe83ee4eab40a42` |

All six independent checksum comparisons against `checksums.txt`: **PASS**. Each archive contained exactly the expected binary, changelog, README files, and license files. `file` confirmed Mach-O amd64/arm64, ELF amd64/arm64, and PE amd64/arm64 outputs. The native macOS arm64 binary reported the expected snapshot version and commit.

## Reproducibility: historical failure and corrective pass

The historical two-build run used master `4b3be78…`, GoReleaser 2.12.0, Go 1.26.5, separate caches, and a fixed `SOURCE_DATE_EPOCH`, but the old configuration still injected wall-clock `.Date`. Results:

| Comparison | Result | Evidence |
| --- | --- | --- |
| Six target binaries | **DIFFER** | Go metadata showed wall-clock `main.date` values `05:38:40Z` and `05:39:47Z` |
| Six archives | **DIFFER** | Binary and archive timestamps varied |
| Checksums file | **DIFFER** | It correctly reflected each differing artifact set |

The corrective run used source `34a9518…`, the same pinned tools, separate caches, and `SOURCE_DATE_EPOCH=1790316006`. The configuration used `main.date={{.CommitDate}}`, `mod_timestamp={{.CommitTimestamp}}`, and explicit archive mtimes from `{{.CommitDate}}`.

| Corrective comparison | Result |
| --- | --- |
| Six target binaries | **MATCH** |
| Six archives | **MATCH** |
| `checksums.txt` contents | **MATCH** |

The exact Build A and Build B hashes are recorded in the artifact table above. No source, dependency, or toolchain divergence was observed.

## Post-merge master qualification

The following results were produced from exact master commit `2223fae589891614c33458a3bf1500973b9c7b90` using Go `1.26.5`, GoReleaser `2.12.0`, separate build caches, identical `SOURCE_DATE_EPOCH`, and the existing Build A/Build B evidence locations:

- Build A: `/private/tmp/landlock-genprof-master-build-a`
- Build B: `/private/tmp/landlock-genprof-master-build-b`
- Snapshot command: `goreleaser release --snapshot --clean --skip=publish`
- Snapshot version: `0.10.0-SNAPSHOT-2223fae`
- Embedded commit: `2223fae589891614c33458a3bf1500973b9c7b90`
- Embedded commit date: `2026-09-25T06:53:15Z`

The six-target matrix remained Linux amd64/arm64, macOS amd64/arm64, and Windows amd64/arm64. All six executable hashes, all six archive hashes, and both checksum manifests matched:

| Target | Executable SHA-256 | Archive SHA-256 |
| --- | --- | --- |
| Darwin amd64 | `fe645a2a8127d3a2462a62a11c01de7fd7571398ab2115bc4804b3da10a0ffa5` | `28059fed9e2bc8434459b05e849535d58e88ac5da95460fceca57507701e83e7` |
| Darwin arm64 | `e508f1011480cacb273903eb1defef391e0c73e6e730b1bc76bfdd8f14774e03` | `5e36e0feae6d79d81fd7ee5b6ae14c931ce06898cb29c01fbc886753f5ae3cda` |
| Linux amd64 | `c97deba904c1673729115c5c2ac68cee0c4d2664194c80d2f5b14387339f2812` | `c234f3d7ae1ccc820d5292fa234cefe71b0181d90bb0f99f58bdb23df45ac2f1` |
| Linux arm64 | `477eebbf0105c7849a9799ff3f7143382186beeb892822b5e489d5566502113c` | `c6f07875d42319057df3bfa3262500ff6535e1b8be433540275307a33ee0c5e7` |
| Windows amd64 | `973418ae8070d10fea541cdce6681e08fdc1f390f35b949c4b82f028c1ae6dbd` | `c9e4f41716b1ebb8f0805cc1c467bbc4e4b937fe383969d23da58c8025dc52fc` |
| Windows arm64 | `3342970fd8ac9dbb52d058aa2803973480cd9dddcb895b314f60ed1f3539b68f` | `8542ad0c2d81e4f16bf89c80bb7852e4bd85b9f0eddbe671c40cad4c9a86ee54` |

`checksums.txt` SHA-256 for both post-merge builds: `e4e29a727228c994ab330aadd0efc6451c1a9db3a8547a853d6674256a9a2e07`.

Post-merge regression results were all **PASS**: `goreleaser check`, `make lint`, `make vet`, `make test-unit`, `make test-envtest`, `make docs-build`, and `git diff --check`. Darwin arm64 archive extraction, binary execution, version/commit reporting, kubectl plugin discovery, `make install`, `make verify-install`, and `make uninstall` all **PASS**. Execution of Linux, Windows, and Darwin amd64 binaries was **NOT VERIFIED**.

## Installation and CLI qualification

| Test | Result | Notes |
| --- | --- | --- |
| Native archive extraction | **PASS** | Darwin arm64 archive extracted to a temporary directory |
| `version` output | **PASS** | Corrective and post-merge snapshot versions, full commit, and commit-derived build date reported |
| `--help` output | **PASS** | Commands and usage rendered |
| Isolated `kubectl plugin list` | **PASS** | Temporary `kubectl-landlock_genprof` discovered |
| `kubectl landlock-genprof version` | **PASS** | Plugin invoked through kubectl |
| `make install` | **PASS** | Explicit temporary user-owned `INSTALL_DIR`; post-merge run passed |
| `make verify-install` | **PASS** | Version/help/PATH/plugin checks passed |
| checksum manifest | **PASS** | Valid 64-character SHA-256 with `LC_ALL=C` |
| `make uninstall` | **PASS** | Only managed plugin and manifest removed; post-merge run passed |

Only Darwin arm64 was executed locally. Linux, Windows, and Darwin amd64 execution remain **NOT VERIFIED**; successful cross-compilation and archive hashing do not establish runtime compatibility on those platforms.

An initial run inherited `C.UTF-8`, and Perl `shasum` failed in the execution environment, leaving an empty checksum field. This was reproduced as an environment/locale issue and passed with `LC_ALL=C`; it should remain a documented prerequisite or be hardened in a future maintenance change.

These tests qualify CLI compatibility and plugin packaging only. They do not prove Landlock kernel behavior, eBPF operation, seccomp enforcement, or Kubernetes realization.

## Repository validation

| Command | Result |
| --- | --- |
| `goreleaser check` | **PASS** |
| `git diff --check` | **PASS** |
| `make test-unit` | **PASS on rerun** | First pass hit the pre-existing intermittent `TestObservationAPIProof_ConcurrentGenerateDistinctProposalNames` provenance race; targeted `-count=10` and the full rerun passed |
| `make test-envtest` | **PASS** — API semantics and workbench/ownership E2E |
| `make lint` | **PASS** — format and vet |
| `make docs-build` | **PASS** — mdBook built; preprocessor version warning only |

GoReleaser's `go mod tidy` hook temporarily pruned `go.sum`; that generated audit mutation was restored and is not part of the maintenance change. The only intended source change is `.goreleaser.yaml`; the two report updates reconcile the historical and corrective evidence.

The post-merge run repeated these checks from current master and passed all of them. mdBook emitted the existing mdbook-mermaid version warning but completed successfully.

## Security and remaining risks

- **PASS:** Release workflow gates the tagged source onto master before publication.
- **PASS:** Cross-platform artifacts are statically built with `CGO_ENABLED=0`.
- **PASS:** Checksums are generated and independently verified for all six archives.
- **PASS:** Byte-for-byte reproducibility was established for all six executables, six archives, and both checksum manifests in the corrective comparison.
- **PASS:** The same byte-for-byte reproducibility result was independently confirmed from post-merge master `2223fae…`.
- **FOLLOW-UP:** The first unit-suite invocation exposed an intermittent pre-existing concurrency-test failure; no product or test code was changed, targeted repetition passed, and the full suite passed on rerun.
- **NOT_TESTED:** Published release download verification, because no release was created.
- **NOT_TESTED:** Linux kernel Landlock/eBPF enforcement, because this qualification ran on macOS and is a distribution audit.
- **NOT_TESTED:** Container image publication and digest verification, because publication was prohibited.

## Recommendation

The reproducibility blocker is resolved and confirmed on post-merge master. Publication still requires owner approval and the normal release-please/tag gate. Before an official release, run the protected release workflow from the approved master tag, verify public assets and container digests, and retain both post-merge build logs and hashes as the qualification record. Reproducible packaging does not establish cross-platform runtime compatibility or Linux kernel Landlock/eBPF enforcement.

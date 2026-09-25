# Release Procedure

This procedure describes a controlled future release of LANDLOCK-GENPROF. It was initially audited at master commit `4b3be78deb12ab15c78d9f787b33bebebf93ddd9`; deterministic-build qualification was completed on the maintenance source commit recorded in the accompanying qualification report. A release must be rebuilt from the exact approved master commit; the older `v0.10.0` tag is not a valid source for a future release.

## 1. Prerequisites and custody

The release operator needs Git, GitHub CLI, Go `1.26.5`, GoReleaser `2.x` (the qualification used `2.12.0`), GNU Make, mdBook for documentation validation, and Docker/Buildx when container images are part of the release. Kubernetes tooling is required for deployment validation, not for the cross-platform CLI build.

Before building:

```sh
git fetch origin --tags
git rev-parse origin/master
gh pr list --state merged --base master
git worktree add --detach /tmp/landlock-genprof-release "$APPROVED_MASTER_SHA"
cd /tmp/landlock-genprof-release
git status --short --branch
```

Record the full commit SHA, the merged PRs it contains, Go/GoReleaser versions, OS/architecture, and the output of `git status`. Stop if the checkout is dirty, the approved SHA is not reachable from `origin/master`, or the intended source is only a release tag with newer master commits missing. Preserve the central checkout and unrelated worktrees.

Do not create or move a tag during qualification. Do not publish from a dirty tree. Retain the source SHA, logs, configuration, checksums, and approval record as release evidence.

## 2. CI and approval path

Release Please prepares the release PR and updates the manifest and release-note files. Its publish job runs only when the release-please branch is merged. The release workflow is then triggered by a `v*.*.*` tag or an explicitly authorized manual dispatch. The workflow verifies that the tag commit is on `origin/master`, runs GoReleaser, publishes GitHub assets, and builds/pushes container images.

The release operator must confirm required CI checks, review the release diff, and obtain owner approval before creating the tag or publishing. A manually built snapshot is evidence only; it is not an official release.

The publishing and recovery workflows derive `SOURCE_DATE_EPOCH` from the checked-out release commit and pass it to GoReleaser. This is a defense-in-depth measure; the GoReleaser configuration also derives its metadata and package mtimes from Git's commit timestamp.

## 3. Local snapshot qualification

Validate the checked-in configuration without publishing:

```sh
goreleaser check
GOCACHE=/tmp/landlock-genprof-gocache goreleaser release \
  --snapshot --clean --skip=publish
```

The audited configuration builds six targets:

| OS | Architectures | Archive |
| --- | --- | --- |
| Linux | amd64, arm64 | `.tar.gz` |
| macOS | amd64, arm64 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |

The binary is built from `./cmd/landlock-genprof` with `CGO_ENABLED=0`. The archives contain the binary, README files, changelog, and both licenses. GoReleaser creates `checksums.txt`.

Inspect every archive and binary before approval:

```sh
for f in dist/*.tar.gz; do tar -tzf "$f"; done
for f in dist/*.zip; do unzip -l "$f"; done
cat dist/checksums.txt
```

Verify each checksum independently and record the results. Inspect file types for every target and run `landlock-genprof version` only on a native executable. Cross-compilation proves packaging and metadata, not execution on the target operating system or kernel.

## 4. Reproducibility

Use the same source SHA and pinned Go/GoReleaser versions for two independent builds, with separate caches and retained output directories. Set the build epoch from the exact commit when invoking the tool:

```sh
export SOURCE_DATE_EPOCH="$(git show -s --format=%ct HEAD)"
GOCACHE=/tmp/landlock-genprof-gocache-a goreleaser release --snapshot --clean --skip=publish
```

The checked-in configuration now uses `{{.CommitDate}}` for `main.date`, `{{.CommitTimestamp}}` for Go module/build mtimes, and `{{.CommitDate}}` for binary and bundled-file archive mtimes. These are commit-derived values; `SOURCE_DATE_EPOCH` remains the reproducible-build convention for the surrounding toolchain and should be set to the same commit timestamp. Compare executable hashes, archive hashes, and `checksums.txt` contents independently. Retain `go version -m` output and the exact source epoch.

If any hashes differ, do not silently normalize or replace artifacts. Record the differing metadata and stop publication until the divergence is explained.

## 5. Installation qualification

For an archive installation, extract to a disposable user-owned directory, rename the binary to `kubectl-landlock_genprof`, and test:

```sh
install -m 0755 landlock-genprof "$HOME/.local/bin/kubectl-landlock_genprof"
kubectl plugin list
kubectl landlock-genprof version
```

For source installation, use the managed Make targets with an explicit user-owned destination:

```sh
make install INSTALL_DIR="$HOME/.local/bin"
make verify-install INSTALL_DIR="$HOME/.local/bin"
make uninstall INSTALL_DIR="$HOME/.local/bin"
```

Verify that installation does not overwrite an existing file without `FORCE=1`, reports a missing PATH entry, writes a checksum manifest, and that uninstall removes only the file recorded by that manifest. A locale with a usable `shasum` implementation is required by the current Makefile; the qualification used `LC_ALL=C` on macOS.

CLI version/help and kubectl plugin discovery do not demonstrate Landlock, eBPF, seccomp, or Kubernetes kernel enforcement. Runtime qualification must be performed separately on a supported Linux kernel with the required capabilities and integrations.

## 6. Publication

After owner approval and passing required CI:

1. Confirm release-please version files, changelog, tag source, and release notes.
2. Create the tag only on the approved master SHA through the authorized release process.
3. Let the protected release workflow run GoReleaser and publish assets.
4. Verify the public release page, every expected asset name, checksums, archive contents, binary version/commit metadata, and unauthenticated downloads.
5. Record container image digests separately from CLI asset checksums.

Never overwrite an existing tag or repurpose an unrelated release. Do not upload private stems, internal reports, credentials, or local build caches.

## 7. Rollback and evidence retention

Do not delete or move a published tag to conceal a faulty artifact. Mark the release as withdrawn or superseded according to project policy, publish a corrected release through the normal approval path, and preserve the faulty asset hash and diagnosis. Revert source changes in a normal reviewed commit rather than rewriting release history.

Retain the approved source SHA, repository status, tool versions, GoReleaser configuration, workflow run IDs, build logs, artifact names and sizes, per-file hashes, installation logs, CI results, publication URLs, image digests, reviewer approval, and any rollback record. Keep evidence outside GitHub releases when it contains internal or private data.

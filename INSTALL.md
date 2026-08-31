# Installing landlock-genprof

This is for installing `landlock-genprof` against a Kubernetes cluster
you already have. Don't have one yet? See
[`docs/test-environment.md`](docs/test-environment.md) — spins up a
disposable `kind` cluster and installs the CLI, in which case skip
straight to §3 below (steps 1-2 here are already done for you).

**None of this requires cloning the repo.** Every method below —
getting the CLI, installing the RBAC/CRDs — works from a released
version number alone.

<!-- x-release-please-start-version -->

**Fastest path, if you're not sure which option to pick:**

> Assumes [Inspektor Gadget](https://www.inspektor-gadget.io/) is already
> deployed on the cluster (`kubectl gadget deploy`) and `kubectl` is
> pointed at it — see [§1 — Prerequisites](#1-prerequisites) below if
> you haven't done that yet.

```bash
go install github.com/idriss-eliguene/landlock-genprof/cmd/landlock-genprof@v0.5.0

kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac.yaml
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/crd-securityprofileproposal.yaml
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-proposal.yaml
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-patched-manifest.yaml
```

That's option A in both §2 and §3 below — no clone, no Helm, works
today. The rest of this page exists for the cases that need something
different: a pre-built binary with no Go toolchain (§2 option B), Helm
instead of raw manifests (§3 option B), or building from a local clone
(§2/§3 option C). Skip straight to [§4 — First run](#4-first-run) once
either path above is done.

## 1. Prerequisites

- **Kernel version** on every node — see [`README.md`](README.md) §6 for
  the exact table (Landlock FS ≥ 5.13, Landlock network ≥ 6.4, eBPF ≥
  5.8 recommended). Check with `./hack/check-kernel.sh` if you have
  shell access to a node (needs a clone; or just read the version table).
- **[Inspektor Gadget](https://www.inspektor-gadget.io/)** already
  deployed on the cluster (`kubectl gadget deploy`) — `trace` doesn't
  work without it. This is the one hard requirement; everything else
  below is about getting the CLI itself in place.
- `kubectl`, pointed at your cluster.
- `go 1.26+` — only for the `go install` method (step 2, option A).
  Not needed for a downloaded binary (option B) or the Helm chart
  (step 3, option A/B).
- `helm` — only for the Helm chart (step 3, option A/B).

**Enforcement is separate from all of this.** Getting `landlock-genprof`
installed and running gets you profile *generation* — actually enforcing
what it generates (PodLock, a NetworkPolicy-capable CNI, SPO) is a
different set of prerequisites entirely, not all of which this project
can set up for you. See
[`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md)
before assuming the tool "isn't working" if a generated profile doesn't
seem to do anything once applied.

## 2. Get the CLI

### Option A — `go install` (recommended, no clone)

```bash
go install github.com/idriss-eliguene/landlock-genprof/cmd/landlock-genprof@v0.5.0
```

Puts `landlock-genprof` in `$(go env GOPATH)/bin` — confirmed working
end to end (fetched straight from the module proxy, no local checkout
of any kind). Swap `@v0.5.0` for `@latest` to track the newest tag
instead of pinning, or a commit hash for something unreleased.

Want it as a `kubectl` plugin instead of standalone? Same command, then
rename:

```bash
mv "$(go env GOPATH)/bin/landlock-genprof" "$(go env GOPATH)/bin/kubectl-landlock_genprof"
kubectl plugin list   # confirms kubectl sees it
```

Underscore, not dash: kubectl reads a literal `-` in a plugin's filename
as a separator between two subcommands (`kubectl-foo-bar` → `kubectl foo
bar`), not as a dash inside one word. `_` in the filename becomes `-` in
the invoked command, which is what gets you `kubectl landlock-genprof`
as a single subcommand.

`go install` doesn't inject build metadata by default, so `landlock-genprof version`
prints generic `dev` info even though this is a real tagged release —
cosmetic only, doesn't affect behavior. Pass `-ldflags` yourself for a
version string that matches the tag:

```bash
go install -ldflags "-X main.version=v0.5.0" github.com/idriss-eliguene/landlock-genprof/cmd/landlock-genprof@v0.1.1
```

### Option B — download a pre-built binary

GitHub Releases, cross-compiled for linux/darwin/windows ×
amd64/arm64 (`.goreleaser.yaml`, `.github/workflows/release.yml`) — no
Go toolchain needed at all:

```bash
# Pick your OS/arch from the release page:
# https://github.com/idriss-eliguene/landlock-genprof/releases
curl -LO https://github.com/idriss-eliguene/landlock-genprof/releases/download/<tag>/landlock-genprof_linux_amd64.tar.gz
tar -xzf landlock-genprof_linux_amd64.tar.gz
sudo install -o root -g root -m 0755 landlock-genprof /usr/local/bin/landlock-genprof
```

Confirmed working as of `v0.5.0` — six real assets on the
[releases page](https://github.com/idriss-eliguene/landlock-genprof/releases)
(`v0.5.0` itself predates this pipeline being wired up; if you're
pinning to that specific tag for some reason, use option A instead).

Same rename trick as option A above for the kubectl-plugin form.

### Option C — build from source (clone required)

Only worth it if you're modifying the code, want the kubectl-plugin
`make` target, or need a build off an unreleased commit:

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
make install-plugin   # kubectl-landlock_genprof, into $(go env GOPATH)/bin, real version via -ldflags
# or, standalone:
go build -o landlock-genprof ./cmd/landlock-genprof
```

One kubectl-plugin quirk worth knowing regardless of how you installed
it: global `kubectl` flags placed *before* the plugin name (`kubectl -n
foo landlock-genprof ...`) are **not** forwarded to the plugin — kubectl
only passes through arguments that come *after* the plugin name. Use
`landlock-genprof`'s own `-n`/`--namespace` instead.

## 3. Install the RBAC and CRDs

The tracer needs its own `ServiceAccount`/RBAC, and every run publishes
a `SecurityProfileProposal` object, so its CRD (plus more RBAC) is
mandatory too.

### Option A — raw manifests, no clone (`kubectl apply -f <url>`)

```bash
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac.yaml
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/crd-securityprofileproposal.yaml
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-proposal.yaml
# Required whenever a run composes securityContext data (commonly true
# in practice when syscalls are observed)
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-patched-manifest.yaml

# Only if you plan to use the matching flag:
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/crd-traininghistory.yaml   # --history
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-history.yaml         # --history
kubectl apply -f https://raw.githubusercontent.com/idriss-eliguene/landlock-genprof/v0.5.0/deploy/rbac-restart.yaml        # --restart
```

Pinned to the `v0.5.0` tag rather than `master` on purpose — reproducible,
and immune to whatever's mid-change on the default branch. Swap the tag
for a newer one as releases come out.

### Option B — Helm chart from GHCR (OCI), no clone

```bash
helm install landlock-genprof oci://ghcr.io/idriss-eliguene/charts/landlock-genprof --version 0.5.0
```

Confirmed published as of `v0.5.0` — check
[github.com/idriss-eliguene?tab=packages](https://github.com/idriss-eliguene?tab=packages)
if a later tag has come out since and you want that version instead
(`v0.5.0` itself predates this pipeline being wired up).

### Option C — raw manifests from a local clone

```bash
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml
kubectl apply -f deploy/rbac-patched-manifest.yaml
kubectl apply -f deploy/crd-traininghistory.yaml   # --history
kubectl apply -f deploy/rbac-history.yaml          # --history
kubectl apply -f deploy/rbac-restart.yaml          # --restart
```

### Option D — Helm chart from a local clone

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

Options C/D install exactly the same things as A/B — only difference is
not needing network access to GitHub/GHCR at apply time, at the cost of
needing a clone. See
[`deploy/helm/landlock-genprof/README.md`](deploy/helm/landlock-genprof/README.md)
for the full `restart.enabled`/`history.enabled` toggle list and a
CRD-upgrade caveat worth knowing before your first `helm upgrade`.

## 4. First run

```bash
# Check host kernel and eBPF prerequisites first
kubectl landlock-genprof doctor

kubectl landlock-genprof trace \
  --pod <your-pod> -n <ns> --binary /path/to/main/binary \
  --duration 60s --out profile.yaml
```

Installed standalone instead of as a kubectl plugin? Drop the `kubectl `
prefix: `landlock-genprof trace ...`. Running from a source clone
without installing anywhere? `go run ./cmd/landlock-genprof trace ...`
works the same way.

`--pod` and `--binary` are the only required flags. See
[`docs/usage.md`](docs/usage.md) for what each `--*-out` flag adds.

## 5. Next steps

- [`docs/usage.md`](docs/usage.md) — training choices and the governed
  review, digest-bound approval, apply, and verify sequence.
- [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md) —
  what's needed to actually enforce a generated profile, and PodLock's
  own real limitation on `kind`-based clusters specifically.
- [`docs/architecture.md`](docs/architecture.md) — how the pieces fit
  together.
- [`docs/PROGRESS.md`](docs/PROGRESS.md) — what is demonstrated today.
- [`docs/PRODUCT_ROADMAP.md`](docs/PRODUCT_ROADMAP.md) — intended future work.

<!-- x-release-please-end -->

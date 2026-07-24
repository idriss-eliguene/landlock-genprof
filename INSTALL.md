# Installing landlock-genprof

This is for installing `landlock-genprof` against a Kubernetes cluster
you already have. Setting up a disposable dev/test cluster from scratch
instead (VM + `kind`)? See [`HOW_TO_START.md`](HOW_TO_START.md) and
`hack/init-vm.sh` — that path is aimed at contributors working on this
project itself, not at using the tool.

## 1. Prerequisites

- **Kernel version** on every node — see [`README.md`](README.md) §6 for
  the exact table (Landlock FS ≥ 5.13, Landlock network ≥ 6.4, eBPF ≥
  5.8 recommended). Check with `./hack/check-kernel.sh` if you have
  shell access to a node.
- **[Inspektor Gadget](https://www.inspektor-gadget.io/)** already
  deployed on the cluster (`kubectl gadget deploy`) — `trace` doesn't
  work without it. This is the one hard requirement; everything else
  below is about getting the CLI itself in place.
- `kubectl`, pointed at your cluster.
- `go 1.26+` — needed either way in step 2 below (no pre-built binaries
  published yet, so both options build from source).
- `helm` — only if using the Helm chart (option B in step 3, not step 2).

**Enforcement is separate from all of this.** Getting `landlock-genprof`
installed and running gets you profile *generation* — actually enforcing
what it generates (PodLock, a NetworkPolicy-capable CNI, SPO) is a
different set of prerequisites entirely, not all of which this project
can set up for you. See
[`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md)
before assuming the tool "isn't working" if a generated profile doesn't
seem to do anything once applied.

## 2. Get the CLI

No pre-built binaries are published yet (no GitHub Releases) — building
from source, one way or the other, is the only option today.

### Option A — install as a kubectl plugin (recommended)

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
make install-plugin   # builds kubectl-landlock-genprof, installs into $(go env GOPATH)/bin
kubectl plugin list   # confirms kubectl sees it
```

The rest of this doc uses `kubectl landlock-genprof ...` as the primary
form once installed this way.

One quirk: global `kubectl` flags placed *before* the plugin name
(`kubectl -n foo landlock-genprof ...`) are **not** forwarded to the
plugin — kubectl only passes through arguments that come *after* the
plugin name. Use `landlock-genprof`'s own `-n`/`--namespace` instead.

### Option B — build from source directly

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
go build ./...
```

Produces a `landlock-genprof` binary via `go build -o landlock-genprof
./cmd/landlock-genprof`, or run it directly without a separate build
step: `go run ./cmd/landlock-genprof ...`. An alternative to option A
above, not an extra step on top of it — pick one.

## 3. Install the RBAC and CRDs

The tracer needs its own `ServiceAccount`/RBAC, and every run publishes
a `SecurityProfileProposal` object, so its CRD (plus more RBAC) is
mandatory too. Two ways to install both:

### Option A — raw manifests

```bash
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml
# Required whenever a run composes securityContext data (commonly true
# in practice when syscalls are observed)
kubectl apply -f deploy/rbac-patched-manifest.yaml

# Only if you plan to use the matching flag:
kubectl apply -f deploy/crd-traininghistory.yaml   # --history
kubectl apply -f deploy/rbac-history.yaml          # --history
kubectl apply -f deploy/rbac-restart.yaml          # --restart
```

### Option B — Helm chart

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

Installs everything above in one release, with `restart.enabled`/
`history.enabled` values instead of separate opt-in manifests. See
[`deploy/helm/landlock-genprof/README.md`](deploy/helm/landlock-genprof/README.md)
for the full toggle list and a CRD-upgrade caveat worth knowing before
your first `helm upgrade`.

## 4. First run

```bash
kubectl landlock-genprof trace \
  --pod <your-pod> -n <ns> --binary /path/to/main/binary \
  --duration 60s --out profile.yaml
```

Built from source directly instead (step 2, option B)? Same command,
different invocation — swap `kubectl landlock-genprof` for
`go run ./cmd/landlock-genprof`:

```bash
go run ./cmd/landlock-genprof trace \
  --pod <your-pod> --namespace <ns> --binary /path/to/main/binary \
  --duration 60s --out profile.yaml
```

`--pod` and `--binary` are the only required flags. See
[`README.md`](README.md) §3 ("How it works") for what each `--*-out`
flag adds, and [`demo/script.md`](demo/script.md) for a worked example
end to end (`nginx-demo`).

## 5. Next steps

- [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md) —
  what's needed to actually enforce a generated profile, and PodLock's
  own real limitation on `kind`-based clusters specifically.
- [`docs/architecture.md`](docs/architecture.md) — how the pieces fit
  together.
- [`docs/roadmap.md`](docs/roadmap.md) — what's built, what isn't yet.

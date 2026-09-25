# Version-targeted contributor environment

The supported entry point for a disposable contributor environment is the
`dev-*` Make interface. It resolves an explicit Git tag or commit, materializes
that source into private user-owned state, and never changes the contributor's
checkout or silently selects `HEAD`.

## Quick start

Run the doctor first. It is read-only and checks tools, resources and the
active Docker runtime:

```bash
make dev-doctor
```

Select an immutable tag or commit for every environment operation:

```bash
VERSION=v0.10.0 make dev-up
VERSION=v0.10.0 make dev-status
VERSION=v0.10.0 make dev-test
VERSION=v0.10.0 make dev-down
```

`VERSION` must resolve locally to a tag or commit. Mutable selectors such as
`HEAD`, `master`, `main`, branch names and `origin/*` are rejected. Fetch the
desired tag or commit explicitly before using it. The selected source SHA is
recorded in the environment state.

For parallel environments targeting the same source revision, provide a
lowercase `INSTANCE` identifier containing 1-24 letters, digits or internal
hyphens:

```bash
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 INSTANCE=qualification-final make dev-up
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 INSTANCE=qualification-final make dev-status
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 INSTANCE=qualification-final make dev-down
```

The instance becomes part of the Kind cluster/context name and state path.
Omitting `INSTANCE` preserves the original source-only names and paths. Each
instance has an independent ownership record; an instance cannot clean up a
different instance's cluster or state.

`dev-up` is the only setup command. It creates an isolated Kind cluster with a
name derived from the selected source SHA and optional instance, an isolated
kubeconfig, and a private ownership record. It runs the selected source's existing bootstrap,
CRD, RBAC and Inspektor Gadget installation scripts. It does not install SPO
or PodLock unless explicitly requested.

`dev-e2e` is deliberately separate because it mutates the owned cluster:

```bash
VERSION=v0.10.0 make dev-e2e
```

Do not run it as part of ordinary development or CI setup.

## macOS with Lima

Prerequisites:

- macOS amd64 or arm64.
- Bash 4+, Git, Go matching the selected source, kubectl, kind and Helm.
- Lima and Docker context `lima-landlock-genprof-core`.
- At least 4 CPUs, 6 GiB memory and 20 GiB free disk.

The existing `hack/bootstrap.sh` Lima path is reused. `dev-up` may start the
existing `landlock-genprof-core` Lima VM, but it creates only the uniquely
named development Kind cluster inside that Docker context. It refuses to
switch Docker contexts and never adopts a same-named cluster without the
matching ownership record.

Source-only tests run on macOS:

```bash
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 make dev-test
```

The Linux tracer and live Golden E2E require a Linux execution environment.
Lima provisioning is supported, but `make dev-e2e` must currently be run from
native Linux or through an explicitly prepared Linux guest executor. The
macOS host plugin is not treated as a substitute for the Linux tracer.

## Native Linux

Prerequisites:

- Linux amd64 or arm64 with Docker or a compatible rootful Docker context.
- Go matching the selected source's `toolchain` directive.
- kubectl, kind, Helm and Bash 4+.
- A kernel suitable for the selected live tracing workflow; component
  readiness alone does not establish Landlock, eBPF or seccomp enforcement.

The active Docker context and endpoint are recorded. The workflow does not
require Lima or Docker-in-Docker.

## Optional backends

SPO and PodLock are explicit opt-in dependencies. Their versions and the
required cert-manager version must be supplied; no `latest` fallback is used.

```bash
DEV_INSTALL_SPO=1 \
DEV_SPO_VERSION=v1.0.0 \
DEV_CERT_MANAGER_VERSION=v1.17.2 \
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 \
make dev-up
```

```bash
DEV_INSTALL_PODLOCK=1 \
DEV_PODLOCK_VERSION=0.1.1 \
DEV_CERT_MANAGER_VERSION=v1.18.2 \
VERSION=f25d5480a64f9c284db0bc26d0c7551e3cab3dc6 \
make dev-up
```

These backends have different runtime requirements. PodLock's NRI and
Landlock behavior requires a suitable real Linux node; a Kind component being
Ready is not behavioral enforcement evidence.

## State, safety and cleanup

State is stored below:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/landlock-genprof/dev/<source-sha>/
```

When `INSTANCE` is set, state is stored below
`${XDG_STATE_HOME:-$HOME/.local/state}/landlock-genprof/dev/<source-sha>/<instance>/`.
The corresponding cluster is
`landlock-genprof-dev-<source-sha-prefix>-<instance>`.

This includes the source snapshot, kubeconfig, Go workspace, dependency pins,
Docker endpoint and ownership metadata. Keep the kubeconfig private.

Inspect the environment without mutation:

```bash
VERSION=<tag-or-commit> make dev-status
```

Cleanup is ownership-gated and deletes only the Kind cluster created for that
source SHA and its private state:

```bash
VERSION=<tag-or-commit> make dev-down
```

It refuses to delete a same-named cluster without a matching owner and source
SHA. It does not stop or delete other Kind clusters, Lima VMs, Docker
contexts, host tools or unrelated namespaces.

## Version and evidence contract

The selected source's `go.mod` and `hack/versions.env` are authoritative for
the effective Go toolchain, kubectl, kind, node image, Helm, Cilium and
Inspektor Gadget pins. The node image must include a digest. The effective
source SHA, instance, Docker context, Docker daemon identity, image pin and
toolchain are recorded after setup.

The environment qualifies CLI/API behavior and the requested governance
workflow. It does not by itself prove that a generated profile was applied or
that the Linux kernel enforced it. Record those claims separately as
GENERATED, APPLIED, BEHAVIORALLY_VERIFIED, UNKNOWN or NOT_ESTABLISHED.

## Troubleshooting

- `VERSION is required`: pass `VERSION=<tag-or-commit>` to every stateful
  command.
- `unknown VERSION`: fetch the tag or commit explicitly; the helper does not
  fetch or guess a moving ref.
- Docker context refusal on macOS: select
  `lima-landlock-genprof-core` deliberately, then rerun `make dev-doctor`.
- Resource failure: increase Lima resources or free host disk; the helper
  will not lower safety thresholds.
- Ownership refusal: inspect `make dev-status VERSION=...`; never delete or
  adopt an unowned cluster manually as part of this workflow. Use a distinct
  `INSTANCE` when the same source revision already has a protected environment.
- Go mismatch: install/select the exact toolchain in the selected `go.mod`.
- Live tracing failure on macOS: use a prepared Linux guest executor or native
  Linux; do not use the Darwin tracer stub as evidence.

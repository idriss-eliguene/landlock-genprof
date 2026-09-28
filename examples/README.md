# Examples

| File | Where it comes from |
|---|---|
| `nginx-generated-profile.yaml` | Real M4 capture, analysed rule by rule in [`docs/e2e-demo.md`](../docs/e2e-demo.md) |
| `nginx-hand-written-reference-profile.yaml` | Hand-written baseline for that analysis, not a trace |
| `nginx-generated-networkpolicy.yaml` | ARM64 capture below |
| `nginx-generated-seccomp.json` | ARM64 capture below |
| `nginx-generated-seccompprofile.yaml` | ARM64 capture below |
| `nginx-generated-capabilities.yaml` | ARM64 capture below |
| `nginx-generated-securitycontext.yaml` | ARM64 capture below |
| `nginx-generated-report.md` | ARM64 capture below |
| `nginx-generated-proposal.yaml` | ARM64 capture below |

## ARM64 capture (issue #94)

> **Warning: the seccomp architecture list is wrong for AArch64.**
> Inspektor Gadget v0.55.1's `advise_seccomp` hardcodes
> `SCMP_ARCH_X86_64`, `SCMP_ARCH_X86` and `SCMP_ARCH_X32` whatever the node
> architecture is
> ([source](https://github.com/inspektor-gadget/inspektor-gadget/blob/v0.55.1/gadgets/advise_seccomp/go/program.go#L152-L156)),
> and landlock-genprof keeps the list the gadget reports. The raw evidence of
> this run (`--events-out`) already carries those three values. The syscall
> names themselves come from the node's own syscall table. Do not use
> `nginx-generated-seccomp.json`, `nginx-generated-seccompprofile.yaml`, or the
> seccomp parts of `nginx-generated-securitycontext.yaml` and
> `nginx-generated-proposal.yaml` for ARM64 enforcement. The capture should be
> repeated once Inspektor Gadget reports the real architecture.

All seven files come from one `trace` run. Each one is the unmodified
generated output (`nginx-demo-<artifact>` renamed to
`nginx-generated-<artifact>`), with only a comment header added on top.
`nginx-generated-seccomp.json` is byte-identical to what the run wrote, since
JSON can't carry a comment; this page is its header. The PodLock profile of
this run is in `nginx-generated-proposal.yaml` under `spec.podLock`.

### Environment

- Host: Apple Silicon Mac, Lima VM `landlock-genprof-core` created by
  `hack/bootstrap.sh` (`template://docker-rootful`, `vz` driver), then
  `make test-env`.
- Guest: Ubuntu 26.04 LTS, kernel `7.0.0-28-generic`, `aarch64`.
- Cluster: kind v0.33.0, node image
  `kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`,
  containerd 2.3.4, Cilium 1.20.1.
- Inspektor Gadget v0.55.1 (Helm chart from `make test-env`).
- landlock-genprof `v0.10.0-18-g3e9e199` (commit `3e9e199`), built with
  `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and run inside the Lima guest: the
  macOS build only has the tracer stub.
- Run window: 2026-09-28, 08:52:09Z to 08:53:10Z.
- Local-only DNS workaround: the gadget DaemonSet got `dnsConfig` `ndots: 1`
  because the capture network's DNS answers every name under its search
  domain, which broke the gadget image pulls from `ghcr.io`. It changes how
  the gadget pod resolves registry names, not what gets traced.

### Workload

- Bare pod `nginx-demo`, container `nginx-demo`, created the same way
  `hack/init-vm.sh` used to (`kubectl run`), with the image pinned by digest:
  `nginx:alpine@sha256:df221db836e1754089190208cee7eeda94f233197056426eda74a43ab1abeac2`
  (image index; the node ran the `linux/arm64` manifest
  `sha256:84dd96a0337ae0f15be81afa8c6bd99d06a514a2ab71a1503aba91ce4392ecc2`),
  nginx 1.31.6.
- Traffic came from a separate pod, `nginx-demo-client`, through a Service.
  Nothing was run with `kubectl exec` inside the target during the run: the
  seccomp advisor captures syscalls at container level, so anything executed
  in the `nginx-demo` container would end up in the profile.

### Commands

```bash
IMG=nginx:alpine@sha256:df221db836e1754089190208cee7eeda94f233197056426eda74a43ab1abeac2

kubectl run nginx-demo --image=$IMG --port=80
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=180s
kubectl expose pod nginx-demo --port=80
kubectl run nginx-demo-client --image=$IMG --command -- sh -c \
  'while true; do wget -q -O /dev/null http://nginx-demo/; wget -q -O /dev/null http://nginx-demo/does-not-exist; sleep 1; done'

# inside the Lima guest, with a kubeconfig for the kind cluster
landlock-genprof trace --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s --restart \
  --network-out --seccomp-out --seccomp-profile-out --capabilities-out \
  --security-context-out --report-out --patched-manifest-out --events-out

kubectl get securityprofileproposal nginx-demo -n default -o yaml
```

`--restart` is there so startup-only activity (`bind`, privilege drop,
capability checks) gets captured, see `docs/e2e-demo.md` Finding 5.

Before this run, a 20s warm-up trace without `--restart` was run only to pull
the gadget images (a cold pull can leave the seccomp output empty, see
`docs/e2e-demo.md` Finding 4). Its outputs were discarded and its
`SecurityProfileProposal` deleted before the real run.

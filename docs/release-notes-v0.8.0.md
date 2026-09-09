# v0.8.0

## Highlights

v0.8.0 adds the Governance Operations Workbench and the production boundary
required to operate it alongside the Observation executor.

## Operations Center

- Single-replica Operations Center Deployment with ClusterIP Service.
- Six authenticated surfaces: Overview, Workloads, Observations, Proposals,
  History, and Attention.
- Authoritative Platform and Projection health presentation.
- Malformed-object containment with bounded diagnostics and `NOT_ELIGIBLE`.
- Lifecycle endpoints, startup/liveness/readiness probes, and bounded graceful
  shutdown.

## Observation Executor

- Dedicated executor Deployment and ServiceAccount.
- Durable resourceVersion claim/lease ownership.
- Expired-claim recovery to truthful `FAILED/EXECUTOR_LOST` state.
- Executor authority remains separate from human governance authority.

## Security & Trust Boundary

- Production authentication requires a trusted proxy and signed HMAC identity.
- The five-minute timestamp freshness window is not a replay cache or nonce.
- The same valid assertion may be replayed within that window.
- The browser must not directly reach the backend in production.
- Operations Center and executor NetworkPolicies are component-scoped.
- Executor governance mutations and direct Operations Center Gadget execution
  are denied.

## Governance

Review, Approve, Reject, Apply, and Rollback remain named, actor-attributed,
resourceVersion-guarded operations. Stale decisions return a conflict rather
than being silently replayed.

## Reliability & Recovery

- Operations Center and executor use one replica and `Recreate`.
- HMAC rotation requires a controlled restart.
- Helm rollback is supported only across CRD-compatible application revisions.
- Uninstall preserves CRD-backed product history.
- Reinstall recovers compatible durable state.

## Observability

Structured logs, request correlation, bounded metrics, authentication and
authorization events, executor ownership correlation, and executor-loss
signals are available. Secrets, signatures, tokens, kubeconfigs, and raw
credentials are not logged or exposed as metric labels.

## Helm / Installation / Upgrade

The chart requires pre-created trusted-proxy and executor-kubeconfig Secrets
when the Operations Center is enabled. Trusted-proxy selectors, target
namespaces, and Kubernetes API CIDRs must be explicit. Foreign cluster-scoped
resources are never silently adopted; compatible platform-managed resources
must be selected through the documented ownership switches.

CRDs are `v1alpha1`, schema migration is manual and reviewed, and Helm
rollback is bounded by schema compatibility. The documented uninstall/reinstall
procedure preserves durable product state.

## Lima Development & UI Testing

The canonical macOS path uses the `landlock-genprof-core` Lima VM, Docker
context `lima-landlock-genprof-core`, and Kubernetes context
`kind-landlock-genprof-core`:

```bash
./hack/bootstrap.sh
make env-doctor
make test-env
make ui-lima
```

`make ui-lima` is a local-development, loopback-only read-only UI launcher and
prints the browser URL. It does not reproduce production authentication. The
production-like UI procedure uses a trusted-proxy fixture and verifies
unsigned `401`, valid signed `200`, and stale signed `401` behavior.

## Platform Qualification

Empirically qualified: macOS ARM64, Lima ARM64, Docker ARM64, Kubernetes
v1.36.4, Cilium v1.20.1, Helm v3.21.4, Go v1.26.5, and linux/arm64 runtime
image. Linux/amd64 is build-supported but not runtime-qualified.

## Known Limitations

- One Operations Center replica and one executor replica.
- Restart and HMAC rotation downtime is expected.
- No replay cache or nonce.
- CRDs are `v1alpha1`; migrations are manual.
- No multi-cluster control plane or HA executor certification.
- Strict network qualification is Cilium-based.
- External monitoring integration and SLOs are not fully defined.
- `G8-Q1-R4-001`: non-blocking global History error-banner presentation debt.
- No published SBOM, provenance attestation, or image signature.

## Upgrade Notes

Before installation, create the required Secrets without placing values in
Git, ConfigMaps, command arguments, or browser responses. Configure trusted
proxy selectors, executor target namespaces, and API CIDRs. Use the documented
external ownership switches for pre-existing cluster resources. Rotate HMAC
material through the restart-based procedure. Review CRD schema changes
manually before upgrading. Do not use Helm release rollback as a substitute
for product ApplyAttempt rollback.

## Security Notes

The executor cannot Review, Approve, Reject, Apply, or Rollback governance
objects. The Operations Center cannot directly execute Gadget in authenticated
mode. The trusted proxy must strip client-supplied identity headers before
signing. Metrics access is selector-bound and is not an authentication bypass.

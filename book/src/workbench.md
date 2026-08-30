# Review proposals with the Workbench

The Workbench is the v0.4.0 experimental local review surface for one
existing `SecurityProfileProposal`. It is implemented inside the
`landlock-genprof` CLI binary. It is local-first, loopback-only, read-only,
and snapshot-based; it is not a cluster dashboard or a production-ready
shared service.

## Install and launch

Install the CLI and prepare cluster-side prerequisites using the
[installation guide](INSTALL.md). The Workbench uses the same kubeconfig,
Kubernetes identity, proposal CRD, and read permission as the CLI. It does
not install a second UI package or create a service account.

With an existing proposal:

```bash
kubectl landlock-genprof ui <proposal> --namespace <namespace>
```

The default URL is `http://127.0.0.1:8080`. Use `--port <port>` for another
local port; open the URL printed by the command and stop it with `Ctrl-C`.
Missing proposals and unreachable clusters are startup errors. Restart the
command to refresh the page after cluster state changes.

## What the page shows

The single review page presents:

- proposal and workload/container/binary identity;
- lifecycle state and the exact canonical candidate digest;
- candidate artifact domains when they exist, without inventing a current
  configuration or a current-to-proposed delta;
- provenance, distinguishing direct evidence from derived policy and showing
  SPO-sourced SeccompProfile content as derived policy;
- approval state, approved digest, and the binding result when canonical
  validation can establish it;
- application and behavioral-verification state only when available in the
  proposal, otherwise `NOT AVAILABLE` or `NOT VERIFIED`;
- unsupported, uncertified, and unknown boundaries.

Coverage is informational metadata. It is not confidence, syscall frequency,
authorization, or proof that an unobserved permission is unnecessary.

The browser cannot approve, reject, or apply a candidate. Use the CLI for
those operations and review the exact digest:

```bash
kubectl landlock-genprof approve <proposal> \
  --namespace <namespace> --expected-digest sha256:<digest>
kubectl landlock-genprof apply-proposal <proposal> --namespace <namespace>
```

Approval is not application; application is not enforcement; enforcement is
not behavioral verification. Backend-specific limits remain documented in
the [support matrix](https://github.com/idriss-eliguene/landlock-genprof/blob/master/docs/support-matrix.md), [architecture](docs/architecture.md),
and [threat model](docs/threat-model.md).

## Trust and snapshot model

Startup performs the only Kubernetes read and builds a typed view before
serving HTML:

```text
Kubernetes API / kubeconfig
        ↓
SecurityProfileProposal and canonical domain functions
        ↓
workbenchView snapshot
        ↓
html/template → 127.0.0.1:<port> → browser
```

The request handler does not hold a Kubernetes client. Browser interaction
therefore cannot trigger a Kubernetes read or mutation in this architecture.
The listener has no persistence, sessions, authentication layer, or remote
access contract. Do not expose it through an ingress or use it as a shared
multi-user service. See the [architecture overview](docs/architecture.md),
[threat model](docs/threat-model.md), and the [detailed Workbench experiment
record](https://github.com/idriss-eliguene/landlock-genprof/blob/master/docs/workbench-experiment.md).

# Full Visual Workbench

The Workbench is the local, loopback-only, read-only engineering and security
inspection surface for a namespace. It starts with workload discovery and can
optionally open one existing `SecurityProfileProposal` as a direct-entry
shortcut. It is not a generic dashboard or a browser mutation interface.

## Install and launch

Install the CLI and prepare cluster-side prerequisites using the
[installation guide](INSTALL.md). The Workbench uses the same kubeconfig,
Kubernetes identity, proposal CRD, and read permission as the CLI. It does
not install a second UI package or create a service account.

Without a proposal, open the workload-first Explorer:

```bash
kubectl landlock-genprof ui --namespace <namespace>
```

With an existing proposal, use the direct-entry shortcut:

```bash
kubectl landlock-genprof ui <proposal> --namespace <namespace>
```

The default URL is `http://127.0.0.1:8080`. Use `--port <port>` for another
local port; open the URL printed by the command and stop it with `Ctrl-C`.
Missing proposals and unreachable clusters are startup errors for the
proposal shortcut. Reload the page to perform another bounded read.

## What the page shows

The Workbench presents:

- namespace-scoped workload and container discovery, followed by canonical
  workload/container identity;
- declared, materialized, binding, runtime, derived-policy, governance, and
  enforcement/behavioral-verification sections, each with its own state;
- ApplyAttempt and RollbackAttempt custody, including partial and unknown
  outcomes when the read capability permits those resources;
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

The browser cannot approve, reject, apply, rollback, or activate custody. Use the CLI for
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

## Trust and bounded-read model

Each page request performs namespace-scoped reads through the bounded
`WorkbenchReadCapability`; there is no Watch, polling loop, or cluster-wide
fanout. Attempt visibility is an optional, unbound read-only RBAC capability
(`deploy/rbac-workbench.yaml`) and does not grant mutation authority.

The request path is:

```text
Kubernetes API / kubeconfig
        ↓
SecurityProfileProposal and canonical domain functions
        ↓
bounded Workbench projection
        ↓
html/template → 127.0.0.1:<port> → browser
```

The HTTP handler holds only the bounded read capability. Browser interaction
cannot approve, apply, rollback, or trigger a Kubernetes mutation. Copyable
next actions are advisory CLI text only.
The listener has no persistence, sessions, authentication layer, or remote
access contract. Do not expose it through an ingress or use it as a shared
multi-user service. See the [architecture overview](docs/architecture.md),
[threat model](docs/threat-model.md), and the [detailed Workbench experiment
record](https://github.com/idriss-eliguene/landlock-genprof/blob/master/docs/workbench-experiment.md).

# Importing SPO-derived seccomp policy (`--seccomp-source=spo`)

By default this project observes syscalls itself and synthesizes the
seccomp profile from what it saw. security-profiles-operator (SPO) does the
same job with a better instrument — a production eBPF recorder or an
audit-log enricher, merged across replicas — so `trace` can instead **import
SPO's generated profile** and govern it.

What SPO does *not* record is filesystem access as Landlock authority, or
network activity at all. Those stay ours in both modes. SPO replaces one
domain of observation, never all of it.

The normative contract is [ADR-0008](../adr/0008-spo-derived-policy-import-boundary.md).

## Two modes, always explicit

| | `--seccomp-source=internal` (default) | `--seccomp-source=spo` |
|---|---|---|
| Syscalls observed by | landlock-genprof | SPO |
| Syscalls in `TrainingHistory` | yes | **no — not collected at all** |
| Seccomp confidence tier | yes | **not applicable** |
| `--seccomp-out` (plain JSON) | available | rejected |
| Filesystem + network | ours | ours |

The source is **never auto-detected**. If it were inferred from whether SPO
happens to be installed, the same command would govern different authority
on different clusters, and `explain`'s syscall section would mean something
different depending on invisible cluster state.

There is also **no fallback in either direction**. If you select `spo` and
the material is missing, the command fails — it does not quietly fall back
to internally-synthesized syscalls, because that would change what is being
governed without saying so.

## Recording with SPO

The recording must produce a **complete** profile that is **inert** until a
human approves it:

```yaml
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: ProfileRecording
metadata:
  name: nginx-rec
  namespace: prod
spec:
  kind: SeccompProfile
  recorder: Bpf
  # Required by the import. It makes SPO leave the generated profile in
  # spec.state: Disabled, so the recorded authority is never enforced on any
  # node before it has been reviewed and approved.
  disableProfileAfterRecording: true
  podSelector:
    matchLabels:
      app: nginx
```

Note `ProfileRecording` is **namespaced** and must live in the workload's
namespace, while the `SeccompProfile` it generates is **cluster-scoped**.
That asymmetry is why lineage is carried by labels rather than by comparing
namespaces.

## Importing

Both names are given explicitly. Nothing is discovered by searching the
cluster for a profile that looks plausible — that is precisely how one
workload's authority ends up governing another.

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo --namespace prod --container tools \
  --binary /usr/sbin/nginx --duration 60s \
  --seccomp-source=spo \
  --spo-recording nginx-rec \
  --spo-profile nginx-rec-tools
```

## What the import checks

Every gate below fails closed, with a message naming what failed.

| Gate | Refused when |
|---|---|
| API shape | not `security-profiles-operator.x-k8s.io/v1`, not `SeccompProfile`, or namespaced (SPO ≤ v0.8.4) |
| Inertness | the recording lacks `disableProfileAfterRecording: true`, or the profile is not `spec.state: Disabled` |
| Completeness | the profile carries SPO's `partial` label, or the recording still has unmerged profiles outstanding |
| Lineage | `recording-namespace`, `recording-id` or `container-id` is absent or disagrees with the target |
| Enforcement content | any field outside `defaultAction`, `architectures`, `syscalls[].names`, `syscalls[].action` |
| Content sanity | no `defaultAction`, or a syscall rule with no action |

### Why unsupported fields are refused rather than dropped

Dropping changes what is enforced relative to what was reviewed — in both
directions. `baseProfileName` **narrows**: SPO unions the named base
profile's syscalls in, so a profile with a base plus three syscalls may
really permit hundreds. `syscalls[].args` **widens**: a rule permitted only
for specific argument values becomes unconditional.

The allow-list is closed, so a field SPO adds in a future release is
refused too, rather than silently lost.

## What you get

The import is a **snapshot**. Content is copied into a new
landlock-genprof-owned `SeccompProfile`; the SPO source object is read once
and never modified, adopted, renamed, or referenced afterwards. Mutating or
deleting the source later cannot change an approved candidate.

The workload is bound to the **governed** copy —
`operator/lg-v1-<pod>-<hash>.json` — never to the SPO source profile.

Provenance rides inside the governed artifact as annotations, so it is
covered by `CandidateDigest`. `review` shows it:

```
Seccomp:
  Source: security-profiles-operator
  Origin: derived policy (not observed by landlock-genprof)
  Source profile: nginx-rec-tools
  Recording: prod/nginx-rec
  Container: tools
  Coverage: unknown
  Confidence: not applicable (derived policy carries no occurrence data)
```

**Confidence is not applicable, and that is stated rather than left blank.**
SPO's generated profile carries syscall *names only* — no timestamps, no
occurrence counts. Any tier would be invented, and a blank where filesystem
rules show `high` would read as `low`.

**Coverage is `unknown` in practice.** ADR-0008 expects SPO to report
`spo.x-k8s.io/syscall-coverage`; SPO v1.0.0 does not set it. Absent coverage
is recorded as the explicit token `unknown` — never `0`, never `full`, never
a confidence tier — and does not block the import.

## Known limitation: merged profiles

SPO's recording merger (`mergeStrategy: Containers`, used for replicated
workloads) sets only `recording-id` and `recording-namespace` on the merged
profile — it does **not** carry `container-id` through. The lineage contract
requires all three, so **merged profiles are currently refused**.

Single-replica recordings (`mergeStrategy: None`, the default) carry all
three labels and import normally.

This is a fail-closed refusal, not a security gap: the alternative would be
importing a profile whose container cannot be confirmed. Supporting merged
profiles requires either an upstream change or a further decision about what
lineage evidence is sufficient without `container-id`.

## Switching sources invalidates approval

Provenance is digested, so changing the source changes `CandidateDigest`
and any prior approval goes stale — apply then fails closed. That is
intended:

- internal → SPO, or SPO → internal, with byte-identical syscalls: the
  digest still changes, because *which system's authority is enforced* is
  part of what the reviewer signed off on;
- re-importing from a different recording: the digest changes, because a new
  recording is genuinely new evidence.

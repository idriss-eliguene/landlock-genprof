# Example Hardening Review — nginx-demo

> This is a format example populated from the repository's existing
> illustrative nginx artifacts (`examples/nginx-generated-proposal.yaml` and
> `examples/nginx-generated-report.md`). It is not a fresh customer capture,
> does not combine unrelated certification results, and must not be presented
> as live verification.

## Executive summary

- **Target:** `nginx-demo` / `default` / `nginx`
- **Assessment:** Example artifact review only
- **Candidate:** Example proposal exists; exact digest must be recomputed by
  `review` for a current run
- **Approval:** Not established in this example
- **Application:** Not established in this example
- **Verification:** Not established for this example workload
- **Decision:** Review candidate with the workload owner; do not infer
  legitimacy or necessity from the example observation window.

## Current configuration

The illustrative proposal contains a patched manifest and generated resources,
but it does not provide a trustworthy pre-change live configuration capture.

| Domain | Current state | Status |
|---|---|---|
| Filesystem / Landlock | Pre-change state not included | UNKNOWN |
| Network | Pre-change NetworkPolicy state not included | UNKNOWN |
| Seccomp | Pre-change workload state not included | UNKNOWN |
| Capabilities | Pre-change securityContext not included | UNKNOWN |

## Available evidence and derived policy

The existing illustrative report shows the following candidate inputs:

| Domain | Available material | Interpretation |
|---|---|---|
| Filesystem | `/etc/nginx`, read | Observed in the illustrative report's stated scope |
| Network | Port 80, ingress | Candidate/report material; live enforcement is not established here |
| Syscalls | `openat`, low confidence in the illustrative report | Observation summary; not a claim of completeness |
| Capabilities | `CAP_SETUID` in the report fixture | Candidate/report material; behavioral effect is not established here |
| SPO | Example proposal field | Derived policy if sourced from SPO, never direct landlock-genprof evidence |

The example does not establish that any unobserved authority is unnecessary.

## Proposal identity and lifecycle

| Field | Example value |
|---|---|
| Proposal | `default/nginx-demo` |
| Candidate digest | Recompute with `kubectl landlock-genprof review nginx-demo --namespace default` |
| Review state | Not established by the static example |
| Approval state | Not established |
| Approved digest | Not established |
| Application state | Not established |
| Verification state | Not established |

The exact candidate digest must be reviewed and explicitly approved before
governed application. The example files do not authorize application.

## Proposed change and review action

The generated artifacts represent a candidate across filesystem, network,
seccomp, and capability/securityContext domains. The meaningful customer
action is to review each domain against workload context, collect missing
current-state evidence, and decide whether staging verification is warranted.

No statement in this example establishes a safe removal, universal
compatibility, enforcement, or behavioral verification result.

## Limitations and handoff

- This is not a live capture.
- Current configuration is unavailable.
- Candidate digest and approval are not established.
- Application and partial-application state are unavailable.
- Backend enforcement and behavioral verification are unavailable.
- The repository's actual support and certification boundaries remain
  authoritative.

For a real pilot, attach the environment capture, raw evidence inventory,
proposal status, exact digest, generated artifacts, application result,
backend verification result, and recovery record described in the template.

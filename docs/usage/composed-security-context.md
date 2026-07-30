# Step 10 — Optional composed securityContext (`--security-context-out`)

Pass `--security-context-out` to also generate a composed
`securityContext` fragment combining the same capabilities data from
Step 9 with a *reference* to the seccomp profile — generated
whenever syscalls were observed, independent of whether `--seccomp-out`/
`--seccomp-profile-out` (Step 8/14) were also passed
this run:

```yaml
capabilities:
  add:
    - NET_BIND_SERVICE   # confidence: high
  drop:
    - ALL
seccompProfile:
  type: Localhost
  localhostProfile: operator/default/nginx-demo.json
```

This is **not** a merge of the seccomp and capabilities exporters —
`seccomp.json`/`capabilities.yaml` are still generated exactly as
before, independently. A seccomp profile has to ship as its own file for
the kubelet to load (`localhostProfile` only ever takes a path
reference, never inline content), so merging the files themselves
wouldn't actually reduce anything — it'd just add indirection. This flag
adds a third, composed *view* on top, for the common case of wanting
both in one place to paste under a container's `securityContext:` key.
`localhostProfile` always follows security-profiles-operator (SPO)'s own
`operator/<namespace>/<pod>.json` naming convention — confirmed live
against a real reconciliation, see Step 14 for why, and for the
flag that actually generates the object at that path.

**Deliberately does not infer** `privileged`, `allowPrivilegeEscalation`,
`runAsNonRoot`, `readOnlyRootFilesystem`, or `runAsUser` — nothing in
this project observes any of them today, and guessing "safe defaults"
regardless of what was actually seen would contradict the project's own
positioning: observe, don't guess.

# Step 6 — Optional target restart (`--restart`)

Resources a process opens once at startup (a pid file, a log fd) and
keeps writing to are invisible to a trace attached to an already-running
container — `trace_open` only observes `openat()`, not later `write()`s
on an already-open fd. Pass `--restart` to have the CLI restart the
target right before observing it, attaching the tracer *first* in every
case so the restart's startup activity is actually captured — delete
+recreate for a bare pod, or the same rollout-restart mechanism `kubectl
rollout restart` uses for a Deployment/StatefulSet/DaemonSet-owned one.

The tracer is pre-targeted differently depending on whether the owner
keeps a stable pod name: a bare pod or StatefulSet keeps its name across
the restart, so the tracer is pre-attached by that name directly. A
Deployment/DaemonSet's replacement gets an unpredictable new name, so
the tracer is instead pre-attached by the **workload's own label
selector** — which also means the generated profile is identified by the
*workload's* name (e.g. `nginx-ds`), not one ephemeral pod, and the
PodLock guidance patches the pod template (`kubectl patch deployment`/
`daemonset`) instead of labeling a single pod that a future rollout would
replace anyway.

Opt-in: it's disruptive to the running workload, and needs additional
RBAC beyond the base manifest — apply
[`../../deploy/rbac-restart.yaml`](../../deploy/rbac-restart.yaml) first.

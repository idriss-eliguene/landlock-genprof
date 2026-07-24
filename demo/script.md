# Demo script — ~75s technical walkthrough

Target audience: someone landing on the repo from the announcement
([Discussion #95](https://github.com/idriss-eliguene/landlock-genprof/discussions/95))
who wants to see the tool actually do something in under two minutes,
before reading a line of docs.

**This is a runbook, not a transcript.** Every command below is real and
matches the current CLI (`cmd/landlock-genprof/trace.go`) — but the exact
output (paths, timings, confidence levels) depends on what your VM/cluster
actually observes. Run it for real and paste the real output before
recording; don't reuse the numbers below as if they were captured output.

## Prerequisites (not part of the recording)

- `kind` cluster + Inspektor Gadget deployed, `nginx-demo` pod running —
  see [`HOW_TO_START.md`](../HOW_TO_START.md).
- CRDs/RBAC applied once: `deploy/rbac.yaml`,
  `deploy/crd-securityprofileproposal.yaml`, `deploy/rbac-proposal.yaml`,
  `deploy/rbac-patched-manifest.yaml`, `deploy/rbac-restart.yaml` — or the
  Helm chart equivalent (`deploy/helm/landlock-genprof`).
- A second terminal ready to generate traffic against the pod during the
  trace window.

## A timing decision to make before recording

A real, meaningful training run is documented at 60s throughout this repo
(`docs/e2e-demo.md`, `docs/roadmap.md`). A 60-90s demo video can't fit a
literal 60s trace *and* everything else. Two honest options — pick one,
don't silently cut the duration and call it the same thing:

1. **Real 60s trace, sped up in editing.** Cut the waiting to ~5s of
   video with a visible "60s real-time, sped up" caption. Most faithful
   to what the tool actually needs for good coverage.
2. **Shorter `--duration` for the recording specifically** (e.g. `20s`),
   combined with `--restart` so the pod's startup-time activity (which
   is otherwise invisible to a trace attached late — see
   `docs/e2e-demo.md` Finding 2) is captured immediately instead of
   waiting for organic traffic. Real flag, real behavior, just a
   shorter window than the docs' own reference run — say so on screen.

This script assumes option 2 below; swap `--duration 20s --restart` for
plain `--duration 60s` (with traffic generated in a second terminal, per
`docs/e2e-demo.md`'s Setup section) if you go with option 1 instead.

---

## Shot list

### [0:00-0:08] The "before"

```bash
kubectl get pod nginx-demo -o jsonpath='{.spec.containers[0].securityContext}'
```

Narration: *"This pod has whatever default permissions containerd gives
it — nothing scoped to what it actually does."* (Expect this to print
nothing or `{}` — that's the point.)

### [0:08-0:14] Run the training run

```bash
go run ./cmd/landlock-genprof trace \
  --pod nginx-demo --namespace default --binary /usr/sbin/nginx \
  --duration 20s --restart \
  --network-out --seccomp-profile-out --patched-manifest-out
```

Narration while it runs: *"It observes the pod's real filesystem,
network, and syscall activity via eBPF — no static config, no guessing."*
`--restart` recreates the pod right before attaching, so the container's
startup-time file opens are captured instead of missed.

> CAPTURE REAL OUTPUT HERE — stdout from this command, including the
> "not-yet-confirmed syscalls" note if one is printed.

### [0:14-0:20] Show the generated profile

```bash
cat nginx-demo-profile.yaml
```

> CAPTURE REAL OUTPUT HERE — real generated YAML, with its real
> `# confidence: ...` comments. (`examples/nginx-generated-profile.yaml`
> in this repo is a real capture too, but from an earlier milestone
> before the contamination fix and confidence annotations — don't reuse
> it as if it were fresh output; see
> [issue #94](https://github.com/idriss-eliguene/landlock-genprof/issues/94)
> for regenerating it.)

Narration: *"Every rule traces back to something actually observed — and
is annotated with how confident the tool is, based on how it was seen."*

### [0:20-0:30] The punchy summary

```bash
go run ./cmd/landlock-genprof review nginx-demo
```

> CAPTURE REAL OUTPUT HERE — the real `WORKLOAD SECURITY REVIEW` block
> (`cmd/landlock-genprof/review.go`): proposal name, container, binary,
> generated-at, history-used, and an availability line per artifact
> (PodLock / NetworkPolicy / Patched Manifest / SPO SeccompProfile).

Narration: *"Every run also publishes this as a `SecurityProfileProposal`
cluster object — reviewable with `kubectl` or GitOps, not just local
files."*

### [0:30-0:45] Apply — and the honest boundary

```bash
kubectl apply -f nginx-demo-profile.yaml
kubectl apply -f nginx-demo-networkpolicy.yaml
kubectl apply -f nginx-demo-patched.yaml   # or the Deployment/DaemonSet's own name if owned
```

Narration: *"This is where this tool's job ends. Applying the PodLock
profile and the NetworkPolicy is where PodLock's own operator and your
CNI take over enforcement — this tool generates what they should
enforce, not the enforcement itself."*

**Do not stage a "blocked access attempt" here.** Neither PodLock nor
security-profiles-operator is installed by this repo's own setup
(`hack/init-vm.sh` deploys Cilium and Inspektor Gadget, not either of
these) — see
[`docs/enforcement-prerequisites.md`](../docs/enforcement-prerequisites.md)
for both. SPO is a real, if opt-in, gap you could close for the
recording if you want that beat. **PodLock is not just undeployed —
its own docs advise against this project's entire `kind`-based
environment**, so don't attempt to demo live PodLock enforcement on
this setup at all. If you want that beat in the video, it needs a
different reference environment (Lima, per PodLock's own quickstart),
not this one — a bigger undertaking than this script assumes.

### [0:45-1:00] Close

```bash
go run ./cmd/landlock-genprof trace --help
```

Narration: *"Prototype stage, v0.1.0, feedback wanted — repo link on
screen."* Point at the good-first-issue labels and
[Discussion #95](https://github.com/idriss-eliguene/landlock-genprof/discussions/95)
for the open design question.

---

## What this script deliberately does not claim

- No aggregate "confidence score" (e.g. "94% confident") — the tool
  reports confidence per path/port/syscall, not a single number. Don't
  invent one for the video.
- No live policy-denial moment unless verified live first (see above).
- No `--history` multi-run confidence upgrade — that needs several runs
  and doesn't fit a 75s cut; mention it in narration as a follow-up
  capability rather than demoing it.

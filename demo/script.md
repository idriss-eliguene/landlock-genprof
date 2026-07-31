# Demo script — ~75s core cut, ~95s with the enforcement beat

Target audience: someone landing on the repo from the announcement
([Discussion #95](https://github.com/idriss-eliguene/landlock-genprof/discussions/95))
who wants to see the tool actually do something in under two minutes,
before reading a line of docs.

**This is a runbook, not a transcript.** Every command below is real and
matches the current CLI (`cmd/landlock-genprof/`) — but the exact
output (paths, timings, confidence levels) depends on what your VM/cluster
actually observes. Run it for real and paste the real output before
recording; don't reuse the numbers below as if they were captured output.

<!-- x-release-please-start-version -->

## Prerequisites (not part of the recording)

- `kind` cluster + Inspektor Gadget deployed, `nginx-demo` pod running —
  see [`HOW_TO_START.md`](../HOW_TO_START.md).
- CRDs/RBAC applied once: `deploy/rbac.yaml`,
  `deploy/crd-securityprofileproposal.yaml`, `deploy/rbac-proposal.yaml`,
  `deploy/rbac-patched-manifest.yaml`, `deploy/rbac-restart.yaml` — or the
  Helm chart equivalent (`deploy/helm/landlock-genprof`).
- `landlock-genprof` installed as a kubectl plugin (`make install-plugin`,
  or `go install .../cmd/landlock-genprof@v0.1.1` + rename — see
  [`INSTALL.md`](../INSTALL.md)). The shot list below uses
  `kubectl landlock-genprof ...` throughout — that's the form worth
  showing on screen, not `go run`, which only makes sense from a source
  checkout.
- **For the optional "proof of real enforcement" beat near the end
  only:** security-profiles-operator installed — v0.8.4, not v0.7.1,
  and with the two chart-image fixes both applied first. Follow
  [`enforcement-prerequisites.md`](../docs/enforcement-prerequisites.md)
  exactly; skip this and the beat below entirely if you'd rather not set
  it up for the recording. PodLock is not part of this option — see the
  caveat further down.

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
plain `--duration 60s` (with traffic generated the same way, for the
full window) if you go with option 1 instead.

**Either way, generate real traffic during the window — don't skip
this.** `--restart` alone captures nginx's own startup activity (config/
log opens, its own binary being executed) but *not* a client connection
— nothing will call `openat`/`accept4` on nginx's behalf unless
something actually talks to it. Confirmed live, repeatedly, this
session: `--restart` with no traffic produces a real but thin profile
(filesystem-only, `syscalls: 0 item(s)`); combined with even a handful
of requests, the same run produces network *and* syscall data too. The
shot below runs both in parallel for exactly this reason.

---

## Shot list

### [0:00-0:08] The "before"

```bash
kubectl get pod nginx-demo -o jsonpath='{.spec.containers[0].securityContext}'
```

Narration: *"This pod has whatever default permissions containerd gives
it — nothing scoped to what it actually does."* (Expect this to print
nothing or `{}` — that's the point.)

### [0:08-0:32] Run the training run, with real traffic alongside it

In the recording terminal:

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo -n default --binary /usr/sbin/nginx \
  --duration 20s --restart \
  --network-out --seccomp-profile-out --patched-manifest-out
```

In the second terminal, started a couple of seconds after the command
above (give `--restart` a moment to delete+recreate the pod first —
don't fire requests at a pod that's mid-restart):

```bash
kubectl port-forward pod/nginx-demo 8080:80 &
for i in $(seq 1 15); do curl -s http://localhost:8080/ -o /dev/null; sleep 1; done
```

Narration while it runs: *"It observes the pod's real filesystem,
network, and syscall activity via eBPF, while real traffic hits it —
no static config, no guessing."* `--restart` recreates the pod right
before attaching, so the container's startup-time file opens are
captured instead of missed; the `curl` loop is what actually gives the
network/syscall domains something to observe.

> CAPTURE REAL OUTPUT HERE — stdout from this command, including the
> "not-yet-confirmed syscalls" note if one is printed. Trim the second
> terminal's own curl output out of the final cut; it's not meant to be
> on screen, just running.

### [0:32-0:40] Show the generated profile

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

### [0:40-0:50] The punchy summary

```bash
kubectl landlock-genprof review nginx-demo
```

> CAPTURE REAL OUTPUT HERE — the real `WORKLOAD SECURITY REVIEW` block
> (`cmd/landlock-genprof/review.go`): proposal name, container, binary,
> generated-at, history-used, and an availability line per artifact
> (PodLock / NetworkPolicy / Patched Manifest / SPO SeccompProfile).

Narration: *"Every run also publishes this as a `SecurityProfileProposal`
cluster object — reviewable with `kubectl` or GitOps, not just local
files."*

### [0:50-1:05] Apply — the reviewed path, not a raw `kubectl apply`

```bash
kubectl landlock-genprof apply-proposal nginx-demo --restart
```

> CAPTURE REAL OUTPUT HERE — the full artifact list, the `[y/N]` prompt,
> and the per-artifact `applied:`/`failed:` lines after confirming.
> Expect `failed: PodLock` on this project's own `kind` reference
> environment (no PodLock CRD installed — see the caveat below); that's
> real, unstaged output, not an error to edit around.

Narration: *"This is the reviewed path — it prints exactly what it's
about to touch and asks before doing anything. `--restart` here is
opt-in on purpose: it's the one artifact that actually restarts the
target pod, so applying it is a decision, not a default."* (Same three
artifacts are available as local files too —
`nginx-demo-networkpolicy.yaml`/`nginx-demo-seccompprofile.yaml`/
`nginx-demo-patched.yaml` — for a `kubectl apply -f` workflow instead;
not shown here since this shot is about the reviewed path.)

**PodLock caveat.** Neither this shot nor the rest of the recording
should stage PodLock actually enforcing anything: its own docs advise
against this project's entire `kind`-based environment, so
`failed: PodLock — ... could not find the requested resource` is the
honest, expected result here, not a bug to hide. A real PodLock
enforcement beat needs a different reference environment (Lima, per
PodLock's own quickstart) — out of scope for this script.

### [1:05-1:20] Optional: proof of real enforcement

**Only if security-profiles-operator is actually installed** (see
Prerequisites above) — skip this whole beat otherwise, don't fake it.

```bash
kubectl get pod nginx-demo
kubectl get seccompprofile nginx-demo -o yaml | grep -A2 "localhostProfile\|status:"
```

> CAPTURE REAL OUTPUT HERE — `nginx-demo` `1/1 Running`, 0 restarts;
> `status: Installed` and a `localhostProfile` path on the
> `SeccompProfile`. This is the one beat earlier drafts of this script
> hedged on ("a gap you could close if you want") — confirmed live this
> session: the applied `SeccompProfile` really does get reconciled by
> SPO and the pod really does keep running under it, seccomp and
> `NetworkPolicy` both actually enforced, not just generated.

Narration: *"security-profiles-operator picked up what was just applied
and materialized it onto the node — this pod is running under the
seccomp profile that was generated a few seconds ago, from what it
actually did."* Don't claim more than this shows: this proves the
profile is *installed and active*, not that a specific blocked syscall
was demonstrated — no live denial was staged for this recording.

### [1:20-1:30 / 1:05-1:15 without the enforcement beat] Close

```bash
kubectl landlock-genprof trace --help
```

Narration: *"Prototype stage, v0.1.1, feedback wanted — repo link on
screen."* Point at the good-first-issue labels and
[Discussion #95](https://github.com/idriss-eliguene/landlock-genprof/discussions/95)
for the open design question.

<!-- x-release-please-end -->

---

## What this script deliberately does not claim

- No aggregate "confidence score" (e.g. "94% confident") — the tool
  reports confidence per path/port/syscall, not a single number. Don't
  invent one for the video.
- No live policy-denial moment (a blocked syscall/connection actually
  observed getting refused) — the optional enforcement beat above shows
  the profile *installed and active*, which is real and confirmed, but
  distinct from staging an actual denial. Don't blur the two in
  narration.
- No PodLock enforcement of any kind — see the caveat in the apply shot.
- No `--history` multi-run confidence upgrade — that needs several runs
  and doesn't fit this cut either way; mention it in narration as a
  follow-up capability rather than demoing it.

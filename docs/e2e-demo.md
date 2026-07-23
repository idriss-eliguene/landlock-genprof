# M4 — end-to-end demo and gap analysis

This document records the M4 milestone: a real training run against the
`nginx-demo` pod on a `kind` cluster (Ubuntu 26.04 VM, kernel ≥ 6.8),
compared against a hand-written reference profile for the same image and
config. See `docs/roadmap.md` for how this fits the overall milestone
sequence.

## Setup

```bash
go run ./cmd/landlock-genprof trace --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s --out profile-generated.yaml
```

run against the `nginx-demo` pod created by `hack/init-vm.sh`
(`nginx:alpine`, container name `nginx-demo`), with `kubectl exec
nginx-demo -- ls /etc` and `kubectl exec nginx-demo -- cat
/etc/nginx/nginx.conf` run in a second terminal during the 60s window to
generate activity.

- Generated profile: [`examples/nginx-generated-profile.yaml`](../examples/nginx-generated-profile.yaml)
- Hand-written reference: [`examples/nginx-hand-written-reference-profile.yaml`](../examples/nginx-hand-written-reference-profile.yaml)

## Gap analysis

| Path | Generated | Reference | Verdict |
|---|---|---|---|
| `/etc/nginx`, `/usr/lib`, `/usr/share/nginx`, `/proc/self`, `/sys/fs/cgroup` | readOnly | readOnly | match |
| `/etc/ssl` | readOnly | readOnly | match (see note below) |
| `/bin`, `/usr/bin` | readExec | *(absent)* | **false positive — contamination** |
| `/proc/sys/kernel`, `/sys/kernel/mm` | readOnly | *(absent)* | **unclear — needs a second, isolated run** |
| `/run`, `/var/log/nginx` | *(absent)* | readWrite | **false negative — blind spot** |

### Finding 1 — training-run contamination (`readExec: /bin, /usr/bin`)

The Inspektor Gadget filter (`internal/tracer/trace_linux.go`) scopes
events by `namespace`/`podname`/`containername` only — never by the
`--binary` the CLI was given. `--binary` is used solely as a label at
export time (`internal/exporter/podlock/export.go:36`), not as an event
filter. Any process sharing the container's namespaces during the
training window is captured and attributed to the traced binary,
including the `ls`/`cat` we ran via `kubectl exec` to generate synthetic
activity. `trace_exec` fired on those, producing `readExec: /bin,
/usr/bin` — paths nginx itself has no reason to ever execute (no CGI, no
exec directive in this config).

This is a real precision problem for a security-profile generator: it
means anyone with `kubectl exec` access into the pod during a training
run — a debugging session, a sidecar, an attacker — can broaden the
profile that gets deployed for the *actual* target binary. Logged in
`docs/threat-model.md` §2 as a methodology risk, not fixed here (M4 is
about documenting gaps, not closing them).

**Fixed at the tracer level** (`internal/tracer/trace_linux.go`,
`commFromBinaryPath` + a per-event `comm` check in all four
`run*Tracer` functions, added after M4): every event is now additionally
scoped to processes whose `comm` matches `--binary`'s basename, not just
the pod/namespace/container. `kubectl exec nginx-demo -- ls /etc` has
`comm == "ls"`, not `"nginx"`, so it no longer produces a `readExec`
entry. Known limitation, deliberately traded for closing this false
positive: a legitimate child process the traced binary execs under a
*different* comm (e.g. a CGI script) would now be filtered out too — not
a concern for this config (no exec directive), but worth knowing before
reusing this against a target that does spawn differently-named
children. See `internal/tracer/trace_linux.go`'s `commFromBinaryPath` and
`docs/threat-model.md` §2 for the same fix's extension to the network
tracers.

**Live re-verification exposed something the original gap analysis
missed entirely.** Re-running the exact M4 scenario (`ls`/`cat` via
`kubectl exec`, no real traffic to nginx) with the comm filter live
produced a **completely empty profile** — not a bug: `comm` for every
`ls`/`cat`-triggered event is `"ls"`/`"cat"`, not `"nginx"`, and nginx
itself, already running since before the trace window started, had
nothing new to `openat()` during 60s of no real requests. This means the
original table's "match" rows (`/etc/nginx`, `/usr/share/nginx`,
`/usr/lib`, `/proc/self`, `/sys/fs/cgroup` — all attributed to nginx and
compared against the hand-written reference) were almost certainly
**also** `ls`/`cat` contamination that happened to coincide with paths
nginx would plausibly touch (their own dynamic linker opening
`/usr/lib/*.so`, `cat` reading exactly `/etc/nginx/nginx.conf`, etc.) —
the M4 methodology never actually exercised nginx's own behavior at all.
Confirmed by sending real traffic instead
(`kubectl exec nginx-demo -- wget -qO- http://localhost/` during the
window): a clean, correctly-attributed `readOnly: [/usr/share/nginx]` —
nginx serving `index.html` — with no `/bin`/`/usr/bin`, no `/etc/nginx`
(config is read once at startup, not per-request, and nginx has been
running since long before this trace attached — see Finding 2 below for
why that startup read is invisible anyway). **Takeaway for any future
training run**: exercise the target with real traffic to it, not
`kubectl exec` commands that only incidentally resemble what it does.

### Finding 2 — empty `readWrite` (pid file, access/error log)

nginx's master opens `/run/nginx.pid` once at startup and workers open
the log file once (even though it's symlinked to `/dev/stdout`/
`/dev/stderr` in this image, the `openat()` call still targets the
`/var/log/nginx/...` path) — then keep writing to the held file
descriptor for the rest of the process lifetime. `trace_open` only
observes `openat()`, not subsequent `write()`s on an already-open fd. If
the trace attaches to a container that was already running before the
window started, that one-time `openat()` has already happened and is
gone — no amount of `curl`ing the pod during the 60s window will
reproduce it, because the log fd is already open and reused.

Practical implication for a real training-run protocol: the traced
process needs to **restart during the observation window**
(`kubectl delete pod` + recreate, or `kubectl rollout restart` for a
Deployment) to guarantee the startup-time opens are actually captured.
Logged in `docs/threat-model.md` §2 as a completeness/false-negative risk.

**Fixed, opt-in, via `trace --restart`** (`internal/k8s/restart.go`):
automates exactly the two manual steps above — delete+recreate for a
bare pod (`hack/init-vm.sh`'s `nginx-demo`), or the same
`kubectl.kubernetes.io/restartedAt` annotation patch `kubectl rollout
restart` itself uses for a Deployment-owned pod. Not automatic: opt-in
because it's disruptive to the running workload, and needs additional
RBAC beyond the base read-only manifest (see `deploy/rbac-restart.yaml`,
`docs/threat-model.md` §1). StatefulSet/DaemonSet-owned pods aren't
supported yet — `Restart` returns a clear error naming the unsupported
owner kind rather than mishandling it.

**First live attempt exposed a second, subtler timing bug — also
fixed.** The first version of `--restart` restarted the pod, *then*
called `tracer.Trace`: the generated profile came back completely
empty. Root cause: attaching all four gadgets is a real gRPC handshake
per gadget (several hundred ms to a few seconds), reliably slower than
an already-cached image's container start — nginx finished its
one-time startup opens before the tracer had even attached, so
restarting the pod first just moved the blind spot, it didn't close it.
Fixed by reversing the order for the bare-pod case:
`tracer.Trace` now takes an `onReady` callback (fired once all four
gadgets have finished attaching — `internal/tracer/trace_linux.go`),
and `cmd/landlock-genprof/trace.go`'s `traceWithRestart` starts the
tracer *first*, waits for that signal, and only then restarts the pod —
relying on Inspektor Gadget's KubeManager filter to dynamically
re-attach to whichever container matches the same pod name, since it's
already listening before the replacement even exists. The
Deployment-owned case still restarts first (its replacement's name isn't
known until after the restart happens, so it can't be pre-targeted the
same way) — same residual gap, not yet closed for that case.

**Confirmed live.** `trace --restart` on `nginx-demo` produced:

```yaml
spec:
  profilesByContainer:
    nginx-demo:
      /usr/sbin/nginx:
        readExec:
          - /usr/sbin
        readOnly:
          - /etc
          - /etc/nginx
          - /etc/nginx/conf.d
          - /etc/ssl
          - /usr/lib
        readWrite:
          - /run
          - /var/log/nginx
```

`readWrite: [/run, /var/log/nginx]` is exactly the gap this Finding
named — the pid file and log fd, opened once at startup, now actually
observed. `readExec: [/usr/sbin]` is new too, and notable for a
different reason: it's nginx's own master process being `execve`'d,
never previously visible because no prior run had ever actually observed
a real startup. The richer `readOnly` set (`/etc/nginx/conf.d`,
`/etc/ssl`, `/usr/lib`) is nginx's genuine config-time reads, correctly
attributed via the `comm` filter (see Finding 1) rather than incidental
`ls`/`cat` contamination. Findings 1 and 2 are both closed.

### Finding 3 — `/proc/sys/kernel`, `/sys/kernel/mm` (low confidence)

Neither is explained by this config the way `/sys/fs/cgroup` is (`worker_processes
auto` reading `cpu.max`). Not attributable to `ls`/`cat` either (busybox
coreutils on alpine don't touch either path in this simple use). Left out
of the hand-written reference as unexplained rather than asserted wrong
either way — recommend a second, isolated run (no concurrent `kubectl
exec`) before deciding whether to allow or drop them.

## M4 status

Demo run end to end successfully; gaps are documented above rather than
silently present in the deployed profile, which was the actual goal of
M4 (`docs/roadmap.md`: "profile generated for nginx, compared against a
hand-written profile, gaps documented"). Findings 1 and 2, initially
logged as open methodology risks in `docs/threat-model.md`, are now both
fixed and confirmed live: `comm`-based process filtering
(`internal/tracer/trace_linux.go`) for Finding 1, and opt-in target
restart with tracer-attach-before-restart ordering
(`internal/k8s/restart.go`, `cmd/landlock-genprof/trace.go`) for
Finding 2. Finding 3 remains unexplained/open by design (needs a second,
isolated run to resolve either way).

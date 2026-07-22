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
hand-written profile, gaps documented"). Findings 1 and 2 point to two
methodology risks now tracked in `docs/threat-model.md`, and a candidate
follow-up (out of scope for M4 itself): filtering `trace_open`/`trace_exec`
events by the traced process's `comm`/`pid` rather than only by
container, to close Finding 1 at the tracer level instead of only in
documentation.

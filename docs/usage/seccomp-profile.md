# Internal/advisory seccomp JSON (`--seccomp-out`)

This output belongs to `--seccomp-source=internal`. In SPO mode, SPO owns
syscall observation and derives the `SeccompProfile`; `--seccomp-out` is
rejected rather than presenting SPO-derived policy as landlock-genprof
observation.

Pass `--seccomp-out` to generate a plain advisory profile from the same
training run (skipped if no syscalls were observed), via Inspektor
Gadget's own `advise_seccomp` gadget (see Step 2's gadget table):

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64"],
  "syscalls": [
    {
      "names": ["accept4", "capget", "capset", "chdir", "epoll_wait", "fstatfs", "futex", "openat", "read", "setgid", "setgroups", "setuid", "statx", "write"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

`capget`, `capset`, `chdir`, `fstatfs`, `futex`, `setgid`, `setgroups`,
`setuid`, and `statx` are always folded in alongside whatever was actually
traced — none of the nine is something the traced
binary itself calls, but the container runtime (runc) needs all of them
during container init, before it even execs into the binary. Confirmed
live (2026-07-30) one at a time, each the next crash after fixing the
last, all inside runc's own `finalizeNamespace`
(`libcontainer/init_linux.go`), called in this exact order:

1. `capget` — a kernel-capability-version probe. Without it: `OCI
   runtime create failed: ... unable to get capability version from the
   kernel: operation not permitted`.
2. `futex` — runc's own init is itself written in Go, and the Go
   runtime's scheduler/GC depend on futex(2) for their whole lifetime,
   not just one step. Without it, the container gets created but
   crashes immediately (`cannot start a stopped process`), `kubectl logs
   --previous` showing a raw Go runtime panic ("The futex facility
   returned an unexpected error code") inside
   `libcontainer.setupUser`/`finalizeNamespace`.
3. `chdir` — sets the container's configured working directory. Without
   it: `OCI runtime create failed: ... chdir to cwd ("/") set in
   config.json failed: operation not permitted`.
4. `capset` — applies the `securityContext.capabilities` every patched
   manifest this project generates sets explicitly
   (`internal/exporter/capabilities`). Predicted rather than
   independently confirmed by its own distinct crash (it's the very next
   call `finalizeNamespace` makes after `chdir`) — if a live test still
   crash-loops with a *different* error after this, that prediction was
wrong.

5. `fstatfs` — runc verifies that `/proc/thread-self/fd` is backed by
   procfs while closing its exec-fd handoff, after installing seccomp and
   before exec'ing the configured user program.
6. `statx` — runc obtains `STATX_MNT_ID` for the procfs mount-identity
   check on `/proc/thread-self/fd` during the same pre-exec validation.
7. `setgroups`, `setgid`, and `setuid` — establish the container's configured
   group and user identity during runtime initialization. Without them, the
   runtime can fail before `/bin/sh` or another user program is executed.

A trace of the traced binary's own behavior can never observe any of
these, since all nine happen in a separate process phase before exec.
They are runtime baseline requirements, not SPO-observed evidence, and do not
increase SPO coverage or create TrainingHistory facts.

Deliberately plain JSON, not YAML with a `# confidence: ...` comment like
the other two outputs: this file is loaded directly by the kubelet/
container runtime (referenced via a pod's
`securityContext.seccompProfile.localhostProfile`, never `kubectl
apply`d), so it has to stay valid, comment-free JSON. Instead, the CLI
prints the syscalls not yet confirmed across multiple `--history` runs to
stdout after writing the file — on a single run without `--history`, that
means every syscall, since `advise_seccomp` reports one deduplicated set
per run rather than a per-occurrence count, so `Confidence` can only ever
be `low` until `--history` accumulates more runs.

This standalone file has no proposal authority on its own. It is worth taking seriously before enforcing: a missing syscall
doesn't just narrow access like an overly-strict `NetworkPolicy` would —
it breaks the container outright. Prefer `--history` over a single internal
run, then use the governed proposal workflow rather than treating confidence
as authorization.

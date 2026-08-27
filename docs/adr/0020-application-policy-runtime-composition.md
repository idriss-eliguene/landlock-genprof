# ADR-0020: Fail-closed application-policy/runtime composition

## Status

Accepted

## Decision

Application-derived syscall authority (`A`) and bootstrap/runtime
requirements (`B`) are distinct domains. The effective authority (`E`) must
not silently become `A ∪ B`: runtime requirements must never be laundered as
application observations or enlarge post-exec workload authority.

The current OCI Seccomp path provides no supported, race-free transition from
bootstrap-only authority to application-only authority. Therefore
`PodLock + application-derived Seccomp` is `UNSUPPORTED_FAIL_CLOSED` unless
authoritative, version-bound compatibility evidence proves a safe composition.
Unknown or missing compatibility evidence is incompatible. This decision is
about selected artifact composition, not artifacts merely present in a
proposal; mono-artifact projections remain supported.

Compatibility must be decided over the complete selected set before the first
artifact mutation. Approval, ordering, names, duplicates, or caller-supplied
compatibility claims cannot establish compatibility authority.

## Evidence and boundary

The motivating counterexample is conditional runc behavior: when an
application profile omits `close_range`, runc's `CloseExecFrom` falls back to
proc-fd handling requiring `fstatfs`/`openat2`, while the persistent Seccomp
filter is already active. The failure occurs before workload exec. This does
not claim PodLock intrinsically requires those syscalls, nor that this is a
complete runtime contract or universal impossibility.

The decision may be revisited if an upstream-supported two-domain mechanism or
an authoritative, versioned runtime/enforcement compatibility contract is
available.

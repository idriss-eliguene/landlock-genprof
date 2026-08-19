# ADR-0001: Exit-code contract (0/1/2/3)

Status: Accepted

Date: 2026-08-07

## Context

Several commands (`doctor`, `abi`, `verify`, `diff`, `policy status`) are
meant to be used as CI gates, not just read by a human. A plain Go error
only lets `main()` distinguish success from failure — it can't tell a CI
pipeline "the input itself was invalid" apart from "the check ran fine
and found something worth blocking on" apart from "the check ran fine
and found something worth flagging but not blocking." Without that
distinction, a broken invocation and a real finding look identical to
an automated caller.

## Decision

The contract: every command must return either `nil` (exit `0`, clean)
or an error implementing `interface{ ExitCode() int }` — the
`exitCodeError` type (`cmd/landlock-genprof/doctor.go`) — carrying one
of:

- `1` — non-blocking finding
- `2` — blocking failure
- `3` — usage error (bad input, not a finding about the input's content)

`main.go` checks for this interface and exits accordingly; a bare error
with no `ExitCode()` defaults to `1`. Once a command ships this
contract, it's frozen — never renumbered (`docs/cli-design.md`'s
stability policy).

## Consequences

**Easier:** a CI pipeline can branch on exit code alone, no text
parsing; `verify --output sarif`/`diff --output junit` carry the same
signal in machine-readable form for dashboards that don't read exit
codes directly.

**Costs:** the contract has to be gotten right per command, by hand —
nothing currently enforces it automatically across the whole command
set. That's a real, already-realized risk, not a hypothetical one:
`verify`'s own usage-error paths originally defaulted to the generic
`1` instead of `3` and shipped that way before being caught and fixed.
Only 5 of the ~15 commands (`abi`, `diff`, `doctor`, `policy`, `verify`)
currently have tests asserting their exit codes explicitly.

## References

- `cmd/landlock-genprof/main.go` — the `ExitCode()` check
- `cmd/landlock-genprof/doctor.go` — `exitCodeError` definition
- `docs/cli-design.md` — stability policy, "the exit-code contract is
  the highest-stakes promise in this design"
- `cmd/landlock-genprof/{abi,diff,doctor,policy,verify}_test.go`

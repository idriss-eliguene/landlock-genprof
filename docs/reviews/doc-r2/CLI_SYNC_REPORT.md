# DOC-R2 CLI Synchronization Report

## Source-derived tree

The current Cobra tree is:

```text
trace, observe, synthesize, review, apply-proposal, rollback,
custody-epoch activate, approve, reject, doctor, abi list/check, verify,
explain, export, diff, evidence show/list, policy list/status, ui, healthz,
executor, version
```

## Changes

- Regenerated `book/src/cli/*` using the repository's `gendocs` command.
- Added `observe` and `executor` to `book/src/SUMMARY.md`.
- Updated the `ui` page's user-facing description to Operations Center and
  current local/authenticated action boundaries.
- Preserved exact generated synopses and flags from Cobra.

## Validation

`CLI_DOC_SYNCHRONIZED=YES` for command names, subcommands, synopses, flags,
and examples generated from source. The `ui` command's source short string
still contains historical Workbench wording, but the user-facing generated
page is bounded to current runtime behavior without modifying product code.

Examples remain environment-dependent where they require a live Kubernetes
cluster, executor, RBAC, or backend. No governance-changing examples were
executed during DOC-R2.

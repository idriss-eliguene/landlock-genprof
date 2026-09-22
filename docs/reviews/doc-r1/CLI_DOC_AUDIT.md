# DOC-R1 CLI Documentation Audit

## Authoritative command tree

Derived from `cmd/landlock-genprof/root.go` and the Cobra generator, not from
documentation:

```text
landlock-genprof
├── trace
├── observe
├── synthesize
├── review
├── apply-proposal
├── rollback
├── custody-epoch
│   └── activate
├── approve
├── reject
├── doctor
├── abi
│   ├── list
│   └── check
├── verify
├── explain
├── export
├── diff
├── evidence
│   ├── show
│   └── list
├── policy
│   ├── list
│   └── status
├── ui
├── healthz
├── executor
└── version
```

The generated source tree contains 29 Markdown command pages. The checked-in
Book contains 27: `observe` and `executor` are missing from
`book/src/SUMMARY.md` and `book/src/cli/`.

## Discrepancies

| Implementation | Documentation | Difference | Severity | Recommended fix |
|---|---|---|---|---|
| `newObserveCmd()` in `cmd/landlock-genprof/observe.go` | `book/src/SUMMARY.md`, `book/src/cli/` | Current `observe` command is absent from the Book navigation/reference. | P1 | Regenerate CLI pages and add the command to the Book summary. |
| `newExecutorCmd()` in `cmd/landlock-genprof/executor.go` | `book/src/SUMMARY.md`, `book/src/cli/` | Current executor command is absent from the Book navigation/reference. | P1 | Decide whether this operator/developer command is user-facing; if yes, regenerate and document its bounded role. |
| `newWorkbenchCmd()` and `runWorkbench()` in `cmd/landlock-genprof/workbench.go` | `book/src/cli/landlock-genprof_ui.md` | Generated source says local Workbench; checked-in page says “read-only Workbench” and describes the old fixed proposal/server-rendered surface. | P0 | Regenerate from current source, then add a current Operations Center usage section. |
| `newRootCmd()` in `cmd/landlock-genprof/root.go` | `book/src/cli/landlock-genprof.md` | Existing root page lacks two current SEE ALSO entries. | P1 | Regenerate. |
| Current `approve`, `reject`, `apply-proposal`, and `rollback` commands | README Operations Center section and `book/src/workbench.md` | Current React product includes governance/apply/attempt surfaces, while user-facing text says browser is read-only and actions remain CLI-only. | P0 | Distinguish read-only local mode from authenticated Operations Center mode; document server authorization and CAS. |
| `cmd/landlock-genprof/workbench.go` | `docs/cli-design.md` | Design document labels a “target shape, not current” tree while multiple listed commands are now shipped and others remain unimplemented. | P2 | Keep as design history, but add a prominent historical/design-only label and link to generated current reference. |

## Flag and example audit

The generated command pages match current Cobra help for the inspected command
flags, including `--namespace`, `--port`, `--duration`, `--pod`, `--container`,
`--binary`, `--expected-digest`, `--yes`, output flags, and verification/export
flags. The main risk is not flag drift; it is missing pages and stale product
semantics around `ui`.

Examples in README and `book/src/workflow.md` use the current core lifecycle:
`trace → review → approve → apply-proposal`, with explicit digest binding.
They omit the newer `observe`/`executor` operational paths and the current
Operations Center governance/attempt workflow.

## CLI lifecycle conclusion

CLI documentation explains a credible governed lifecycle, but it does not
clearly distinguish the CLI candidate-v1/artifact workflow from Operations
Center candidate-v2/read-model workflow. The command reference should be
regenerated before public positioning is updated.

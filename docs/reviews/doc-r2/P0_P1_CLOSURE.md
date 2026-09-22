# DOC-R2 P0/P1 Closure

## P0 closure

### 1. Current CLI surface was incomplete

- **Finding:** `observe` and `executor` existed in the Cobra tree but were
  absent from the checked-in Book navigation/reference.
- **Files changed:** `book/src/SUMMARY.md`, generated `book/src/cli/*`.
- **Old semantics:** the Book implied the generated CLI reference was complete.
- **New semantics:** the Book lists the current command tree and generated
  pages include both commands.
- **Implementation basis:** `cmd/landlock-genprof/root.go`, `observe.go`,
  `executor.go`, and Cobra-generated output.
- **Status:** CLOSED.

### 2. Retired Workbench was presented as current

- **Finding:** README, Book, CLI `ui` page, demo instructions, and migration
  text described the old Workbench as current or used retired `/next/` routes.
- **Files changed:** README, `book/src/index.md`, `book/src/workbench.md`,
  `book/src/cli/landlock-genprof_ui.md`, `docs/operations-center-demo.md`,
  Operations Center migration/product references.
- **Old semantics:** server-rendered/read-only Workbench at the old route.
- **New semantics:** React Operations Center is canonical at `/`; `/next/*`
  is retired; historical Workbench content is explicitly labeled historical.
- **Implementation basis:** M10.9-B2/B3/B4 routing and retirement state.
- **Status:** CLOSED.

### 3. CLI candidate-v1 and Operations Center candidate-v2 were ambiguous

- **Finding:** the distinction existed in ADRs/internal docs but was not
  visible in the public workflow.
- **Files changed:** new `book/src/operations-center.md`, updated README and
  lifecycle wording.
- **Old semantics:** related candidate terminology could be read as shared
  object/digest identity.
- **New semantics:** candidate-v1 and candidate-v2 are related lifecycle
  representations, not automatically the same object, digest, persistence, or
  lineage.
- **Implementation basis:** candidate-v1 digest ADRs and Operations Center
  candidate-v2 read-model contract.
- **Status:** CLOSED.

## P1 closure

- Current Operations Center guide added.
- Health, Attention, exact lineage, governance custody, and troubleshooting
  added to the public Book path.
- Current CLI reference synchronized and surfaced in Book navigation.
- README now describes the current product model and six-item L1 navigation.

`P0_REMAINING=0`  
`P1_REMAINING=0`

# DOC-R1 Documentation Audit Report

## Scope and method

Audited the repository at `0649068ebda54b0f57e8e8a14360fb7b723ab1f2` without
modifying existing documentation or product code. The CLI inventory was
derived from the Cobra command tree and a temporary generated-docs directory;
the checked-in Book was compared with that generated output. User-facing
claims were searched across README, Book, demos, usage, product docs, release
records, ADRs, and progress/certification records.

## Executive findings

- CLI implementation is authoritative and contains `observe` and `executor`
  pages absent from the checked-in Book.
- The checked-in `ui` CLI page and Book Workbench chapter describe a retired or
  superseded surface.
- The Book is present but `PARTIAL / STALE` as a final product guide.
- Operations Center documentation is `PARTIAL`: internal semantic coverage is
  strong, public onboarding coverage is missing.
- README is `PARTIAL`: strong lifecycle and limitation language, but stale
  read-only/action and navigation claims remain.
- Real React-era screenshots exist, but the set is incomplete for final Health,
  root, context, and lineage states.
- Candidate-v1 and candidate-v2 are distinguished in ADRs and semantic docs,
  but the distinction is not consistently visible in user-facing material.
- Evidence/source/backend boundaries are generally well documented. The main
  danger is old product framing, not systematic semantic collapse.

## Required outputs

See the companion files in this directory:

- `DOCUMENTATION_INVENTORY.md`
- `CLI_DOC_AUDIT.md`
- `PRODUCT_TERMINOLOGY_AUDIT.md`
- `CLAIM_SAFETY_AUDIT.md`
- `OPERATIONS_CENTER_DOC_AUDIT.md`
- `BOOK_INFORMATION_ARCHITECTURE_REVIEW.md`
- `SCREENSHOT_PLAN.md`
- `README_AUDIT.md`
- `EXAMPLE_VALIDITY.md`
- `DOCUMENTATION_REMEDIATION_BACKLOG.md`

## Audit classifications

```text
CLI_IMPLEMENTATION_INVENTORIED=YES
CLI_DOC_STATUS=PARTIAL / STALE
CLI_STALE_ITEMS=observe missing; executor missing; ui description stale

BOOK_PRESENT=YES
BOOK_STATUS=PARTIAL / STALE

OPERATIONS_CENTER_DOC_STATUS=PARTIAL
OPERATIONS_CENTER_MISSING_TOPICS=public guide, final Health, exact custody,
candidate-v1/v2 boundary, current context/lineage workflow

CURRENT_REACT_SCREENSHOTS_PRESENT=YES, PARTIAL
LEGACY_SCREENSHOTS_PRESENT=NO direct legacy asset; stale Workbench terminology
SCREENSHOTS_RECOMMENDED=7

README_STATUS=PARTIAL / NEEDS REMEDIATION

CLI_V1_OC_V2_CONFLATION_FOUND=AMBIGUOUS LOCATIONS FOUND
BEHAVIOR_SOURCE_BACKEND_CONFLATION_FOUND=LIMITED / MOSTLY CONTROLLED
APPLIED_VERIFIED_CONFLATION_FOUND=NO SYSTEMIC CLAIM; historical wording requires review
LANDLOCK_ENFORCEMENT_OVERCLAIM_FOUND=NO IN CURRENT CLAIM LEDGER; stale contexts need labels
UNIVERSAL_IR_OVERCLAIM_FOUND=NO EXPLICIT CURRENT CLAIM; terminology ambiguity remains
SECURITY_SCORE_CLAIM_FOUND=NO CURRENT SCORE CLAIM; roadmap/history references exist
```

## Priorities

```text
P0_COUNT=3 work items
P1_COUNT=4 work items
P2_COUNT=3 work items
P3_COUNT=4 work items
P4_COUNT=4 work items
```

## Recommendation

`RECOMMENDED_BOOK_CHAPTERS=What is landlock-genprof; Operations Center;
Troubleshooting; current CLI reference integration`

`RECOMMENDED_SCREENSHOT_COUNT=7`

`DOCUMENTATION_READY_FOR_PUBLIC_PRODUCT_POSITIONING=NO`

`DOC_R2_REMEDIATION_REQUIRED=YES`

The product implementation is not being rejected. Documentation is not yet
safe as the sole public product explanation because the front door and Book
still expose superseded Workbench-era semantics.

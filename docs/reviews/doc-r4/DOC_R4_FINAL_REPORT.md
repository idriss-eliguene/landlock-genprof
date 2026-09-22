# DOC-R4 Final Documentation Certification

This record covers the final documentation validation pass before the
controlled documentation commit.

## Custody

```text
HEAD_BEFORE=0649068ebda54b0f57e8e8a14360fb7b723ab1f2
HEAD_AFTER=SEE_FINAL_RESPONSE_COMMIT_SHA
BRANCH=feat/operations-center-frontend-migration-v1
PRODUCT_CODE_MODIFIED=NO
DEMO_B1_REPORT_MODIFIED=NO
UNRELATED_CHANGES_PRESERVED=YES
```

## Gate results

```text
DOC_R1_FINDINGS_CLOSED=YES
DOC_R2_REMEDIATION_VERIFIED=YES
DOC_R3_SCREENSHOTS_VERIFIED=YES
SCREENSHOTS_AUTHENTIC=7
SCREENSHOTS_INTEGRATED=7
BOOK_BUILD=PASS
BROKEN_LINKS=0
MISSING_ASSETS=0
README_STATUS=PASS
CLI_REFERENCE_STATUS=PASS
OPERATIONS_CENTER_GUIDE_STATUS=PASS
BOOK_VISUAL_STATUS=PASS
SEMANTIC_REVIEW=PASS
NEW_DOCTEST_FAILURES=0
```

## Screenshot verification

All seven PNG assets under `book/src/assets/operations-center/doc-r3/`
are valid PNG files and are linked from `book/src/operations-center.md`
with semantic alt text and captions. The DOC-R3 capture register and
semantic review provide provenance for every image. The set contains
current React Operations Center captures; reused captures are explicitly
identified there rather than being presented as one atomic browser session.

The rendered Book contains all seven images, with readable presentation,
captions, and no missing image requests. No image presents the retired
Workbench or `/next/` as current. The surrounding text preserves the
candidate-v1/candidate-v2 boundary, `APPLIED != VERIFIED`, source-specific
evidence, explicit uncertainty, and bounded Health semantics.

## mdBook test classification

The canonical `mdbook build book --dest-dir /tmp/doc-r4-book` passes.

`mdbook test book` remains non-zero after the smallest safe authored-doc
fixes. The remaining failures are limited to 28 ignored generated pages
under `book/src/cli/`, including the generated `observe` and `executor`
pages. Their synopsis, options, and examples are plain CLI text in fences
that rustdoc interprets as Rust doctests. These files are generated and
ignored by the repository; they are not authored DOC-R4 pages. The
previously failing authored pages in `demo/README.md`,
`docs/usage/spo-seccomp-import.md`, and `CONTRIBUTING.md` were corrected
with explicit text fences and no longer fail.

```text
MDBOOK_TEST=FAIL
PRE_EXISTING_FAILURES_REMAINING=28 generated ignored CLI chapters
NEW_DOCTEST_FAILURES=0
```

The remaining generated-page failures are a documentation-tooling
limitation, not broken product examples. No CLI syntax or product code was
changed to silence them. They are not a publication blocker for the
rendered Book, but the repository does not have a fully green mdBook test
gate until generated CLI fence generation is addressed separately.

## Publication decision

```text
DOCUMENTATION_READY_FOR_PUBLICATION=YES_WITH_MDBOOK_TEST_LIMITATION
BLOCKERS=No authored-documentation or rendered-Book blocker. Full mdBook doctest status remains non-green solely for 28 ignored generated CLI pages; this is explicitly retained and classified above.
```

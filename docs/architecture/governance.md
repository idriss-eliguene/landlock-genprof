Architecture Governance Guide
=============================

Purpose
-------
This guide documents landlock-genprof's architecture governance: how RFCs,
ADRs, implementation plans, code, and conformance evidence relate; the review
and hostile-review methods the project uses; and the policies for identity-
sensitive changes and replay-safe migrations.

Scope
-----
This is documentation only. It records durable project governance and review
methods. It does not change code, tests, RFCs, ADRs, golden files, or runtime
semantics.

Governance hierarchy
--------------------
RFC
  ↓ constrains
ADR
  ↓ guides
Implementation plan / workflow
  ↓ executes
Code (implementation)
  ↓ verified by
Tests / Conformance Evidence

Layer definitions
-----------------
RFC
- A normative specification of the domain model (primitives, axioms,
  invariants, identity rules). RFCs define what MUST remain true across all
  implementations. RFCs MUST NOT prescribe representation, language, or
  implementation sequencing.

ADR
- A durable architecture decision recorded within the constraints of RFCs.
  An ADR documents context, alternatives considered, chosen design, rationale,
  consequences, and replay/identity impact where applicable. ADRs do not
  override RFC axioms; if they conflict, the ADR must STOP and the RFC must
  be challenged explicitly.

Implementation plan / workflow
- A concrete plan describing how an accepted ADR is implemented safely. This
  may include implementation stages (non-normative names), test sequencing,
  migration steps, and rollout order. Workflow gate names are implementation
  artifacts and are not normative architecture concepts.

Implementation (code)
- The production code realizing RFC + ADR decisions. Implementations may
  evolve within the constraints of RFCs and ADRs; changes that would alter
  normative semantics require ADR/RFC updates per the process below.

Conformance evidence (tests)
- Tests and artifacts that demonstrate implementation conformance: unit tests,
  golden identity tests, hostile/countermodel tests, replay fixtures, and
  CI policies. Tests are evidence, not normative sources.

Decision classification guidance
--------------------------------
New design question → "rewrite test": if the rule must hold regardless of
language or implementation, it is likely RFC-level. If it is a durable
architectural choice but not universally required, it is ADR-level. Otherwise
it belongs in the implementation plan or an issue.

RFC Conflict STOP rule
----------------------
If a proposed ADR contradicts an accepted RFC, STOP. The ADR author must:
1. construct a countermodel showing the contradiction;
2. determine whether the RFC or the ADR is incorrect;
3. if the RFC is insufficient, propose an explicit RFC revision with
   consequences and migration analysis;
4. only after RFC resolution resume ADR work.

Design review method
--------------------
1. Define the domain and scope.
2. Inventory existing primitives, owners, and frozen identities.
3. Separate facts from proposed semantics (repository fact, RFC requirement,
   ADR decision, implementation behavior, or assumption).
4. List axioms/invariants and identify which are normative.
5. Propose candidate models and document consequences.
6. Construct hostile countermodels to attempt falsification.
7. Reject models violating RFC invariants or identity guarantees.
8. Classify the decision (RFC / ADR / implementation plan).
9. Freeze identity-sensitive contracts before refactors.
10. Implement in focused slices and verify via conformance evidence.

Terminology
-----------
- Axiom: foundational rule in the model/specification.
- Invariant: a property that must hold across valid system states.
- Assumption: a temporary proposition used for analysis; must be explicit.
- Derived property/Theorem: a property proven from axioms/invariants.
- Implementation fact: a property of current code, not normative by itself.
- Countermodel: a concrete scenario demonstrating a claimed rule fails.

Identity-sensitive change policy
--------------------------------
Any change affecting SubjectIdentity, ActIdentity, Proposition identity,
AssertionEventIdentity, ProducerRef, EvidenceGroups, BeliefState, or replay
reconstruction must include:
- explicit identity-impact analysis;
- old vs new identity functions and counterexamples;
- replay/version compatibility definitions;
- frozen golden tests for the old behavior;
- new golden tests for the new behavior.

Hostile architecture review
---------------------------
A hostile review tries to falsify a proposed architecture. The review must
explicitly test malformed input, duplicate keys, missing data, attacker-
controlled metadata, and replay determinism. A PASS means no unresolved
blocker remains in scope; it does not imply design immutability.

Review levels
-------------
RFC review: focus on definitions, axioms, invariants, identity.
ADR review: focus on RFC compliance, failure semantics, migration, trust
boundaries.
Implementation conformance review: focus on tests, golden identities, replay
fixtures, and CI.

ADR lifecycle
-------------
Statuses: Proposed → Accepted → Superseded | Deprecated.
Accepted ADRs are historical records; do not rewrite an Accepted ADR — create a
new ADR and mark the old one Superseded.

Repository examples
-------------------
- ADR-0003: extract-domain-projectors — ADR (implementation architecture).
- ADR-0004: producer granularity and provenance policy — ADR (semantic
  boundary decision).
- ADR-0005: evidence provenance model — ADR (provenance persistence and
  validation).
- RFC-0001 / RFC-0002: normative semantic foundations and vocabulary rules.

Governance checklist
--------------------
Before accepting an ADR:
- verify RFC compatibility (rewrite test);
- produce candidate countermodels and hostile-review notes;
- document identity-impact and replay implications;
- record implementation stages and test plan for non-interference.

Document lifecycle and maintenance
---------------------------------
This guide is itself normative documentation of process. Changes to the guide
should be proposed via an ADR when they alter the project's governance model.

Contact
-------
Architecture owners: refer to project maintainers and repository owners.


Status
======

1. Status

This document is a draft pre‑RFC describing normative choices that build on RFC‑0001 (commit 82d1e54a270fea853c53e70f744891966c602dc9). RFC‑0001 is authoritative and frozen; this document does not modify RFC‑0001, introduce new primitives, or redefine existing concepts. It records only the NEW normative decisions authorized by the RFC‑0001 audit and the derived properties explicated from RFC‑0001.

2. Abstract

This draft defines canonical patterns and minimal preservation requirements for Term minting, the recording of Grounding and Consequence Conditions, Minting Authority metadata, and delegation representation. It ensures historical interpretability while conforming to the primitives and invariants of RFC‑0001.

3. Motivation

Interoperable vocabulary publishing and long‑term interpretability require a small set of normative decisions that RFC‑0001 intentionally left open. This document narrows those choices to enable consistent minting and evolution practices without changing RFC‑0001.

4. Scope

Normative scope includes only the NEW decisions authorized by the RFC‑0001 audit:
- minimal Minting Authority metadata;
- Grounding Conditions representation required at minting;
- Consequence Conditions representation required at minting;
- canonical minting pattern (Act + Assertion Event template);
- delegation modelling (assertion pattern);
- preservation mechanism for identity snapshots (in‑Graph);
- reconstructability requirements.

This document MUST NOT introduce primitives, encodings, transport protocols, or implementation-level identifiers.

5. Non-goals

- Do NOT change RFC‑0001 text or behavior.
- Do NOT mandate serialization formats or transport protocols.
- Do NOT require particular cryptographic encodings or identifier schemes.
- Do NOT introduce new semantic primitives.

6. Dependencies on RFC-0001

Normative derivations and allowed choices reference RFC‑0001 clauses and axioms. In particular:
- Term identity includes Minting Authority, Arity, Argument Sorts, Grounding Conditions as‑of‑minting, and Consequence Conditions as‑of‑minting (RFC‑0001 §8.4.1).
- Append-only / immutability constraints: RFC‑0001 A4 (append-only, no retroactive mutation).
- Reconstructability requirement: RFC‑0001 C8 (historical reconstruction requirement).
- Temporal/quantification primitives and defeasible labeling: RFC‑0001 A6 and A7.

All derived properties in this document cite RFC‑0001 sections/axioms precisely where used.

7. Terminology

Terms defined in RFC‑0001 are reused with identical meaning. Additional labels in this document:
- Minting Authority: the Source identity component used in the Term identity tuple (RFC‑0001 §8.4.1).
- Minting Act: an Act that uses evidence and outputs an Assertion Event declaring a Term (pattern, not a new primitive).
- Identity snapshot: the set of Graph Propositions recorded at minting that together constitute the identity-defining Grounding and Consequence Conditions.

8. Minting Authority (NEW NORMATIVE DECISIONS)

Normative statements:
- The Minting Authority component recorded in a Term declaration MUST identify a recorded Subject/Source in the Graph. The Authority component is an identity atom: two different recorded Subject identities are distinct for identity purposes unless they are the same recorded Subject (DERIVED from RFC‑0001 §8.4.1).

- The following metadata about the Minting Authority MUST be asserted in the Graph at the time of minting (NEW):
  - authority-label: a human-readable label (MAY be present; NOT identity material).
  - authority-assertion: an Assertion Event that declares the Subject/Source and any delegation relationships required for later resolution (MUST be present and preserved in-Graph).
  - authority-provenance-record: an Assertion Event (or set of events) describing provenance statements that enable distinguishability and reconciliation (MUST be present and preserved in-Graph). The format of these Propositions is not prescribed; only their presence is required.

Normative reconciliation rule (NEW):
- A reconciliation/mapping Assertion MAY relate two distinct Authority Subjects for federation or semantic reconciliation purposes. Such a reconciliation Assertion:
  * is itself an Assertion Event recorded in the Graph;
  * does NOT mutate or rename the original recorded Subject identities; and
  * MUST NOT retroactively change the Authority component of existing TermIdentity tuples.

Rationale and constraints:
- RFC‑0001 §8.4.1 requires Authority as part of the Term identity; Authority in RFC‑0001 is a recorded Subject/Source and therefore identity material (DERIVED).
- RFC‑0002 MUST NOT equate reconciliation with identity. Implementations MAY use reconciliation Assertions to present a mapped view, but recorded identities remain authoritative and immutable (RFC‑0001 A4).

OPEN DESIGN QUESTION: RFC‑0002 SHOULD recommend a minimal set of provenance claims to ease reconciliation (for example: canonical Subject label, assertion provenance, and issuance context). The exact list is deferred to governance and is not required to be normative for Draft 1.

9. Term Declaration

Normative statements:
- A Term declaration MUST be produced by an Assertion Event whose Proposition includes an explicit encoding of the Canonical Term Identity tuple (DERIVED requirement; structure below) as Graph content.
- The Assertion Event announcing the Term declaration MUST reference the producing Act via produced-by and MUST be preserved immutable in the Graph (DERIVED from RFC‑0001 Act/Assertion primitives and A4).

10. Canonical Term Identity

Derived property (RFC‑0001 §8.4.1): The canonical Term identity tuple is:

TermIdentity = ⟨Minting Authority, Arity, Argument Sorts, Grounding Conditions (as‑of‑mint), Consequence Conditions (as‑of‑mint)⟩

Normative statement (DERIVED): The Term declaration Assertion Proposition MUST include values for each tuple component. The representation is a collection of Graph Propositions linked to the Assertion Event; encodings are out of scope.

Snapshot identity comparison (NEW NORMATIVE CLARIFICATION - d2):

This section replaces any language invoking a "canonical structural form" or
serialization-driven identity. RFC‑0002 defines StructuralEqual, a
representation-independent, recursive equality relation over snapshot objects.
Identity comparison for TermIdentity MUST use StructuralEqual. StructuralEqual is
mathematical and does not depend on serialization, byte order, or hashing.

10.1 Snapshot mathematical domain

A snapshot is a finite structured object built from the following primitive
constructs (all representation-independent):
- atomic literals (numbers, booleans, strings) and RFC‑0001 recorded identity
  tokens (Subjects, Terms, Assertion Events as recorded by RFC‑0001);
- ordered tuples (finite sequences where position is semantically significant);
- mathematical sets (finite unordered collections where order is not
  semantically significant and duplicates have no multiplicity semantics);
- records/maps keyed by semantic field identity (field names are part of the
  semantic record);
- structured Propositions formed from predicate identity and argument
  structures that are themselves ordered tuples or set-valued components;
- explicit references to recorded Graph identities (Assertion Event id, Term id,
  Subject id) as atomic reference values.

The snapshot domain MUST NOT rely on bytes, hashes, JSON, RDF, UUIDs, URIs or
any implementation-local addresses.

10.2 StructuralEqual definition (normative)

StructuralEqual(x,y) is defined recursively over the snapshot domain:

- Atomic recorded identities:
  StructuralEqual(a,b) iff RFC-0001 recordedIdentity(a) = RFC-0001 recordedIdentity(b).

- Atomic literals:
  Numeric, boolean, and string literals compare by their semantic value.

- Ordered tuples:
  StructuralEqual(⟨x1,...,xn⟩, ⟨y1,...,yn⟩) iff n = m and for all i in 1..n,
  StructuralEqual(xi, yi) holds.

- Mathematical sets:
  StructuralEqual(X, Y) iff there exists a bijection f : X → Y such that for
  every x in X, StructuralEqual(x, f(x)).
  (Implementations may use the equivalent pairwise existence checks, but the
  semantic definition is bijection-based to avoid hidden multiset assumptions.)

- Records/maps:
  StructuralEqual(mapA, mapB) iff mapA and mapB have the same set of field
  names and for every field name F, StructuralEqual(mapA[F], mapB[F]) holds.

- Structured Propositions:
  StructuralEqual(P1, P2) iff P1 and P2 reference the same recorded predicate
  identity, have the same argument arity, and their corresponding ordered
  arguments compare by StructuralEqual; any arguments declared as set-valued
  compare by set StructuralEqual.

StructuralEqual MUST NOT invoke entailment, subsumption, belief state,
reconciliation, or any form of semantic equivalence reasoning. Two distinct
recorded Assertion Events or Propositions that are semantically equivalent do
NOT become StructuralEqual solely for that reason.

10.3 Atomic reference rule

When a snapshot contains an explicit reference to an Assertion Event or other
recorded object (reference(E)), StructuralEqual compares by recordedIdentity(E)
only. StructuralEqual MUST NOT attempt to recursively expand referenced
Assertion Events for identity comparison. This rule ensures termination and
representation independence.

10.4 Ordered / unordered classification (normative)

RFC‑0002 declares the following snapshot components as ORDERED or UNORDERED
for identity purposes. Implementations MUST treat these classifications as
semantic (not presentation) choices:

ORDERED:
- Argument Sorts (argument position matters for Term arity and application).
- Predicate positional arguments except where the predicate explicitly declares
  a particular argument position as set-valued.

UNORDERED (set-valued):
- declared-producers;
- evidence-references;
- assumptions-list;
- declared-entailments;
- consequence-preconditions;
- referenced-terms when represented as a collection rather than positional
  predicate arguments.

For set-valued components, duplicates have no semantic multiplicity; treat them
as mathematical sets. If a particular predicate or field requires different
ordering semantics, RFC‑0002 MUST be amended to record that explicitly.

10.5 Cross-Graph and reconciliation

If two snapshots reference different recorded Assertion Event identities or
Subject identities, StructuralEqual is false unless the recorded identities are
identical. An explicit reconciliation/mapping Assertion MAY relate distinct
recorded identities for federation or query views, but such reconciliation is a
separate, non-mutating operation and does NOT make StructuralEqual true.

10.6 Cycles and termination

StructuralEqual treats identity-bearing references as atomic; cycles in the
Graph therefore do not require coinductive reasoning. Structural equality
terminates by comparing recorded identities rather than recursively expanding
entire dependency closures.

10.7 Independent implementation requirement

Given the same abstract recorded Graph and snapshots, any two conforming
implementations MUST return the same StructuralEqual truth value. Implementers
may use different algorithms or representations but MUST implement the
StructuralEqual semantics exactly.

10.8 Interaction with preservation

StructuralEqual is independent of preservation decisions (d4). Preservation
ensures the referenced Assertion Events and proximate dependencies remain
available for interpretation; StructuralEqual uses recorded identities and the
snapshot structure only.



11. Grounding Conditions (NEW NORMATIVE DECISIONS)

Purpose: to make "grounding as of minting" interpretable indefinitely.

Normative requirements (each is NEW unless marked DERIVED):
- The identity-defining Grounding Conditions MUST be recorded as an Identity snapshot in the Graph at the time of minting (NEW). External references alone are NOT sufficient (NEW).

- Minimum fields that MUST be present in the grounding snapshot (NEW decisions):
  - declared-producers: an explicit Proposition listing the role(s) or Subject(s) authorized to produce assertions grounding the Term (DERIVED from RFC‑0001 Grounding concept; recording is NEW requirement).
  - evidence-references: explicit links to the Assertion Event(s) or other Graph Propositions used as evidence for the minting Act (DERIVED from RFC‑0001 provenance primitives; presence in snapshot is NEW).
  - validity-scope: if the grounding is time-scoped or context-scoped, an explicit Proposition expressing that scope MUST be included (DERIVED when applicable from RFC‑0001 A6; requirement to include it when used is DERIVED).
  - assumptions-list: an explicit Proposition or set of Propositions enumerating the essential assumptions/premises used in grounding (NEW). Recording assumptions separately is a normative choice to ensure reconstructability.

Preservation and dependency closure (NEW NORMATIVE CLARIFICATION - d4):
- For every identity-defining snapshot, every explicitly referenced Assertion Event needed to interpret that snapshot MUST remain available in the append-only Graph. Implementations MUST preserve the minimal syntactic dependency closure defined as the least set containing the directly referenced objects and closed under explicit reference relations required for interpretation (for example: produced-by, uses, evidence-references, and term references).

- The dependency closure does not require traversal of inferred entailments, active closures, or unrelated provenance chains. It is purely syntactic: include objects explicitly referenced by preserved objects until no new explicit references remain.

- A digest MAY supplement preserved material to support integrity checks, but a digest MUST NOT substitute for the preserved content required for historical interpretation and reconstruction.

Explanatory notes:
- Human-language descriptions or external document pointers MAY be included but MUST NOT be relied upon as the only identity material.
- Implementers MAY include content digests (recommended) but digests are OPTIONAL and cannot replace preserved content (NEW).

12. Consequence Conditions (NEW NORMATIVE DECISIONS)

Purpose: to record the conclusions that the use of the Term was intended to license at mint time.

Normative requirements:
- Consequence Conditions MUST be recorded as explicit Graph Propositions in the identity snapshot (NEW). External references alone are NOT sufficient.

- Minimum fields that MUST be present (NEW decisions unless marked DERIVED):
  - declared-entailments: explicit Proposition(s) that state the intended entailment patterns (e.g., "T(x) ⇒ U(x) under quantifier Q"). These Propositions are identity material and MUST be preserved in-Graph (DERIVED from inclusion of Consequence Conditions in identity; recording is NEW requirement).
  - quantification-and-modality: if an entailment includes quantification or modality constraints, those constraints MUST be recorded as part of the entailment Proposition (DERIVED when applicable from RFC‑0001 A6; inclusion is required when applicable).
  - consequence-preconditions: explicit Proposition(s) enumerating preconditions under which the entailment holds (NEW). This separate precondition field is a normative choice to ensure clear reconstruction.
  - referenced-terms: any Terms referenced by consequence Propositions MUST be recorded by identity tuple reference (DERIVED).

Preservation and dependency closure (d4):
- Consequence Propositions that reference other Assertion Events or Terms require the same preservation of syntactic dependency closure: every explicitly referenced Assertion Event or Term identity required to interpret the consequence Proposition MUST be preserved in-Graph.

Explanatory notes:
- Consequence Propositions MAY be expressed informally, but implementers SHOULD provide sufficient structure to enable automated reconstruction.

OPEN DESIGN QUESTION: the exact syntactic shape for declared-entailments (free‑form Propositions vs restricted micro‑schema) is OPEN DESIGN QUESTION.

13. Term Minting Pattern (NEW NORMATIVE DECISIONS)

Normative pattern (MUST):
- A minting operation MUST follow this minimal pattern, expressed entirely with RFC‑0001 primitives (no new primitives):
  1. An Act is recorded that lists uses (evidence Assertion Events) and declares the performing Source (the minting agent).
  2. The Act outputs an Assertion Event (the Term declaration) whose Proposition contains or links to the Canonical Term Identity tuple and the full identity snapshot (Grounding and Consequence Conditions as recorded per sections 11–12).
  3. The Assertion Event MUST include produced-by linking to the Act and MUST include a durable link to the authority-assertion for the claimed Minting Authority.

Rationale:
- This pattern reuses RFC‑0001 Act/Assertion primitives (DERIVED) and prescribes a minimal required shape for interop (NEW decision to standardize the template).

14. Delegation (NEW NORMATIVE DECISIONS)

Normative statements:
- RFC‑0002 SHALL permit representation of delegation relationships in the Graph by explicit Assertion Events (NEW).
- A delegation Assertion Event MUST state: delegator (Authority Subject), delegate (Subject performing an Act), scope (what actions are delegated), and effective-validity (time bounds) as Graph Propositions (NEW).
- When a minting Act is performed by a delegate, the minting Assertion Event MUST reference the delegation Assertion Event used to authorize the operation (NEW).

Delegation temporal semantics (d1):
- A delegation Assertion is an immutable historical Assertion Event. If a later Assertion defeats or revokes that delegation, the later Assertion MAY alter the current Belief Status of the delegation but MUST NOT remove the original delegation Assertion, change its Record Time, alter the minting Assertion Event, or mutate the TermIdentity of any Term minted while that delegation was asserted.

- RFC‑0002 distinguishes between the historical fact "delegation was asserted at t" and the current defeasible belief/trust in that delegation. RFC‑0002 DOES NOT define social or legal acceptance policy for reconstructed or contested mints; higher-level governance RFCs MAY do so.

- If a delegation Assertion includes a declared valid-time scope, historical interpretation of mints MUST respect the recorded valid-time bounds rather than rely solely on current Belief Status. For delegation valid-time ranges RFC‑0002 adopts the half-open convention [start, end): start is inclusive and end is exclusive. Open-ended validity is represented as [start, +∞). This convention is representation-independent and applies to delegation valid-time used by RFC‑0002; if RFC‑0001 later provides an alternative convention, RFC‑0002 MUST align with it.

Alternatives and constraints:
- Implementations MAY model delegation as chains of Assertion Events. RFC‑0002 does not mandate a particular naming or encoding for delegation predicates.

15. Vocabulary (DERIVED)

Normative statement (DERIVED):
- Vocabulary SHALL be represented as a derived construct: a set of Terms plus Assertion Events that declare grouping, release metadata, and provenance. RFC‑0002 MUST NOT introduce a new core primitive for Vocabulary (DERIVED from RFC‑0001's omission of OntologyVersion as core).

Identity vs reconciliation (d5 clarification):
- RFC‑0002 distinguishes Identity Comparison from Reconciliation. Identity Comparison answers "Are these recorded Authority Subjects the same recorded Subject?" and operates on recorded Subject identities only. Reconciliation is a separate operation that consumes reconciliation Assertions (see section 8) to present mapped or federated views; reconciliation MUST NOT mutate stored identities.

16. Evolution (DERIVED + normative patterns NEW)

Derived invariants (DERIVED):
- Term identity MUST NOT be retroactively mutated (RFC‑0001 A4). Evolution operations (deprecation, replacement, split, merge) MUST be expressed as new Assertion Events referencing existing Terms (DERIVED).

Normative patterns (NEW):
- RFC‑0002 RECOMMENDS a minimal pattern for replacement: a replacement Assertion Event MUST reference the replaced Term identity tuple, the replacing Term identity tuple, and an explicit Statement of scope/reason (NEW). This recommendation is a normative pattern to encourage interoperable behavior but does not create new primitives.

17. Historical Interpretation (DERIVED + NEW preservation requirement)

Derived theorem (DERIVED):
- Given that Term identity tuple and identity-defining snapshots are preserved, historical Proposition interpretation is reconstructible (logical derivation from RFC‑0001 A4, C8, §8.4.1).

Preservation requirement (NEW normative):
- To guarantee historical reconstruction, RFC‑0002 REQUIRES that the identity snapshot (Grounding and Consequence Conditions) and the minting Assertion Event be preserved in-Graph at minting time (NEW). External-only preservation is insufficient.

18. Derived Properties (DERIVED)

This section lists properties that follow directly from RFC‑0001 and from the pattern mandated here. Each item cites RFC‑0001:
- Term identity immutability: RFC‑0001 A4 ⇒ TermIdentity components recorded at minting MUST remain unchanged.
- Distinct Authorities produce distinct Term identities: RFC‑0001 §8.4.1 ⇒ Authority part of identity.
- Evolution via assertions: RFC‑0001 primitives allow recording deprecation/replacement as new Assertion Events.

19. Invariants (DERIVED)

The system MUST preserve the following invariants (logical consequences of RFC‑0001):
- Append-only recording of minting Assertion Events (RFC‑0001 A4).
- Identity snapshots are authoritative for historical interpretation (RFC‑0001 C8 + §8.4.1).

20. Conformance Requirements (NEW normative choices + DERIVED citations)

Implementations that claim conformance to RFC‑0002 MUST satisfy all the NEW requirements enumerated in sections 8, 11, 12, 13, 14, and 17. Specifically:
- Record the minimal Minting Authority metadata (section 8) in-Graph at minting time (NEW).
- Record grounding and consequence identity snapshots as required (sections 11–12) (NEW).
- Follow the minting pattern described in section 13 (NEW).
- When delegation is used, record delegation Assertion Events as required (section 14) (NEW).

Derived conformance obligations (DERIVED):
- The conformance obligations rely on RFC‑0001 primitives: Act, Assertion Event, produced-by, uses, outputs, and Subject/Source representation (RFC‑0001 §7, §8).

21. Examples (non-normative)

(Non-normative examples illustrating the pattern; terse; not exhaustive.)

22. Rejected Alternatives (non-normative)

- Relying solely on external document references for identity snapshots (rejected because it fails reconstructability guarantees; see RFC‑0001 C8 and preservation requirement in section 17).
- Introducing a new primitive TermMintAct (rejected; existing primitives suffice per RFC‑0001 §7 and §8).

23. Downstream RFC Dependencies (non-normative)

RFC‑0002 intentionally defers: the exact syntactic micro‑schema for declared-entailments and assumptions-list (sections 11 and 12) and formal delegation predicate naming are left for downstream governance RFCs. These are OPEN DESIGN QUESTIONs.


Appendix: OPEN DESIGN QUESTION markers

- Authority provenance record content: OPEN DESIGN QUESTION — the exact minimal set of provenance claims to include for authority distinguishability.
- Declared-entailments syntactic shape: OPEN DESIGN QUESTION — whether RFC‑0002 should mandate a restricted micro‑schema for entailments or allow free-form Propositions.
- Assumptions-list normalization: OPEN DESIGN QUESTION — whether assumptions MUST follow a prescribed shape or may be arbitrary Propositions.

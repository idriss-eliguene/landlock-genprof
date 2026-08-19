# RFC-0001 — Semantic Foundation

## 1. Status

| Field | Value |
|---|---|
| Number | RFC-0001 |
| Title | Semantic Foundation |
| Status | **Normative** |
| Revision | 2 — semantic repair of A4, A6, A7, A8 |
| Category | Semantics — foundational |
| Supersedes | none |
| Superseded by | none |
| Normative dependencies | none |

### 1.1 Revision 2 — scope and governance note

Revision 2 repairs defects established by model-theoretic analysis of
Revision 1, not by review opinion. Revision 1's A7 admitted conforming
Graphs with **no** admissible Belief State, and admitted others with
**several**; D10, D16, D17 and D18 were shown independent of the axioms
they cited, and D7's totality clause was shown underivable from A8.
Revision 2 changes A4, A6, A7 and A8; restates the derived properties and
invariants that depended on them; and withdraws those that remain
underivable. Sections 6–13 are amended only where they restate repaired
axiom content. The ontology of Section 7 is unchanged.

Section 1 of Revision 1 required that a change to Section 14 be published
as a new RFC superseding RFC-0001 in whole. Revision 2 changes Section 14
in place. This is a deliberate departure, recorded here rather than
concealed: the changed axioms were not merely undesirable but were shown
to have no models over Graphs the same document declared conforming, and
an inconsistent root reference cannot be coherently superseded by a
document that must cite it. Every subsequent RFC MUST cite Revision 2.

RFC-0001 is the root normative reference. Every subsequent RFC MUST cite
RFC-0001 and MUST NOT contradict any axiom, invariant, or conformance
requirement stated here. A subsequent RFC MAY narrow a permission granted
here. A subsequent RFC MUST NOT widen a prohibition stated here.

Changes to Section 14 (Normative Axioms) or Section 19 (Rejected
Alternatives) MUST be published as a new RFC that explicitly supersedes
RFC-0001 in whole. Changes to Sections 15–17 MAY be published as
amendments, and each amendment MUST restate the axiom derivation for
every property or invariant it alters. Sections 6–13 MAY be amended only
where the amendment alters no axiom, no invariant, and no conformance
requirement.

---

## 2. Abstract

This document specifies the semantics of the domain: what may be
asserted, what those assertions mean, how they are identified, how they
relate, how belief in them is determined, and where the boundary between
inference and authority lies.

The specification defines six concepts, thirteen primitive relations,
eight normative axioms, twenty derived properties, thirty-four
invariants, and seventeen conformance requirements. Every invariant cites
the axioms that imply it, or, where it follows from a definition rather
than an axiom, the definition. Every derived property cites the axioms from
which it follows, and each derivation has been checked to be valid under
the axioms as stated. Three identifiers (D14, I18, I28) are withdrawn and
retained as reserved, so that identifiers remain stable across revisions.

The central semantic commitment is A7: for every conforming historical
Graph, the Belief State is the information-least labelling satisfying the
local acceptability conditions. That labelling exists, is unique, assigns
exactly one Belief Status to every Assertion Event, and is preserved
under every permitted extension of the Graph. D19 proves it.

This document specifies semantics only. It defines no representation, no
protocol, no storage model, and no interface.

---

## 3. Motivation

A system that recommends restrictions on the behaviour of an observed
Subject MUST be able to answer four questions with a stated warrant:

1. What is claimed, and about which Subject.
2. Who claims it, on what ground, and at what time.
3. What is currently held to be true, and what defeated the rest.
4. Who took responsibility for acting on it.

These questions are not answerable from a record of outputs. They require
that claims, the acts of claiming, the grounds of claiming, and the acts
of committing be separately identified and separately defeasible.

This document specifies the minimum semantic structure under which all
four questions are answerable for every claim the system holds, at every
past instant, without exception.

---

## 4. Scope

RFC-0001 specifies:

- the concepts that constitute the domain;
- the identity conditions of every concept;
- the primitive relations and their formal properties;
- the modal stratification of claims;
- the conditions under which a claim is held, defeated, or undecided;
- the boundary between inference and authority;
- the provenance obligations attached to every claim;
- the axioms, invariants, and conformance requirements binding on all
  subsequent RFCs.

---

## 5. Non-goals

RFC-0001 does not specify, and subsequent RFCs MUST NOT treat it as
specifying:

- any representation, encoding, serialisation, or schema;
- any storage, indexing, retrieval, or transport mechanism;
- any interface, protocol, command, or naming convention;
- any evaluation strategy, algorithm, or complexity bound;
- any concrete vocabulary of Terms;
- any confidence arithmetic, scoring function, or statistical model;
- any enforcement mechanism, or any property of any enforcement
  mechanism;
- any policy about which Sources are trustworthy.

Confidence arithmetic and vocabulary content are deferred to subsequent
RFCs (Section 20). Their absence here is deliberate and MUST NOT be
resolved by inference from this document.

---

## 6. Terminology

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL
NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and
**OPTIONAL** in this document are to be interpreted as described in
RFC 2119.

Each term below is defined exactly once. Every later section uses these
terms with exactly these meanings and introduces no others.

### 6.1 Entities

**Subject** — a referent: an entity in the world, a Term, or an element
of the Graph. Being a Subject is a role a thing plays when something is
claimed about it. It is not membership in the Graph, and it does not
exclude membership in the Graph.

**Designator** — an opaque identifier assigned to a Subject at
declaration, distinguishing it from every other Subject, including any
other Subject sharing its Continuity Criterion.

**Phase** — a maximal interval over which a Subject's Continuity
Criterion is claimed to hold. A Subject is partitioned into Phases.

**Continuity Criterion** — the declared condition under which a Subject
is claimed to persist as the same Subject. A Continuity Criterion is
declared, never intrinsic.

**Term** — a predicate or a sort. (That a Term is a Subject is stated at
7.7; it is not part of this definition.)

**Proposition** — a truth-apt claim, individuated by its truth
conditions.

**Assertion Event** — an act of committing to one Proposition, by one
Source, on one ground, at one Record Time.

**Act** — a bounded occurrence, performed by one Source, over one
interval, which consumes zero or more Assertion Events and produces zero
or more Assertion Events.

**Source** — the agent that performs an Act or a Commitment.

**Commitment** — a binding decision by an authority over a Frozen
Target.

### 6.2 Components

**Modality** — the register of a Proposition. Exactly one of Actual,
Dispositional, or Deontic.

**Actual** — the Modality of a claim that a state of affairs obtained.

**Dispositional** — the Modality of a claim that a Subject requires,
or would exhibit, a capability.

**Deontic** — the Modality of a claim that a Subject is permitted or
forbidden something.

**Valid Time** — the interval over which a Proposition claims its
content holds.

**Quantification** — the extent of a Proposition over the instances of
its Subject Phase. Exactly one of *this-instance* or *all-instances*.

**Argument** — a component of a Proposition supplied to its Term,
positionally, and conforming to that Term's declared Argument Sorts.

**Arity** — the number of Arguments a Term takes.

**Argument Sort** — the declared class of admissible values at one
Argument position of a Term.

**Record Time** — the instant at which an Assertion Event entered the
Graph.

**Act Kind** — exactly one of Contact, Inference, Testimony, or
Authority.

**Contact** — the Act Kind of an Act claiming causal coupling to its
Subject.

**Inference** — the Act Kind of an Act that consumes Assertion Events.

**Testimony** — the Act Kind of an Act grounded in a Source's report
rather than in coupling or derivation.

**Authority** — the Act Kind of an Act grounded in a Source's standing
to stipulate.

**Warrant** — the Act or Commitment that produced an Assertion Event.
Where the producer is an Act, the Warrant additionally comprises that
Act's Act Kind; where the producer is a Commitment, it does not. A
Warrant is not an entity.

**Minting Authority** — the Source that introduced a Term.

**Grounding Condition** — the set of producers that MAY produce an
Assertion Event whose Proposition uses a given Term. Each member is
either an Act Kind or Commitment.

**Consequence Condition** — the Subsumption and Incompatibility
relations asserted of a given Term.

**Vocabulary** — a set of Terms sharing a Minting Authority.

**Frozen Target** — an immutable, identified set of Assertion Events
bound by a Commitment.

**Decision Time** — the instant at which a Commitment was decided.

**Effective Interval** — the interval over which a Commitment declares
itself in force.

**Jurisdiction** — the declared extent of a Commitment's authority.

**Enactment** — the effect of a Commitment outside the Graph. The Graph
holds no representation of an Enactment; it holds only Commitments and
Propositions.

### 6.3 Relations

**states** — the total, single-valued relation from an Assertion Event
to the Proposition it commits to.

**produced-by** — the total, single-valued relation from an Assertion
Event to the Act or Commitment that produced it.

**performs** — the total, single-valued relation from an Act or a
Commitment to its Source.

**about** — the total, single-valued relation from a Proposition to a
Phase.

**modality** — the total, single-valued relation from a Proposition to
its Modality.

**phase-of** — the total, single-valued relation from a Phase to its
Subject.

**uses** — the relation from an Act to the Assertion Events it consumes.
It MAY be empty.

**outputs** — the relation from an Act or Commitment to the Assertion
Events it produces. It MAY be empty for an Act. It is the converse of
*produced-by*.

**defeats** — the relation by which one Assertion Event undercuts
another Assertion Event or an Act. It attacks a Warrant, never a
Proposition.

**subsumes** (⊑) — the relation asserted between two Terms to claim that,
in every possible world, a Proposition using the first Term is true only
if the otherwise identical Proposition using the second Term is true.

**incompatible** (⊥) — the relation asserted between two Terms to claim
that no possible world makes two otherwise identical Propositions, one
using each Term, both true.

Both are **claims**, not facts the Graph has access to. A1 makes every
instance of either the content of an Assertion Event, and therefore
sourced, warranted, and defeasible (13.3, I22). What the clauses above
state is what a Source asserts when it asserts one of these relations;
they do not fix the extension of the relation independently of assertion,
and a conforming system MUST NOT treat either relation as holding except
where it has been asserted. Which asserted instances bear on Entailment
and Rebuttal is settled by activation (6.4, A6).

**supersedes** — the relation by which one Commitment replaces another
over the same Frozen Target.

**binds** — the relation from a Commitment to its Frozen Target.

### 6.4 Derived notions

**Support** — the relation holding between two Assertion Events when the
first is in the *uses* set of an Act producing the second.

**Premise Set** — for an Assertion Event *e*, written prem(*e*): the
*uses* set of the Act that produced *e*, where the producer is an Act;
the empty set, where the producer is a Commitment. *produced-by* is
single-valued (A2), so prem(*e*) is well defined for every Assertion
Event.

**Strict Ancestry** — the transitive closure of Support. It is a
relation on Assertion Events, and it is irreflexive (D5).

**Ancestry** — for an Assertion Event *e*, the least **set** containing
*e*; every Assertion Event standing in Strict Ancestry to *e*; every Act
or Commitment that produced a member; and every Source that performs such
an Act or Commitment. Ancestry is a set-valued notion over a mixed
domain, and *e* ∈ Ancestry(*e*). It is not the Strict Ancestry relation
and MUST NOT be treated as one. Where a document requires an irreflexive
ordering of Assertion Events by derivation, that notion is Strict
Ancestry.

**Closure** — for a relation, the least relation containing it and closed
under the rules that generate it.

**Alignment** — agreement of two Propositions in Phase, Modality,
Quantification, and overlap in Valid Time.

**Belief Status** — exactly one of *in*, *out*, or *undecided*.

**Labelling** — a total function from Assertion Events to Belief
Statuses.

**Information Order** (⊑<sub>i</sub>) — the partial order on Labellings
under which *L* ⊑<sub>i</sub> *L′* holds when every Assertion Event that
*L* labels *in* is labelled *in* by *L′*, and every Assertion Event that
*L* labels *out* is labelled *out* by *L′*. *Undecided* is the least
element at every Assertion Event. The Information Order records how much
has been settled; it is not an order of preference, plausibility, or
priority, and MUST NOT be read as one.

**Ground Set** — for an Attack, the set of Assertion Events on which that
Attack jointly depends: for a Defeat, the single Assertion Event stating
the *defeats* instance; for a Rebuttal, the Assertion Events stating
every Subsumption and Incompatibility in one derivation of the
Incompatibility under 9.3.4. A Ground Set is conjunctive, and A1 requires
every such instance to be the content of an Assertion Event, so a Ground
Set is never empty.

**Attack** — the derived notion unifying the two ways one Assertion Event
bears against another. An Attack on *e* consists of an attacking
Assertion Event and a Ground Set. There is one Attack on *e* for each
Assertion Event stating a *defeats* instance directed at *e*, and one for
each pairing of an Assertion Event whose Proposition Rebuts the
Proposition of *e* with one derivation of the Incompatibility that
generates that Rebuttal. Where an instance is stated several times over,
or a derived Incompatibility has several derivations, there are several
Attacks, one per ground; the disjunction across them is supplied by A7.1
and A7.2 and is not built into the Ground Set. Defeat and Rebuttal differ
in what they attack — a Warrant and a Proposition respectively (9.4.1,
9.8) — and they are unified only for the purpose of labelling.

**Attack Status** — relative to a Labelling *L*, an Attack is:

- **active** where its attacking Assertion Event is *in* under *L* and
  **every** member of its Ground Set is *in* under *L*;
- **void** where its attacking Assertion Event is *out* under *L*, or
  **some** member of its Ground Set is *out* under *L*;
- **potential** otherwise.

No Attack is both active and void: active requires the attacking
Assertion Event *in* and every ground *in*, void requires it *out* or
some ground *out*, and no Assertion Event is both.

**Active Semantic View** — relative to a Labelling *L*, the sub-relations
of *subsumes*, *incompatible*, and Independence whose asserting Assertion
Events are *in* under *L*, together with everything those generate under
A6. It is a derived evaluation over the Graph, never a stored structure
and never an entity. The Graph retains every asserted instance whether or
not that instance is active (A4, I21).

**Active Subsumption** — an instance of *subsumes* is active relative to
*L* where some Assertion Event stating it is *in* under *L*; void where
every Assertion Event stating it is *out*; potential otherwise. Active
Incompatibility and Active Independence are defined in the same way.

**Entailment** (⊨) — the relation between Propositions generated by
Active Subsumption, Quantification, and Valid Time, as specified in A6.

**Rebuttal** — the symmetric conflict generated between two Propositions
whose Terms are Actively Incompatible and which satisfy Alignment.

**Belief State** — the Labelling determined by A7: the ⊑<sub>i</sub>-least
Labelling satisfying A7's acceptability conditions over a given
historical Graph. Exactly one such Labelling exists for every conforming
historical Graph (D19). A Belief State is derived from the Graph and is
never an entity.

**Corroboration** — the relation holding between two Assertion Events
stating the same Proposition, produced by distinct Acts, whose Ancestries
are disjoint with respect to a stated Failure Mode.

**Independence** — disjointness of two Ancestries with respect to a
stated Failure Mode.

**Failure Mode** — the stated condition relative to which Independence
is claimed.

**Blast Radius** — the set of elements whose Belief Status, or whose
participation in the Corroboration relation, may change when a given
element is defeated.

**Conservative Extension** — an addition to a Vocabulary that changes no
Entailment among pre-existing Terms.

**Graph** — the totality of Propositions, Assertion Events, Acts,
Sources, Commitments, and the relations among them. Subjects are the
referents of Propositions; a Subject is a member of the Graph if and only
if it is itself one of the foregoing.

---

## 7. Core Semantic Model

7.1 The Graph consists of exactly six concepts: Subject, Proposition,
Assertion Event, Act, Source, and Commitment. A conforming system MUST
NOT introduce a seventh.

7.2 The Graph MUST NOT hold the state of a Subject. Every representation
of a Subject's state MUST be a Proposition. This holds whether or not the
Subject is itself a member of the Graph.

7.3 A Proposition is content. It carries no Source, no Record Time, no
Warrant, and no Belief Status.

7.4 An Assertion Event is an act. It carries exactly one Proposition,
exactly one Warrant, and exactly one Record Time.

7.5 An Act grounds Assertion Events. A Commitment grounds Deontic
Assertion Events and no others.

7.6 A Source performs Acts and Commitments alike, by the single relation
*performs*. Trust attaches to a Source, never to an Act, never to a
Commitment, and never to a Proposition.

7.7 Terms are Subjects. Consequently, claims about Terms are Propositions
and are subject to every rule governing Propositions, including
defeasibility and provenance.

7.8 Elements of the Graph are Subjects. Consequently, claims about
Assertion Events, Acts, Sources, and Commitments are Propositions.

---

## 8. Identity Semantics

### 8.1 Proposition

8.1.1 A Proposition is identified by exactly six components:

```
⟨ Phase , Modality , Term , Arguments , Valid Time , Quantification ⟩
```

8.1.2 Two Propositions are identical if and only if all six components
are equal.

8.1.3 A Proposition MUST NOT be identified by, and its identity MUST NOT
depend on: its Source, its Record Time, its Warrant, its Belief Status,
the Assertion Events that state it, or the premises of any Act that
produced it.

8.1.4 Entailment MUST NOT induce identity. Two mutually entailing
Propositions are distinct Propositions.

### 8.2 Assertion Event

8.2.1 An Assertion Event is identified by:

```
⟨ Act-or-Commitment , Proposition ⟩
```

Source is not an identity component: it is determined by the Act or
Commitment through *performs*, which is single-valued (A2). Record Time
is not an identity component: it is an attribute of the Assertion Event
(7.4), constrained by A3.

8.2.2 Two Assertion Events are identical if and only if they were
produced by the same Act or the same Commitment and state the same
Proposition. Identity of content is not identity of act.

8.2.3 Where a Source does not expose the identity of its own acts, two
Assertion Events with identical content MUST be treated as distinct and
MUST be granted no Corroboration.

### 8.3 Act

8.3.1 An Act is identified by:

```
⟨ Source , Act Kind , the Act's interval (6.1) , uses set ⟩
```

8.3.2 Where an Act's *uses* set cannot be stated, the Act MUST be
recorded as unreproducible. Its identity MUST NOT be presented as
determinate.

### 8.4 Term

8.4.1 A Term is identified by:

```
⟨ Minting Authority , Arity , Argument Sorts ,
  Grounding Conditions as of minting ,
  Consequence Conditions as of minting ⟩
```

8.4.2 A Term MUST NOT be identified by its name.

8.4.3 A Conservative Extension MUST NOT mint a new Term. Asserting a
Subsumption or Incompatibility that alters no Entailment among
pre-existing Terms is a Conservative Extension and MUST NOT change the
identity of any pre-existing Term.

8.4.4 A change to a Term's Grounding Conditions, and any assertion that
alters Entailment among pre-existing Terms, is not a Conservative
Extension. Such a change MUST mint a new Term. The prior Term MUST remain
unchanged.

### 8.5 Subject

8.5.1 A Subject is identified by:

```
⟨ Designator , Continuity Criterion ⟩
```

Two Subjects sharing a Continuity Criterion but bearing distinct
Designators are distinct Subjects.

8.5.2 A Subject MUST NOT be identified by structural equality of state.
Two Subjects with identical state are distinct Subjects.

8.5.3 A violation of a Continuity Criterion MUST close the current Phase
and open a new one. It MUST NOT alter the Subject's identity.

8.5.4 A Proposition about one Phase MUST NOT be treated as a Proposition
about another Phase. Transfer between Phases MUST be effected by an
Inference Act consuming an explicit Assertion Event stating the
exchangeability premise.

### 8.6 Commitment

8.6.1 A Commitment is identified by:

```
⟨ Source , Frozen Target , Decision Time ⟩
```

8.6.2 A Commitment MUST record the Record Time as of which its Belief
State was read. By D18 that instant determines a unique Belief State, so
no further component is required to fix which Belief State the Commitment
was decided under. D18 rests on A7's uniqueness result (D19) and on the
append-only extension discipline of A4; where either is weakened, this
clause fails with them.

### 8.7 Deduplication

8.7.1 A conforming system MUST NOT merge nodes.

8.7.2 A conforming system MUST NOT express deduplication as a mutation of
the Graph. Two Assertion Events stating the same Proposition and not
identical under 8.2.2 MUST both remain present.

8.7.3 Whether two such Assertion Events stand in the Corroboration
relation is determined by 9.9. No further deduplication rule is specified
by this document.

---

## 9. Relation Semantics

### 9.1 Primitive relations

The primitive relations are exactly: *states*, *produced-by*, *performs*,
*about*, *modality*, *phase-of*, *uses*, *outputs*, *defeats*,
*subsumes*, *incompatible*, *supersedes*, *binds*.

A conforming system MUST NOT introduce a primitive relation not listed
above. Support, Premise Set, Strict Ancestry, Ancestry, Attack, Attack
Status, Active Semantic View, Active Subsumption, Active Incompatibility,
Active Independence, Entailment, Rebuttal, Belief State, Corroboration,
and Blast Radius are derived and MUST be computed from the primitives,
never asserted directly. In particular, no conforming system asserts an
Attack, an Attack Status, an activation, or a Belief Status: each is read
off the Graph and the Labelling A7 determines.

### 9.2 Subsumption (⊑)

9.2.1 *subsumes* as asserted is an arbitrary binary relation on Terms. It
is **not** required to be reflexive or transitive, and a conforming
system MUST NOT require a Source to assert *t* ⊑ *t*. The **Closure** of
the Active Subsumption relation is reflexive and transitive, and it is
the Closure, not the asserted relation, that generates Entailment (A6).

9.2.1.1 This distinction is load-bearing. A1 makes every asserted ⊑
instance an Assertion Event, and therefore defeasible; a defeasible
relation cannot also be required to be reflexive and transitive, because
defeating one instance would violate the requirement. Reflexivity and
transitivity are properties of the derived Closure, which is recomputed
from whatever is active, and are therefore never violated by a defeat.

9.2.2 The Closure of ⊑ is **not required** to be antisymmetric. Two Terms
with distinct Minting Authorities MAY mutually subsume and MUST remain
distinct.

9.2.3 The Closure of ⊑ is therefore a **preorder**. A conforming system
MUST NOT treat it as a partial order and MUST NOT quotient the Graph by
mutual subsumption.

9.2.4 ⊑ is **not required** to be a lattice. A conforming system MUST NOT
assume the existence of a common supertype for any two Terms.

9.2.5 A Subsumption asserted between Terms of distinct Minting
Authorities has no property distinguishing it from any other
Subsumption. It MUST NOT be modelled as a separate relation.

### 9.3 Incompatibility (⊥)

9.3.1 The Closure of ⊥ is symmetric. *incompatible* as asserted need not
be, and a conforming system MUST NOT require a Source to assert an
Incompatibility in both directions.

9.3.2 The Closure of ⊥ is irreflexive. An assertion of ⊥(t,t) MUST be
rejected, and so MUST any assertion whose acceptance would make the
Closure of ⊥ over the asserted relations reflexive at any Term (C4).

9.3.2.1 The second clause is required because 9.3.4 can derive ⊥(t,t)
from assertions none of which is reflexive: where t₁ ⊑ t₂ and t₂ ⊥ t₁,
downward closure yields t₁ ⊥ t₁. Irreflexivity is enforced against the
Closure of the asserted relations, which is the widest it can become; the
Closure of the Active relations is a subset of it and is therefore
irreflexive in every Belief State (I31).

9.3.3 ⊥ is **not** transitive.

9.3.4 ⊥ is downward-closed along ⊑: if t₁ ⊑ t₂ and t₂ ⊥ t₃ then t₁ ⊥ t₃.
Applied to the Active Semantic View, a derived Incompatibility is
**active** where every Subsumption and Incompatibility in some derivation
of it is active; **void** where every derivation of it contains a step
that is void; and **potential** otherwise. Each derivation contributes
one Attack, whose Ground Set is that derivation's asserting Assertion
Events (6.4).

9.3.5 ⊥ does not defeat. It constrains the Belief State by generating
Rebuttal under Alignment, and it does so only where it is active (A6,
A7).

### 9.4 Defeat

9.4.1 Defeat is undercutting only. It attacks a Warrant.

9.4.2 Defeat is **not required** to be symmetric, asymmetric,
irreflexive, or transitive. A conforming system MUST NOT assume any of
these properties.

9.4.3 Where *e₁* defeats *e₂* and *e₂* defeats *e₃*, it does not follow
that *e₁* defeats *e₃*. Under A7, *e₁* being *in* renders *e₂* *out*,
which may render *e₃* *in*. This is a consequence of the labelling, not a
relation: a conforming system MUST NOT materialise it (9.1).

9.4.4 Defeat propagates through Acts, never through Propositions.
Premise-invalidity is one of the two acceptability conditions of A7, not
a rule applied after labelling: an Assertion Event is *in* only where
every member of its Premise Set is *in*, and is *out* where any member of
its Premise Set is *out*. A Proposition remains held if any Assertion
Event stating it is *in*.

9.4.4.1 Revision 1 stated propagation as a separate clause that overrode
the labelling. That formulation had no solution over an Act with a
defeated premise and an unattacked output: the *in* condition, which
consulted attackers only, forced the output *in*, and the propagation
clause forced it *out*. The two conditions are now clauses of one
acceptability test and cannot disagree (D19).

9.4.5 Defeat MAY be cyclic. A7 assigns a Belief Status to every Assertion
Event on a cycle, and does so without resolving the cycle by inference
(D10, 11.8).

9.4.6 A *defeats* instance bears on the Belief State only where it is
active. By A1 every *defeats* instance is the content of an Assertion
Event; where every such Assertion Event is *out*, the Attack it grounds
is void and constrains nothing. Defeat is therefore defeasible in the
same way as every other asserted relation (13.3), and by the same
mechanism.

### 9.5 Support

9.5.1 Support is acyclic.

9.5.2 Strict Ancestry is the transitive closure of Support and is a
strict partial order. Ancestry is the set-valued notion of 6.4, contains
its own Assertion Event, and is not an ordering. The two MUST NOT be
conflated: where an irreflexive ordering of Assertion Events by
derivation is required, the notion is Strict Ancestry; where the
provenance of an Assertion Event is required, including the Acts,
Commitments and Sources behind it, the notion is Ancestry.

9.5.3 Support is **not required** to be intransitive. Where an Act *a₁*
uses *e₁* and outputs *e₂*, an Act *a₂* uses *e₂* and outputs *e₃*, and
an Act *a₃* uses *e₁* and outputs *e₃*, Support holds on all three pairs.
A4 permits *a₃* to be added at any time, so intransitivity cannot be
required of a Graph that was conforming before the addition. Revision 1
asserted intransitivity; the assertion followed from no axiom and is
withdrawn.

### 9.6 Supersession

9.6.1 Supersession is irreflexive, transitive, and asymmetric — a strict
partial order. Transitivity is asserted by A8; irreflexivity and
asymmetry follow from the Decision Time ordering of A3 (D7).

9.6.2 Supersession is **not required** to be connex, on a single Frozen
Target or anywhere else. Two Commitments binding the same Frozen Target
and standing in no Supersession relation are **concurrent**. Restricted
to one supersession chain over one Frozen Target, Supersession is a
strict total order; a Frozen Target MAY carry several such chains, and
MAY therefore have several maximal Commitments in force at once.

9.6.2.1 Revision 1 required Supersession to be a strict total order per
Frozen Target. That requirement followed from no axiom — A8 nowhere
excluded two Commitments at one Decision Time, and 8.6.1 makes two
Commitments distinct when they share a Frozen Target and a Decision Time
but differ in Source. It also contradicted 12.7, which requires a
conflict between Commitments to be recorded and forbids resolving it by
inference: a forced total order resolves every same-target conflict by
inference, in the act of imposing the order. The requirement is
withdrawn. Concurrency is now the recorded outcome, and 12.7 governs it.

9.6.3 Supersession applies to Commitments only. A Commitment MUST NOT be
the target of Defeat.

### 9.7 Entailment (⊨)

9.7.1 ⊨ is a preorder. It is not antisymmetric.

9.7.2 ⊨ is generated by exactly three sources and no others:

- **Active Subsumption**: where t₁ ⊑ t₂ is active, the Proposition at t₁
  entails the Proposition at t₂, all other components held equal.
- **Quantification**: *all-instances* entails *this-instance*.
- **Valid Time**, whose direction is determined by Modality (Section
  10.4).

9.7.2.1 Entailment is relative to a Belief State, because Active
Subsumption is. A conforming system MUST state the Record Time at which
an Entailment query is answered, and MUST NOT present an Entailment as
holding independently of the Belief State under which it was computed.
Quantification and Valid Time generate Entailment unconditionally: they
are components of Proposition identity (8.1.1), not asserted relations,
and nothing can defeat them.

9.7.3 ⊨ MUST NOT cross Modality.

9.7.4 Where the Valid Time domain is unbounded or dense, the Closure of ⊨
is infinite (D11). A conforming system MUST NOT materialise the Closure of
⊨ except where it has established that its Valid Time domain is finite. A
conforming system MUST be able to answer ⊨ as a query in all cases.

### 9.8 Rebuttal

9.8.1 Rebuttal holds between two Propositions when their Terms are
**Actively** Incompatible and the Propositions satisfy Alignment.

9.8.2 All four conditions of Alignment are required. Agreement in fewer
than four MUST NOT generate Rebuttal.

9.8.3 An Incompatibility that is void generates no Rebuttal; an
Incompatibility that is potential generates an Attack that is itself
potential, and so may leave the Assertion Events it bears on *undecided*
without labelling any of them *out* (A7). The Incompatibility remains in
the Graph in every case (A4).

### 9.9 Corroboration

9.9.1 Corroboration requires distinct Acts.

9.9.2 Corroboration requires disjoint Ancestries with respect to a
stated Failure Mode. Independence claimed without a stated Failure Mode
MUST be rejected.

9.9.3 Independence is a claim. It MUST be recorded as an Assertion Event
and is defeasible. Two Assertion Events stand in the Corroboration
relation only while the Independence claim is **active**: where every
Assertion Event stating it is *out*, Corroboration does not hold, and
where the claim is potential, Corroboration does not hold either. The
Independence claim remains in the Graph in every case (A4, I21).

9.9.5 Corroboration is therefore relative to a Belief State, as
Entailment is (9.7.2.1). Structural Ancestry does not change when an
Independence claim is defeated: what changes is whether the disjointness
of those Ancestries is currently claimed with active support (D17).

9.9.4 Replay MUST NOT generate Corroboration. Where two Assertion Events
are identical under 8.2.2, there is one Assertion Event, and 9.9.1 is not
satisfied.

---

## 10. Modal Semantics

10.1 Every Proposition has exactly one Modality: Actual, Dispositional,
or Deontic. The three partition the set of Propositions.

10.2 Production is restricted by Modality:

| Producer | MAY produce | MUST NOT produce |
|---|---|---|
| Contact Act | Actual | Dispositional, Deontic |
| Inference Act | Actual, Dispositional | Deontic |
| Testimony Act | Actual, Dispositional | Deontic |
| Authority Act | Actual, Dispositional | Deontic |
| Commitment | Deontic | Actual, Dispositional |

10.3 Entailment is closed within each Modality and empty across
Modalities.

10.4 The direction of Valid Time Entailment is determined by Modality, as
specified in A6:

- **Actual** Propositions are upward-monotone in Valid Time. A claim over
  τ entails the claim over every τ′ ⊇ τ, and MUST NOT be taken to entail
  the claim over any proper subinterval.
- **Dispositional** and **Deontic** Propositions are downward-monotone in
  Valid Time. A claim over τ entails the claim over every τ′ ⊆ τ, and
  MUST NOT be taken to entail the claim over any proper superinterval.

10.5 The transition from Actual to Dispositional MUST be effected by an
Inference Act, and that Act's *uses* set MUST contain an Assertion Event
stating the completeness premise on which the transition depends.

10.6 The transition from Dispositional to Deontic MUST be effected by a
Commitment. No Act of any Kind may effect it.

---

## 11. Inference Model

11.1 An Inference Act consumes Assertion Events and produces Assertion
Events.

11.2 An Inference Act MAY produce zero Assertion Events. An Inference Act
with an empty *outputs* set is admissible and MUST NOT be rejected on
that ground.

11.3 An Inference Act MUST declare its premise set. The declared premise
set is that Act's *uses* set, and MUST comprise an Assertion Event for
every assumption, parameter, threshold, and Vocabulary axiom the Act
declares reliance upon.

11.3.1 Conformance to 11.3 is assessed against the declaration. Whether
an Act relied upon a premise it did not declare is not determinable from
the Graph. This limit is stated, not remedied, and is of the same kind as
that stated at 13.9.

11.5 Every Assertion Event carries exactly one Belief Status, determined
as specified in A7. Exactly one Belief State exists over every conforming
historical Graph (D19).

11.6 A cycle in *defeats* yields Belief Status *undecided* for its
members wherever the conditions of D10 hold: no member has an *out*
member of its Premise Set; every Attack on a member either comes from
within the cycle or is not active; and every member carries at least one
Attack from within the cycle no member of whose Ground Set is *out*.
Where any of those fails, the member is labelled by A7.1 or A7.2 and the
remaining members are labelled on that basis. Two failures in particular:
where a member is attacked from outside the cycle by an Assertion Event
that is *in*, that member is *out*; and where the Ground Set of an Attack
inside the cycle contains an Assertion Event that is *out*, that Attack
is void, and the cycle may resolve entirely. *Undecided* is a Belief
Status, not a concept.

11.6.1 Revision 1 required every member of a *defeats* cycle to be
*undecided* without qualification. That requirement was inconsistent with
A7 over a cycle attacked from outside: A7 labels the attacked member
*out*, and the two demands cannot both be met. The qualification is not a
narrowing of intent but the correction of a statement that had no
solution. The precise condition is D10.

11.7 An Assertion Event is *in* if and only if every Attack on it is void
and every member of its Premise Set is *in*. It is *out* if and only if
some Attack on it is active or some member of its Premise Set is *out*.
It is *undecided* otherwise. The three conditions are evaluated against
one Belief State, and by A7 that Belief State is the ⊑<sub>i</sub>-least
one satisfying them.

11.7.1 Both clauses of 11.7 consult Attacks and Premise Sets together.
Neither overrides the other, and no ordering between them is specified or
required, because none is needed: the conditions are mutually exclusive
by construction (D19).

11.8 A conforming system MUST NOT resolve an *undecided* labelling by
inference. Resolution of *undecided* requires a Commitment. This is why
A7 selects the ⊑<sub>i</sub>-least Labelling: any greater Labelling
settles at least one Assertion Event that the Graph does not settle, and
settling it is exactly the inference 11.8 forbids.

---

## 12. Governance Boundary

12.1 A Commitment is decided by a Source acting under a stated
Jurisdiction.

12.2 A Commitment MUST bind exactly one Frozen Target (I29). The content
of a Frozen Target MUST NOT change after binding (I25).

12.3 A Commitment MUST record the Record Time as of which its Belief
State was read (8.6.2).

12.4 A Commitment MUST NOT be defeated. A Commitment MUST NOT be deleted.
A Commitment MAY be superseded.

12.5 Defeat of any Assertion Event in a Commitment's Frozen Target, or of
any element of that target's Ancestry, MUST NOT alter the Commitment.

12.6 Enactment MUST NOT precede Decision Time. An Effective Interval MAY
begin before Decision Time; where it does, both instants MUST be
recorded.

12.7 Where two Commitments with overlapping Jurisdiction conflict, the
conflict MUST be recorded and MUST NOT be resolved by inference. This
holds whether or not the two bind the same Frozen Target: concurrent
Commitments over one Frozen Target are admissible (9.6.2), and a
conforming system MUST NOT order them by Decision Time, by Source, or by
any other means in order to make the conflict disappear. Resolution
requires a further Commitment that supersedes both.

12.7.1 A conflict between the Deontic Assertion Events two Commitments
produce is expressed as Rebuttal, and A7 labels the conflicting
Assertion Events *undecided* where neither attack is decided. This
records the conflict; it does not resolve it, and it does not defeat
either Commitment (A8, I27).

12.8 A Commitment does not compose. Where a Deontic Proposition bound by
a Commitment entails another Deontic Proposition, the entailment is a
property of the Propositions and MUST NOT be represented as an additional
Commitment.

---

## 13. Provenance Model

13.1 Every Assertion Event MUST have exactly one Warrant.

13.2 Every Act MUST have exactly one Source.

13.3 Every instance of *defeats*, *subsumes*, *incompatible*, and every
Independence claim MUST itself be an Assertion Event, and is therefore
sourced, warranted, and defeasible.

13.4 Record Time MUST be recorded for every Assertion Event and MUST NOT
be revised.

13.5 A Contact Act MUST NOT warrant a Proposition whose Valid Time lies
outside that Act's interval.

13.6 A claim originating outside the Graph MUST be ingested as follows,
and MUST NOT be ingested as a premise-free Assertion Event:

- **13.6.1** A Testimony Act, performed by a local Source, produces an
  Assertion Event whose Proposition is about the foreign Source and
  states that the foreign Source asserted a given content.
- **13.6.2** Where the object-level Proposition is to be held locally, it
  MUST be produced by an Inference Act whose *uses* set contains both the
  Assertion Event of 13.6.1 and an Assertion Event stating the trust
  premise.
- **13.6.3** Where the foreign content uses a foreign Vocabulary, that
  Inference Act's *uses* set MUST additionally contain an Assertion Event
  stating the Subsumption between the foreign Term and a local Term.

13.7 Two Assertion Events ingested from the same foreign Source under
13.6 MUST be treated as dependent for the purpose of Corroboration unless
an Independence claim states otherwise.

13.8 Identification of a foreign Proposition with a local Proposition
MUST be effected by an Inference Act. It MUST NOT be performed
automatically on the basis of matching content.

13.9 The identity of a Source is declared. A conforming system MUST NOT
present Source identity as verified by the Graph.

---

## 14. Normative Axioms

**A1 — Reflection.**
Every relation instance other than *states*, *produced-by*, *performs*,
*about*, *modality*, *phase-of*, *uses*, *outputs*, *binds*, and
*supersedes* MUST be the content of an Assertion Event, and therefore
MUST have a Source, a Warrant, and a defeat condition.

**A2 — Functionality.**
*states*, *produced-by*, *performs*, *about*, *modality*, and *phase-of*
are total and single-valued. *performs* is total over Acts and over
Commitments alike. *binds* is total and single-valued over Commitments.

**A3 — Temporal strictness.**
Record Time and Decision Time are drawn from one strict total order. For
every Act *a*, every *e* ∈ uses(*a*), and every *e′* ∈ outputs(*a*):
RecordTime(*e*) < RecordTime(*e′*). Where a Commitment *c₁* supersedes a
Commitment *c₂*, DecisionTime(*c₂*) < DecisionTime(*c₁*). Record Time and
Decision Time MUST NOT be revised.

**A4 — Append-only extension.**
Nodes and relation instances MAY be added. They MUST NOT be deleted,
merged, or mutated.

A Graph *G′* is a **conforming extension** of a Graph *G* exactly where
all four of the following hold:

- **A4.1 Containment.** Every node and every relation instance of *G* is
  a node or relation instance of *G′*, and the value of every
  single-valued relation of A2 is unchanged at every node of *G*.
- **A4.2 Temporal append.** Every Assertion Event in *G′* and not in *G*
  has a Record Time strictly later than the Record Time of every
  Assertion Event in *G*; every Commitment in *G′* and not in *G* has a
  Decision Time strictly later than the Decision Time of every Commitment
  in *G*. Relation instances added between nodes already present in *G*
  are added by way of the Assertion Events that state them (A1), and are
  therefore governed by the same clause.
- **A4.3 Invariance.** *G′* satisfies every invariant of Section 16.
- **A4.4 No revision.** No Record Time and no Decision Time differs
  between *G* and *G′* at any node present in both.

Only a conforming extension is permitted. A4.2 is what makes the Graph a
history rather than a mutable structure: without it, an addition could
introduce an Assertion Event bearing a Record Time already passed, the
historical Graph *G<sub>T</sub>* would change after the fact, and the
Belief State as of *T* would not be reconstructible (C8, D18).

A conforming extension of a conforming Graph is a conforming Graph, and
the Belief State exists and is unique over it (D20).

**A5 — Modal stratification.**
Propositions are partitioned by Modality. Entailment is closed within a
Modality. Contact Acts produce only Actual Propositions, and only
Propositions whose Valid Time lies within the producing Act's interval.
Inference, Testimony, and Authority Acts produce only Actual or
Dispositional Propositions. Deontic Propositions are produced only by
Commitments.

**A6 — Semantic generation.**
*subsumes* and *incompatible* as asserted are arbitrary binary relations
on Terms, subject only to the following. They hold only between Terms of
equal Arity whose Argument Sorts are pairwise compatible. The Closure of
the asserted *incompatible* relation under symmetry and under downward
closure along the Closure of asserted *subsumes* is irreflexive. Every
Term has at least one Grounding Condition.

Relative to a Belief State, the **Active Semantic View** comprises the
Active Subsumption and Active Incompatibility instances (6.4). The
Closure of Active Subsumption is reflexive and transitive; the Closure of
Active Incompatibility is symmetric, irreflexive, and downward-closed
along the Closure of Active Subsumption.

Entailment is generated by the Closure of Active Subsumption, by
Quantification, and by Valid Time monotonicity — upward for Actual
Propositions, downward for Dispositional and Deontic Propositions — and
by nothing else. Rebuttal is generated by the Closure of Active
Incompatibility under Alignment, and by nothing else.

Reflexivity, transitivity, symmetry and downward closure are properties
of the derived Closures and are recomputed from whatever is active. They
are never properties of the asserted relations, which A1 makes
defeasible; requiring them of a defeasible relation would make the defeat
of a single instance a violation of the axiom.

**A7 — Defeasible labelling.**

A Labelling *L* over a historical Graph *G<sub>T</sub>* is **acceptable**
exactly where, for every Assertion Event *e*, all three of the following
hold under *L*:

- **A7.1** *L*(*e*) = *out* if and only if some Attack on *e* is active
  under *L*, or some member of prem(*e*) is *out* under *L*.
- **A7.2** *L*(*e*) = *in* if and only if every Attack on *e* is void
  under *L*, and every member of prem(*e*) is *in* under *L*.
- **A7.3** *L*(*e*) = *undecided* in every other case.

The **Belief State** of *G<sub>T</sub>* is the ⊑<sub>i</sub>-least
acceptable Labelling over *G<sub>T</sub>*. It exists, is unique, and
assigns exactly one Belief Status to every Assertion Event (D19).

A Proposition is held if and only if some Assertion Event stating it is
*in* under the Belief State.

*Notes, normative.*

- Attack, Attack Status, and prem are the derived notions of 6.4. An
  Attack is active only where both its attacking Assertion Event and its
  Ground Set support it, so a *defeats* instance or an Incompatibility
  whose every asserting Assertion Event is *out* constrains nothing
  (9.4.6, 9.8.3).
- A7.1 and A7.2 are jointly exhaustive of the definite cases and mutually
  exclusive: no Attack is both active and void, and no Assertion Event is
  both *in* and *out*. Premise-invalidity and attack are conditions of
  one test, not two tests applied in sequence.
- A conforming system MUST NOT select an acceptable Labelling other than
  the ⊑<sub>i</sub>-least one. Several may exist — a two-member *defeats*
  cycle admits three — and every one other than the least settles some
  Assertion Event that the Graph leaves unsettled. Selecting it would
  resolve an *undecided* labelling by inference, which 11.8 forbids.
- A7 specifies no evaluation strategy and no order of evaluation, and
  none is required: the ⊑<sub>i</sub>-least acceptable Labelling is
  characterised by the conditions above and not by any procedure for
  finding it (Section 5). Any procedure that yields an acceptable
  Labelling ⊑<sub>i</sub>-below every acceptable Labelling yields this
  one.

**A8 — Authority closure.**
Commitments are immune to Defeat and are ordered only by Supersession.
Supersession is transitive. Supersession is not required to be connex:
two Commitments binding one Frozen Target and standing in no Supersession
relation are concurrent, and no axiom orders them. Every Commitment binds
exactly one Frozen Target and records the Record Time as of which its
Belief State was read. An Enactment does not precede the Decision Time of
the Commitment whose effect it is.

Transitivity is asserted here because it does not follow from A3. A3
constrains Supersession only by requiring the superseded Commitment to
carry the earlier Decision Time, which yields irreflexivity and
asymmetry but permits a Supersession relation containing ⟨*c₁*,*c₂*⟩ and
⟨*c₂*,*c₃*⟩ and not ⟨*c₁*,*c₃*⟩.

---

## 15. Derived Properties

Each property below follows from the cited axioms and MUST NOT be
independently postulated.

| ID | Property | Implied by |
|---|---|---|
| **D1** | The Closure of Active Subsumption is a preorder and is not required to be antisymmetric: Term identity includes Minting Authority (8.4.1), so two mutually subsuming Terms with distinct Minting Authorities are distinct, and A4 forbids merging them. Semantic equivalence therefore never merges Terms. Asserted *subsumes* is required to be neither reflexive nor transitive (9.2.1); the preorder properties belong to the Closure and are recomputed from whatever is active, which is what keeps them compatible with A1's making every asserted instance defeasible. | A4, A6 |
| **D2** | ⊑ is not required to be a lattice: A6 asserts no upper-bound property, so a conforming Graph MAY contain two Terms with no common supertype. Generalisation is therefore not guaranteed to be available. | A6 |
| **D3** | ⊥ is not transitive: A6 asserts closure of ⊥ along ⊑ only, and asserts no composition of ⊥ with ⊥. | A6 |
| **D4** | Over the Closures of Active Subsumption and Active Incompatibility, ⊑ ∘ ⊥ = ⊥. Incompatibility is inherited downward along Subsumption. *Proof:* ⊑ ∘ ⊥ ⊆ ⊥ is A6's downward closure; ⊥ ⊆ ⊑ ∘ ⊥ follows from reflexivity of the Closure of Active Subsumption (A6). ∎ The equality does **not** hold of the asserted relations: A6 requires no reflexivity of asserted *subsumes*, so the right-to-left containment fails there. Downward closure alone holds of the asserted relations, and that is what C4 and I31 are discharged against. | A6 |
| **D5** | Support is acyclic. *Proof:* Support relates *e* to *e′* only where *e* ∈ uses(*a*) and *e′* ∈ outputs(*a*) for some Act *a*, whence RecordTime(*e*) < RecordTime(*e′*) (A3). Record Times lie in a strict total order (A3), which is transitive and irreflexive; a cycle would require RecordTime(*e*) < RecordTime(*e*). ∎ | A3 |
| **D6** | **Reinstatement.** Where the Attack of *e₁* on *e₂* is active and the Attack of *e₂* on *e₃* has *e₂* as its attacking Assertion Event, *e₁* *in* forces *e₂* *out* (A7.1), whereupon that Attack on *e₃* is void (6.4). Where it was the only Attack on *e₃* and prem(*e₃*) is *in*, *e₃* is *in* (A7.2). This is a property of the Belief State, not a relation, and MUST NOT be materialised (9.1). | A7 |
| **D7** | Supersession is a strict partial order. *Proof:* transitivity is asserted by A8. A3 requires DecisionTime(*c₂*) < DecisionTime(*c₁*) wherever *c₁* supersedes *c₂*, and Decision Times lie in a strict total order; irreflexivity and asymmetry follow. ∎ Totality per Frozen Target is **not** derivable and is not claimed: A8 admits two Commitments binding one Frozen Target at distinct Decision Times with neither superseding the other, and 8.6.1 makes two Commitments distinct when they share Frozen Target and Decision Time but differ in Source (9.6.2). | A3, A8 |
| **D8** | ⊨ is a preorder on each Modality, with no relation between Modalities. | A5, A6 |
| **D9** | Valid Time Entailment is upward-monotone for Actual Propositions and downward-monotone for Dispositional and Deontic Propositions. | A6 |
| **D10** | **Closed live cycles are undecided.** Let *S* be a non-empty set of Assertion Events such that for every *e* ∈ *S*: **(i)** no member of prem(*e*) is *out*; **(ii)** every Attack on *e* either has its attacking Assertion Event in *S* or is not active; **(iii)** at least one Attack on *e* has its attacking Assertion Event in *S* and has no member of its Ground Set *out*. Then every member of *S* is *undecided*. *Proof:* by induction along the construction of the ⊑<sub>i</sub>-least acceptable Labelling (§15.1), using that the construction is ⊑<sub>i</sub>-increasing, so an Assertion Event that is not *in* at the fixed point is *in* at no stage, and likewise for *out*. At stage 0 no member of *S* is definite. Suppose none is definite at stage *n*, and let *e* ∈ *S*. **A7.1 does not fire.** It requires an active Attack on *e*, or a member of prem(*e*) that is *out*. The latter is excluded by (i). For the former, take any Attack on *e*: either its attacking Assertion Event lies in *S* and is undecided at stage *n*, so the Attack is not active, since activity requires that Assertion Event *in* (6.4); or the Attack is not active by (ii). **A7.2 does not fire.** It requires every Attack on *e* to be void. An Attack is void on either of two grounds: its attacking Assertion Event is *out*, **or** some member of its Ground Set is *out* (6.4). Take the witness Attack of (iii). Its attacking Assertion Event lies in *S*, so it is undecided at stage *n* and not *out*, closing the first ground; and no member of its Ground Set is *out*, closing the second. That Attack is therefore not void, so not every Attack on *e* is void. Hence A7.3 applies and *e* is *undecided* at stage *n*+1. ∎ Clause (iii) is what Revision 2's first statement of this property omitted. An Attack whose attacking Assertion Event lies inside the cycle is nonetheless **void** where its Ground Set has been defeated, and the cycle then resolves rather than remaining undecided (18.2). The conditions are sufficient, not necessary: the necessary and sufficient condition is A7.3 itself, which carries no structural information and is therefore not stated as a property. | A7, D19 |
| **D11** | Where the Valid Time domain is unbounded or dense, the Closure of ⊨ is infinite: D9 generates one Entailment per admissible interval. Nothing in A6 bounds the presentation of that Closure, and Revision 1's claim that it is finitely presented in every case is withdrawn: it followed from no axiom and used an undefined notion. What A6 does supply is that ⊨ is generated by three sources and no others, so membership in ⊨ is decided by inspecting the Active Subsumption Closure, the Quantification, and the Valid Time of two Propositions — which is what C10 requires and all it requires. | A6, D9 |
| **D12** | The Graph is monotone in extension and non-monotone in labelling. Every element changes only by addition (A4); only Belief Status changes by revision (A7). | A4, A7 |
| **D13** | Blast Radius of a Commitment under Defeat is empty: A8 admits no Defeat of a Commitment, so no Defeat alters one. | A8 |
| **D14** | *Withdrawn.* The former claim relating Blast Radius to path redundancy followed from no axiom and used undefined magnitudes. Identifier reserved. | — |
| **D15** | **Modal Separation.** Every path from an Actual Proposition to a Deontic Proposition passes through a Commitment. *Proof:* Entailment is closed within a Modality (A5), so no ⊨ edge crosses. No Act Kind produces Deontic Propositions (A5), so no Act path reaches the Deontic partition. Deontic Propositions are produced only by Commitments (A5). ∎ | A5 |
| **D16** | **Semantic withdrawal without deletion.** Where every Assertion Event stating a Subsumption is *out*, that Subsumption is void, so it is absent from the Active Subsumption Closure, and every Entailment it alone generated is absent from ⊨ (A6, 6.4). Where every Assertion Event stating an Incompatibility is *out*, that Incompatibility is void, the Attacks it grounded are void, and Assertion Events that were *out* on that ground alone become *in* provided their Premise Sets are *in* (A7.2). Independently, every Act whose *uses* set contains an Assertion Event that is *out* has every member of its *outputs* set *out* (A7.1). The asserted instances remain in the Graph throughout (A4, I21): what is withdrawn is their participation in the Active Semantic View, never their presence. *Proof:* the activation conditions of 6.4 are the sole route by which an asserted ⊑ or ⊥ instance enters A6's generation clauses; A7 fixes those conditions; nothing else does. ∎ | A4, A6, A7 |
| **D17** | **Corroboration tracks active support; Ancestry does not move.** Ancestry and Strict Ancestry are computed from *uses* and *outputs* (6.4), which A1 exempts from reflection and which no defeat withdraws; they are therefore invariant under every change of Belief State. Corroboration requires distinct Acts, a stated Failure Mode, and an **active** Independence claim (9.9.2, 9.9.3, C11). Defeating the Independence claim, or defeating a Subsumption on which the Assertion Events' Belief Status depends, therefore changes participation in Corroboration while leaving both Ancestries structurally unchanged. *Proof:* immediate from the activation condition on Independence and the exemption of *uses* and *outputs* under A1. ∎ Revision 1 claimed Ancestry membership itself changed; it does not, and that claim is withdrawn. | A1, A4, A7 |
| **D18** | **A Belief State is uniquely determined by a Record Time *T*.** *Proof:* let *G<sub>T</sub>* be the Graph restricted to elements with Record Time ≤ *T*. (i) *G<sub>T</sub>* is uniquely determined by the history: elements are only added (A4), no addition carries a Record Time at or before any already present (A4.2), and Record Time is never revised (A3, A4.4); so no conforming extension alters *G<sub>T</sub>*. (ii) By D19, exactly one acceptable Labelling of *G<sub>T</sub>* is ⊑<sub>i</sub>-least. (iii) By A7 that Labelling is the Belief State of *G<sub>T</sub>*. Therefore exactly one historical Belief State corresponds to *T*. ∎ Nothing further is claimed: in particular *T* does not determine the Belief State of any Graph other than *G<sub>T</sub>*, and a later addition changes the current Belief State without changing this one (D12). | A3, A4, A7, D19 |
| **D19** | **Existence, uniqueness, and exactly-one status.** Over every conforming historical Graph there is exactly one ⊑<sub>i</sub>-least acceptable Labelling, and it assigns exactly one Belief Status to every Assertion Event. Proof in §15.1. | A1, A2, A7 |
| **D20** | **Extension safety.** Where *G′* is a conforming extension of a conforming Graph *G* (A4.1–A4.4), *G′* is a conforming Graph and has exactly one Belief State. *Proof:* D19 is proved for every conforming Graph without hypothesis on how it arose, and A4.3 requires *G′* to satisfy Section 16; so D19 applies to *G′* directly. ∎ The property Revision 1 lacked was not existence over some Graphs but existence over *all* of them; D19 supplies it, and closure under extension follows rather than needing separate machinery. | A4, D19 |
| **D21** | **Activation is monotone in the Information Order.** Where *L* ⊑<sub>i</sub> *L′*, every Attack active under *L* is active under *L′*, every Attack void under *L* is void under *L′*, and every Subsumption, Incompatibility and Independence active under *L* is active under *L′*. *Proof:* each activation condition is a conjunction and disjunction of conditions of the form "*x* is *in*" and "*x* is *out*", both of which are preserved upward in ⊑<sub>i</sub> by its definition (6.4). ∎ D21 is the lemma on which D19 rests, and it is the reason the Active Semantic View can be belief-sensitive without making the Belief State ill-defined. | A7, 6.4 |

### 15.1 Proof of D19

Write Φ for the operator taking a Labelling *L* over a conforming Graph
to the Labelling Φ(*L*) given by A7.1–A7.3 evaluated against *L*.

**Lemma 1 — Φ is well defined.** A7.1 and A7.2 cannot both hold of the
same Assertion Event under the same *L*. Suppose both. A7.2 gives every
Attack on *e* void. A7.1 gives either some Attack on *e* active — but no
Attack is both active and void (6.4) — or some member of prem(*e*) *out*
while A7.2 gives every member *in*, and *L* is a function. Either branch
contradicts. So exactly one of A7.1, A7.2, A7.3 applies, and Φ(*L*) is a
Labelling. ∎

**Lemma 2 — activation is monotone.** This is D21.

**Lemma 3 — Φ is monotone in ⊑<sub>i</sub>.** Let *L* ⊑<sub>i</sub> *L′*.
If Φ(*L*)(*e*) = *out*, then some Attack on *e* is active under *L*, or
some *p* ∈ prem(*e*) has *L*(*p*) = *out*. In the first case that Attack
is active under *L′* by Lemma 2; in the second *L′*(*p*) = *out* by the
definition of ⊑<sub>i</sub>. Either way Φ(*L′*)(*e*) = *out*. If
Φ(*L*)(*e*) = *in*, then every Attack on *e* is void under *L* and every
member of prem(*e*) is *in* under *L*; by Lemma 2 and ⊑<sub>i</sub> both
persist under *L′*, so Φ(*L′*)(*e*) = *in*. Where Φ(*L*)(*e*) =
*undecided* nothing is required. Hence Φ(*L*) ⊑<sub>i</sub> Φ(*L′*). ∎

**Lemma 4 — acceptable Labellings are exactly the fixed points of Φ.**
A7.1–A7.3 are biconditionals asserting, at every Assertion Event, that
*L*(*e*) is the value Φ assigns from *L*. So *L* is acceptable if and
only if Φ(*L*) = *L*. ∎

**Theorem (D19).** Over every conforming Graph there is exactly one
⊑<sub>i</sub>-least acceptable Labelling.

*Proof.* Let ⊥ be the Labelling assigning *undecided* everywhere; it is
the least element of ⊑<sub>i</sub>. Define L<sub>0</sub> = ⊥,
L<sub>α+1</sub> = Φ(L<sub>α</sub>), and at limit ordinals L<sub>λ</sub> =
the pointwise supremum of the preceding Labellings.

*The sequence increases.* L<sub>0</sub> ⊑<sub>i</sub> L<sub>1</sub>
because ⊥ is least. If L<sub>α</sub> ⊑<sub>i</sub> L<sub>α+1</sub> then
Φ(L<sub>α</sub>) ⊑<sub>i</sub> Φ(L<sub>α+1</sub>) by Lemma 3, so the
property carries to successors; at limits it holds of the supremum. Since
the chain is ⊑<sub>i</sub>-increasing, each Assertion Event is
*undecided* until it takes a definite status and keeps it thereafter, so
each pointwise supremum is a Labelling.

*The sequence stabilises.* Every strict increase makes at least one
further Assertion Event definite, and no Assertion Event is made definite
twice. The number of strict increases is therefore bounded by the number
of Assertion Events; the sequence reaches some L\* with Φ(L\*) = L\*.
Where the Graph is finite with *n* Assertion Events, stabilisation occurs
within *n* + 1 steps.

*Existence.* By Lemma 4, L\* is acceptable.

*Leastness.* Let *M* be any acceptable Labelling. By transfinite
induction L<sub>α</sub> ⊑<sub>i</sub> *M*: the base holds because ⊥ is
least; if L<sub>α</sub> ⊑<sub>i</sub> *M* then L<sub>α+1</sub> =
Φ(L<sub>α</sub>) ⊑<sub>i</sub> Φ(*M*) = *M* by Lemma 3 and Lemma 4; limit
stages hold of the supremum. So L\* ⊑<sub>i</sub> *M*.

*Uniqueness.* ⊑<sub>i</sub> is antisymmetric. Two ⊑<sub>i</sub>-least
acceptable Labellings are each ⊑<sub>i</sub> the other, hence equal.

*Exactly one status.* L\* is a Labelling, that is, a total function into
{*in*, *out*, *undecided*}. ∎

**Corollary — mixed dependency cycles.** The proof assumes nothing about
the shape of the dependency structure. Support is acyclic (D5), but A3
constrains no *defeats* instance temporally, so a cycle may run through
Support edges and Attack edges together — for instance *e₁* in prem(*e₂*),
*e₂* attacking *e₃*, *e₃* attacking *e₁*. Monotonicity of Φ is a
pointwise property of A7.1 and A7.2 and is untouched by such a cycle;
existence and uniqueness follow as above, and the members receive
*undecided* wherever D10's conditions hold of them — which requires not
only that no active Attack reaches the cycle from outside, but that no
Attack within it has been made void through its Ground Set (D10).

**On the proof not used.** Revision 1's derivations of D10 and D18
reasoned from the labelling they were constructing, and a repair by
decomposition into strongly connected components of *defeats* would
inherit the same defect: it presupposes that the components can be
ordered and solved in sequence, which is an evaluation strategy (Section
5 forbids specifying one) and which fails outright once Premise Sets
create dependencies across components. The proof above orders nothing.

### 15.2 Choice of labelling semantics

A7 selects the ⊑<sub>i</sub>-least acceptable Labelling. The alternatives
were assessed against the properties RFC-0001 already required of a
Belief State, and against nothing else.

| Candidate | Existence | Uniqueness | Assessment against RFC-0001 |
|---|---|---|---|
| **⊑<sub>i</sub>-least acceptable Labelling** *(adopted)* | always (D19) | always (D19) | Satisfies 11.8: it settles an Assertion Event only where the Graph settles it, so it never resolves an *undecided* labelling by inference. Satisfies 9.4.5, being total over cyclic *defeats*. Satisfies C8, which presupposes *the* Belief State at a past Record Time. Declarative, so Section 5's prohibition on specifying an evaluation strategy is respected. |
| Every acceptable Labelling, none preferred | always | **no** | Rejected. A two-member *defeats* cycle admits three; C8 and 8.6.2 both use the definite article and require one. |
| Maximal acceptable Labellings | always | **no** | Rejected on uniqueness, and on 11.8: maximising settles Assertion Events the Graph does not settle, which is resolution by inference. |
| Two-valued acceptable Labellings only | **no** | no | Rejected on existence: an odd *defeats* cycle admits none, and 9.4.5 permits such cycles. It also eliminates *undecided*, which 11.6 and 20.1 require to exist. |
| Well-founded semantics for the corresponding rule set | always | always | **Materially identical here.** Well-founded semantics differs from the least-fixed-point reading only where positive dependencies are cyclic; Support is acyclic by D5, so the positive part of the dependency structure is well founded and the two coincide on every conforming Graph. Adopting it would change no Belief State and would import an apparatus that does no work. |
| Least fixed point under three-valued Kleene evaluation | always | always | **Materially identical here**, for the same reason. |

The three deterministic candidates coincide on every conforming Graph,
and the non-deterministic candidates fail a requirement RFC-0001 already
stated. The choice is therefore forced by this document's existing
commitments rather than selected among live options, and A7 states it in
the form that needs no auxiliary apparatus: an acceptability condition
and a selection by Information Order.

---

## 16. Invariants

Each invariant below MUST hold at all times in a conforming system. Each
cites the axioms that imply it, or, where it follows from a definition of
Section 6 or an identity condition of Section 8 rather than from an
axiom, that clause. An invariant citing no axiom is not thereby weaker;
it is derived from a definition, and Revision 1's practice of citing an
axiom that did not imply it is not continued.

### 16.1 Structural

| ID | Invariant | From |
|---|---|---|
| **I1** | Every Assertion Event states exactly one Proposition. | A2 |
| **I2** | Every Assertion Event is produced by exactly one Act or exactly one Commitment. | A2 |
| **I3** | Every Act is performed by exactly one Source. | A2 |
| **I4** | Every Proposition is about exactly one Phase. | A2 |
| **I5** | Every Proposition has exactly one Modality. | A2, A5 |
| **I6** | Every Phase belongs to exactly one Subject. | A2 |
| **I6.1** | Phases of a Subject are non-overlapping and totally ordered. | 6.1, 8.5.3 |

### 16.2 Temporal

| ID | Invariant | From |
|---|---|---|
| **I7** | An Act's outputs have Record Time strictly later than every member of its *uses* set. | A3 |
| **I8** | A Contact Act warrants only Propositions whose Valid Time lies within that Act's interval. | A5 |
| **I9** | Record Time and Decision Time are never revised. | A3 |
| **I10** | An Enactment never precedes the Decision Time of its Commitment. | A8 |

### 16.3 Modal

| ID | Invariant | From |
|---|---|---|
| **I11** | Contact Acts produce only Actual Propositions. | A5 |
| **I12** | Inference, Testimony, and Authority Acts produce only Actual or Dispositional Propositions. | A5 |
| **I13** | Deontic Propositions are produced only by Commitments. | A5 |
| **I14** | Entailment never crosses Modality. | A5 |

### 16.4 Vocabulary

| ID | Invariant | From |
|---|---|---|
| **I15** | Terms are immutable. | A4 |
| **I16** | Term identity includes Minting Authority, and Grounding Conditions and Consequence Conditions as of minting. | A4, A6 |
| **I17** | ⊑ and ⊥ hold only between Terms of equal Arity with pairwise compatible Argument Sorts. | A6 |
| **I18** | *Withdrawn.* ⊥ irreflexivity is asserted directly by A6 and restated here without derivation. The requirement is unchanged and is enforced by C4. | — |
| **I19** | Every Term has at least one Grounding Condition. | A6 |
| **I20** | A Term with no Consequence Condition generates no Entailment and no Rebuttal. | A6 |

### 16.5 Graph integrity

| ID | Invariant | From |
|---|---|---|
| **I21** | No node and no relation instance is deleted, merged, or mutated. | A4 |
| **I22** | Every instance of *defeats*, *subsumes*, *incompatible*, and every Independence claim is an Assertion Event. | A1 |
| **I23** | Support is acyclic, and Strict Ancestry is a strict partial order. | A3 |
| **I24** | Supersession is acyclic and transitive. It is **not** required to be total per Frozen Target; concurrent Commitments over one Frozen Target are admissible. | A3, A8 |

### 16.6 Governance

| ID | Invariant | From |
|---|---|---|
| **I25** | The content of a Frozen Target does not change after binding. | A4, A8 |
| **I26** | Every Commitment records the Record Time as of which its Belief State was read. | A8 |
| **I27** | No Commitment is the target of Defeat. | A8 |
| **I29** | Every Commitment binds exactly one Frozen Target. | A2, A8 |

### 16.7 Epistemic

| ID | Invariant | From |
|---|---|---|
| **I28** | *Withdrawn.* No axiom implies the Corroboration condition; it is a conformance requirement, not a derived invariant. The requirement is unchanged and is stated at 9.9 and enforced by C11. | — |
| **I30** | Every conforming historical Graph has exactly one Belief State, and every Assertion Event in it carries exactly one Belief Status. | A7, D19 |
| **I32** | Every Attack has a non-empty Ground Set: no Attack bears on the Belief State unless some Assertion Event states the *defeats* instance or the Incompatibility that grounds it. | A1 |

### 16.8 Activation and extension

| ID | Invariant | From |
|---|---|---|
| **I31** | The Closure of *incompatible* under symmetry and downward closure along the Closure of *subsumes* is irreflexive, over the asserted relations and therefore over the Active ones. | A6 |
| **I33** | A conforming extension alters no historical Graph *G<sub>T</sub>* and no Belief State of any such Graph. | A3, A4, D18 |
| **I34** | Asserted *subsumes*, *incompatible* and Independence instances remain in the Graph when inactive. Activation changes the Active Semantic View and never the Graph. | A4, D16 |
| **I35** | Every conforming extension of a conforming Graph is a conforming Graph and has exactly one Belief State. | A4, D20 |

---

## 17. Conformance Requirements

A conforming system:

**C1** MUST implement exactly the six concepts of Section 7.1 and MUST
NOT introduce a seventh.

**C2** MUST implement exactly the thirteen primitive relations of Section
9.1 and MUST NOT introduce a fourteenth.

**C3** MUST satisfy every invariant of Section 16 at all times.

**C4** MUST reject, at the point of assertion, any input whose acceptance
would place the Graph in violation of any invariant of Section 16, or of
the irreflexivity required by A6 of the Closure of *incompatible* under
symmetry and downward closure along the Closure of *subsumes*. An input
requesting deletion, merger, or mutation MUST be rejected on the same
ground, as MUST any input that would violate A4.1–A4.4, including one
bearing a Record Time at or before a Record Time already present.

**C4.1** C4 is discharged against the **asserted** relations, not the
Active ones. Rejecting against the Active Semantic View would make
admissibility depend on the current Belief State, and a later defeat
could then leave a previously accepted Graph inadmissible — which A4
forbids, since nothing may be removed to restore it.

**C5** MUST NOT merge nodes under any circumstance, including where two
Assertion Events state the same Proposition and are not identical under
8.2.2.

**C6** MUST require an Inference Act to declare its premise set, MUST
record that set as the Act's *uses* set, and MUST reject an Inference Act
that declares no premise set. Conformance is assessed against the
declaration (11.3.1).

**C7** MUST accept and record an Inference Act whose *outputs* set is
empty.

**C8** MUST be able to reconstruct the Belief State as of any past Record
Time. Exactly one such Belief State exists for each Record Time (D18),
and a conforming system MUST NOT present any other Labelling as that
Belief State.

**C9** MUST be able to state, for any held Proposition, every Assertion
Event stating it, every Warrant of each, and the Ancestry of each.

**C10** MUST be able to answer Entailment as a query, and MUST NOT
materialise the Closure of ⊨ unless it has established that its Valid
Time domain is finite (9.7.4). It MUST state the Record Time at which an
Entailment query was answered, since Entailment is generated by Active
Subsumption and is therefore relative to a Belief State (9.7.2.1).

**C11** MUST NOT place two Assertion Events in the Corroboration relation
unless they were produced by distinct Acts, a Failure Mode is stated, and
an Independence claim relative to that Failure Mode is recorded as an
Assertion Event that is *in*. Where that Assertion Event ceases to be
*in*, the two Assertion Events cease to stand in the Corroboration
relation, and the Independence claim nevertheless remains in the Graph
(I34).

**C12** MUST NOT alter, delete, or defeat a Commitment. It MUST express
reversal as Supersession.

**C13** MUST ingest foreign claims as specified in Section 13.6 and MUST
NOT record a foreign claim as a premise-free Assertion Event.

**C14** MUST assign Belief Status *undecided* wherever A7.3 applies, and
MUST NOT assign *in* or *out* on any ground other than A7.1 and A7.2. In
particular it MUST NOT label a member of a cycle in *defeats* *in* or
*out* except where A7.1 or A7.2 requires it, and MUST NOT label such a
member *undecided* where A7.1 requires *out* (11.6, D10).

**C15** MUST NOT present Source identity, Independence, a Continuity
Criterion, or an Inference Act's declared premise set as verified. Each
is declared, and each is defeasible.

**C16** MUST compute the Belief State as the ⊑<sub>i</sub>-least
acceptable Labelling and MUST NOT present any other acceptable Labelling
as the Belief State. Where a system's evaluation procedure yields an
acceptable Labelling that is not ⊑<sub>i</sub>-least, the system is
non-conforming; the procedure itself is unconstrained (Section 5).

**C17** MUST retain every asserted *defeats*, *subsumes*, *incompatible*
and Independence instance in the Graph when it becomes void or potential,
and MUST NOT express deactivation as removal, tombstoning, or any other
alteration of the Graph. Deactivation is a change in the Belief State,
which is derived; the Graph does not change (A4, I34).

---

## 18. Examples

**Every example in this section is non-normative.** No example creates,
narrows, or widens any requirement. Where an example appears to conflict
with Sections 6–17, those sections govern.

### 18.1 Two Sources, one Proposition *(non-normative)*

Sources `r₁` and `r₂` each perform a Contact Act reporting that Subject
`S`, in Phase `φ₁`, exhibited Term `t` over Arguments `⟨x⟩` during Valid
Time `τ`, with Quantification *this-instance*, Modality Actual.

- Where all six identity components coincide, there is one Proposition
  `p`, two Assertion Events `e₁`, `e₂`, two Acts, and two Warrants
  (8.1.2, 8.2.2).
- Where the two reports differ in any identity component — for instance a
  Valid Time drawn from unsynchronised clocks, or a distinct Designator
  for `S` — they state two Propositions, and no Corroboration arises. An
  Inference Act consuming reconciliation premises may then produce an
  Assertion Event stating that the two are one Proposition; that
  Assertion Event is defeasible like any other.
- Where there is one Proposition, `e₁` and `e₂` stand in the
  Corroboration relation only relative to a stated Failure Mode under
  which their Ancestries are disjoint (9.9.2).

### 18.2 Defeat cycle *(non-normative)*

`e₃` states that the Act producing `e₄` was performed under a degraded
instrument. `e₄` states the same of the Act producing `e₃`.

- Both are labelled *undecided* (D10). Every Attack on each originates in
  the cycle, so no Attack is ever active; and the Ground Set of each —
  the Assertion Event stating the *defeats* instance — is *in*, so no
  Attack is void either. D10's clause (ii) is what blocks A7.1 here, and
  clause (iii) is what blocks A7.2; clause (i) holds vacuously, both
  Premise Sets being empty.
- Neither may be resolved by an Inference Act (11.8).
- Resolution requires a Commitment.

Now suppose instead that `e₆`, itself unattacked, defeats the Assertion
Event stating that `e₃` defeats `e₄` — that is, it attacks the *ground*
of one Attack rather than either party to the cycle.

- That Assertion Event is *out*, so the Attack from `e₃` on `e₄` is void
  (6.4), on the Ground Set branch rather than the attacker branch.
- `e₄` now has every Attack on it void and no *out* premise, so `e₄` is
  *in* (A7.2); whereupon the Attack from `e₄` on `e₃` is active and `e₃`
  is *out* (A7.1).
- D10 does not apply, and must not: clause (iii) fails at `e₄`, which
  carries no Attack from within the cycle with an intact Ground Set. The
  cycle is resolved, and nothing inferred it away — an Assertion Event
  about the warrant of a defeat did.

Now add `e₅`, itself unattacked, stating that the Act producing `e₃` was
performed outside its declared interval.

- `e₅` is *in* (A7.2).
- `e₃` is *out*: the Attack from `e₅` is active (A7.1).
- `e₄` is now *in*: its only Attack, from `e₃`, is void, and its Premise
  Set is *in* (A7.2). The cycle is broken from outside, not resolved by
  inference — nothing inferred that `e₄` was sound; an independent
  Assertion Event settled `e₃`.
- Under Revision 1 this configuration had no admissible labelling at all:
  11.6 required `e₃` *undecided* while A7 required it *out*. This is the
  former MUC-2.

### 18.3 Modal separation *(non-normative)*

A Contact Act produces an Actual Proposition. An Inference Act consuming
it, together with a completeness premise, produces a Dispositional
Proposition. No further Act produces a Deontic Proposition. A Commitment
binds the Frozen Target containing the Dispositional Assertion Event and
produces a Deontic Proposition.

- Every path from the Actual Proposition to the Deontic Proposition
  passes through the Commitment (D15).

### 18.4 Blast Radius *(non-normative)*

A Subsumption `t₁ ⊑ t₂` is defeated, and it is the sole Assertion Event
stating that Subsumption.

- The Assertion Event stating the Subsumption becomes *out*, so the
  Subsumption becomes void and leaves the Active Semantic View. The
  asserted instance remains in the Graph (I34, C17).
- Entailments requiring that Subsumption and having no parallel
  derivation no longer hold (D16).
- Every Act whose *uses* set contains that Assertion Event has every
  member of its *outputs* set labelled *out* (A7.1). A Proposition among
  those outputs remains held if another Assertion Event stating it is
  *in*.
- Rebuttals generated via 9.3.4 through `t₁` are withdrawn; Assertion
  Events previously *out* on that ground alone become *in*, provided
  their Premise Sets are *in* (D16, A7.2).
- Commitments are unaffected (D13).

Consider one of those outputs, `y`, which nothing attacks. Under
Revision 1 the labelling rules required `y` to be *in*, because it had no
attacker, and simultaneously *out*, by premise propagation; no Belief
State existed over this Graph. This is the former MUC-1. Under A7.1 and
A7.2 as one acceptability test, `y` is *out* and nothing requires
otherwise.

### 18.5 Vocabulary revision *(non-normative)*

A Consequence Condition of Term `t` is to be changed in a way that alters
Entailment among pre-existing Terms.

- The change is not a Conservative Extension (8.4.4).
- A new Term `t′` is minted. A Subsumption between `t` and `t′` may be
  asserted, and is itself an Assertion Event (I22).
- Propositions using `t` retain their identity and their truth conditions
  (8.1.2).

---

## 19. Rejected Alternatives

Each alternative below is rejected normatively. A subsequent RFC MUST NOT
reintroduce one without superseding RFC-0001 in whole.

| Alternative | Rejected because |
|---|---|
| A single fused Assertion node carrying content, Warrant, and provenance | Cannot separately defeat one Warrant while recognising that two Assertion Events bear on one Proposition. Cannot express an unasserted Proposition, and therefore cannot state a negation as a Rebuttal target. |
| **Warrant** as an entity | Adds no distinction. Every candidate attribute resolves to the Act Kind, to the already-identified Act or Commitment, or to a Proposition about that Act. |
| **Bridge** as a primitive relation | Identical to Subsumption between Terms of distinct Minting Authorities (9.2.5). Its distinguishing properties are epistemic and attach to Source. |
| **Assumes** as a relation distinct from *uses* | Algebraically indistinguishable: defeat of either premise undercuts identically, and both enter Ancestry. |
| **Classification** as a Vocabulary relation | Classification relates an individual to a sort and is a claim about the world. It is an ordinary Proposition with the Blast Radius of any premise. |
| Identity by content equality, with merging | Converts Corroboration into a single Warrant and destroys the alternative Support paths on which 9.4.4 depends. |
| Identity by logical equivalence | Fuses conclusions with premises and collapses Entailment into identity (8.1.4). |
| A single time axis | Cannot distinguish Valid Time from Record Time, and therefore cannot reconstruct a past Belief State (C8). |
| Absence of evidence treated as evidence of absence | A claim about an Act's output is a Proposition about that Act, not about the Subject. The bridge between them is a premise and MUST appear in a *uses* set (11.3). |
| Vocabulary as infrastructure | Vocabulary axioms generate Entailment. Unattributed Entailment edges make D15 unverifiable. |
| Commitment as an Act Kind | Would place a producer of Deontic Propositions inside the Act partition, making the crossing in D15 derivable by inference. |
| Mutable Defeat overlay | Prevents reconstruction of past Belief States (C8). Defeat is expressed by addition (A4). |
| A single global Vocabulary | Assumes a common upper bound that ⊑ does not provide (D2), and blocks recording of a Proposition pending Vocabulary agreement. |
| Premise propagation as a rule applied after labelling | Has no solution over an Act with a defeated premise and an unattacked output: the labelling makes the output *in*, the propagation rule makes it *out*. This was Revision 1's A7 and it is the reason Revision 1 admitted conforming Graphs with no Belief State (9.4.4.1). |
| Belief Status determined by attackers alone | Same defect from the other side. Premise-invalidity is a ground for *out* and its absence a condition of *in*; both belong to the acceptability test (A7). |
| Selecting any acceptable Labelling, or a maximal one | Not unique, and settles Assertion Events the Graph does not settle. C8 and 8.6.2 require *the* Belief State; 11.8 forbids resolving *undecided* by inference (15.2). |
| Two-valued belief, with *undecided* eliminated | No labelling exists over an odd cycle in *defeats*, which 9.4.5 permits. Also removes the only Belief Status under which a recorded, unresolved conflict can be represented (12.7, 20.1). |
| Requiring cycle members to be *undecided* unconditionally | Inconsistent with A7 wherever a cycle is attacked from outside by an Assertion Event that is *in*. This was Revision 1's 11.6 and D10 (11.6.1). |
| Supersession total per Frozen Target | Follows from no axiom, and resolves every same-target conflict by imposing an order, which 12.7 forbids. Concurrency is recorded instead (9.6.2.1). |
| Requiring ⊑ and ⊥ themselves to be preorders | Incompatible with A1, which makes every instance defeasible: defeating one instance would violate the axiom. The closure properties belong to the derived Active Closures, which are recomputed (9.2.1.1, A6). |
| Deactivating a relation instance by removing it | Violates A4 and destroys reconstruction of past Belief States (C8). Deactivation is a change in the derived Belief State, not in the Graph (C17). |

---

## 20. Future RFC Dependencies

The following are deliberately unspecified here and MUST be specified by
subsequent RFCs. Each MUST cite RFC-0001 and MUST conform to Sections
14–17.

| RFC | Subject | Constraint inherited from RFC-0001 |
|---|---|---|
| RFC-0002 | Vocabulary declaration: Grounding Conditions, Consequence Conditions, Minting Authority, sorts and arity | I15–I20; 8.4; D1, D2 |
| RFC-0003 | Confidence and Corroboration arithmetic | 9.9; C11; D17. MUST NOT place Assertion Events in the Corroboration relation absent a stated Failure Mode |
| RFC-0004 | Independence and Failure Mode taxonomy | 9.9.2, 9.9.3; C11, C15 |
| RFC-0005 | Historical reconstruction of Belief States | A3, A4 including A4.1–A4.4; C8, C16; D18, D19, D20; I26, I30, I33 |
| RFC-0006 | Commitment lifecycle: Jurisdiction, Supersession, conflict recording | A8; 12.1–12.8; I25–I27; I24. MUST NOT order concurrent Commitments over one Frozen Target (9.6.2) |
| RFC-0007 | Federation and foreign claim ingestion | 13.6–13.9; C13 |
| RFC-0008 | Subject Continuity Criteria and Phase transitions | 8.5; C15 |

### 20.1 Concepts deliberately not introduced

The following were required by no axiom and are therefore absent. A
subsequent RFC proposing any of them MUST first demonstrate that the
requirement cannot be met under Sections 14–17.

- **Foreign or opaque assertion as a distinct kind.** Expressible under
  13.6 as a Testimony Act producing a Proposition about the foreign
  Source, followed by an Inference Act consuming a trust premise.
- **Belief State as an entity.** It is the Labelling determined by A7
  and is derived from the Graph.
- **Undecided as a concept.** It is a Belief Status, forced by A7 on a
  cycle in *defeats* satisfying D10 — attacked only from within, with no
  *out* premise and no *out* member in the Ground Set of a cycle Attack.
- **Active Semantic View as an entity.** It is a derived evaluation over
  the Graph relative to a Belief State (6.4). Materialising it as a
  stored structure would create a second thing to keep consistent with
  the Graph, and A4 already forbids the mutation that keeping it
  consistent would require (C17).
- **Attack as a primitive relation.** It is the derived union of an
  asserted *defeats* instance and a Rebuttal generated by an asserted
  Incompatibility (6.4). Asserting it directly would bypass A1 and leave
  an Attack with no Ground Set, hence no defeat condition (I32).
- **Scope or objective as a Proposition component.** Expressible as
  Arguments of a Term (8.1.1).
- **Ontology version as an entity.** Terms are immutable (I15); a version
  is a set of Terms and carries no semantics of its own.

---

*End of RFC-0001.*

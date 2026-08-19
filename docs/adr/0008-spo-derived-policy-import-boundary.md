# ADR-0008: SPO derived-policy import boundary

Status: Accepted

Date: 2026-08-18

Revised: 2026-08-19 — retargeted to the modern SPO API. `SeccompProfile`
became cluster-scoped at v0.9.0, so the original namespace-equality
lineage check and the `<pod>` governed name are both invalid and are
replaced below. The import model — derived-policy snapshot at the artifact
layer, copied into a governed object, digest-bound — is unchanged.

## Context

Security Profiles Operator (SPO) records syscalls with a production eBPF
recorder or an audit-log enricher, merges per-replica recordings, and
generates a `SeccompProfile`. It does this better than this project's own
syscall path: our seccomp data comes from the `advise_seccomp` gadget, is
captured in a single bounded window filtered to one process, and is
already marked `OriginType: "advisory"` in our own provenance model
(`cmd/landlock-genprof/provenance_test.go`, `trace.go:790`) — it was never
direct observation to begin with.

SPO does not record filesystem access as Landlock authority and does not
record network activity at all. So SPO cannot replace our observation; it
can replace one domain of it. That is the D-MIN target: our tracer owns
filesystem and network, SPO owns syscalls and seccomp synthesis, and this
project governs the combined result.

The question this ADR settles is **where** an SPO-generated
`SeccompProfile` may enter, and what it may claim once inside.

The tempting entry point is the evidence layer — turn SPO's output into
`Event`s and let it flow through `TrainingHistory` like everything else.
That is wrong and must be foreclosed explicitly. SPO's generated profile
carries syscall **names only**: no timestamps, no invocation counts, no
per-observation records. Feeding it into a model built on `seenInRuns`
forces a choice between two fabrications — claim `seenInRuns = 1` and
report `low` confidence for syscalls SPO saw on every replica, or claim
`seenInRuns = runsRecorded` and invent occurrence data that was never
captured.

The correct entry point is the **artifact layer**. `proposal.Spec`
already stores each artifact as its full rendered content — a string, not
a reference to a live object (`internal/proposal/types.go:107-116`) — and
`CandidateDigest` already binds `spoSeccompProfile` among its six fields
(`internal/proposal/digest.go:19-33`). An imported profile occupies a
field that already exists and is already digested. No new type system, no
`candidate-v2`, no evidence-model change.

## Decision

Adopt **Option B — import as a derived-policy snapshot.**

An SPO-generated `SeccompProfile` enters landlock-genprof at the artifact
layer as **derived policy**. Its supported enforcement content is copied
into a new, landlock-genprof-owned `SeccompProfile` artifact, carrying
provenance, and that governed artifact — not the SPO source object —
becomes part of the `SecurityProfileProposal`, is bound by the existing
`candidate-v1` `CandidateDigest`, is approved, and is applied under the
ordering and readiness contract of ADR-0007.

The import never touches `internal/evidence`, `internal/history`, or
`confidenceFor()`. Import is a snapshot, never a live reference. Source
selection is explicit and never inferred from cluster state.

## Semantic classes

| Class | Definition | Members |
|---|---|---|
| **Raw observation** | An individual event with time and identity | Our `evidence.Event` (timestamp, syscall/path/port, `provenanceId`). SPO's enricher log stream — node-local, not a consumable API |
| **Aggregated observation** | Events summarized with occurrence structure retained | Our `TrainingHistory` items (`seenInRuns`, `runsRecorded`). SPO's `syscall-coverage` annotation — **replica** coverage within one recording, not occurrence over time |
| **Derived policy** | A rule set produced from observation, occurrences discarded | Our synthesized artifacts. **SPO's generated `SeccompProfile`.** Our own seccomp output, which is `advisory`-origin rather than direct |
| **Authorized policy** | Derived policy a human has bound a decision to | A proposal with `approvalState: Approved`, `approvedCandidateDigest`, `candidate-v1`. SPO has no member of this class |

An SPO `SeccompProfile` is derived policy on entry and authorized policy
only after approval. It is never either observation class.

## Import preconditions

Import MUST fail closed unless all hold:

1. **Kind and apiVersion** — `SeccompProfile`,
   `security-profiles-operator.x-k8s.io/v1`, **cluster-scoped**. v0.2
   targets modern SPO only; `v1beta1` and the namespaced scope it was
   served under through v0.8.4 are out of scope (ADR-0007, "Backend
   readiness").
2. **Not partial** — the profile MUST NOT be marked partial. The exact
   label key and value MUST be read from SPO's own API constants by the
   backend adapter rather than hardcoded here; this ADR names the
   requirement, not the string. For a replicated workload, being
   non-partial means `mergeStrategy: Containers` was used and the
   `ProfileRecording` was deleted, which is what triggers SPO's merge.
3. **Lineage** — see below.
4. **Supported fields only** — see "Unsupported fields".
5. **Non-empty enforcement content** — a profile with no `defaultAction`
   is not a usable candidate.
6. **Explicit source selection** — the operator chose SPO as the seccomp
   source; import never happens implicitly.

## Lineage contract

Accepting a valid `SeccompProfile` belonging to a different workload would
silently authorize unrelated syscalls for the governed pod. Lineage
checking is therefore **REQUIRED FOR v0.2**.

The original form of this check — "the source profile's namespace equals
the target workload's namespace" — is **invalid under the modern API**: a
cluster-scoped `SeccompProfile` has no `metadata.namespace` to compare.
`ProfileRecording`, however, remains **namespaced**, and SPO closes the
gap itself with labels on every generated profile
(`api/profilerecording/v1`):

| Label | Meaning |
|---|---|
| `spo.x-k8s.io/recording-id` | name of the `ProfileRecording` that produced this profile |
| `spo.x-k8s.io/recording-namespace` | namespace of that recording — SPO's own doc comment says it exists to disambiguate cluster-scoped profiles from recordings sharing names across namespaces |
| `spo.x-k8s.io/container-id` | container the profile was recorded from |

That is a better foundation than the name parsing the previous revision
relied on, and it restores the namespace dimension the scope change
removed from the object itself.

**Required (v0.2)** — all read from the source profile's labels, never
parsed from its name:

- `spo.x-k8s.io/recording-namespace` equals the target workload's
  namespace.
- `spo.x-k8s.io/recording-id` equals the `ProfileRecording` the operator
  explicitly named at import.
- `spo.x-k8s.io/container-id` equals the container being governed.
- The operator **names the source explicitly**. Lineage is asserted by the
  operator and verified against the object's labels; it is never inferred
  by scanning the cluster for a profile that looks plausible.

**Lineage strength: STRUCTURAL, and that is the ceiling.** These labels
bind a profile to a recording by *name and namespace*, not by UID or
`ownerReference`. Stronger, UID-bound lineage is not merely absent — it is
**structurally impossible** here: Kubernetes does not support a
cluster-scoped dependent owned by a namespaced owner, so a cluster-scoped
`SeccompProfile` cannot carry an `ownerReference` to a namespaced
`ProfileRecording`. Labels are the strongest relation the API can express,
which is presumably why SPO uses them.

**Consequences of that ceiling, stated rather than hidden.** A principal
able to write labels on a cluster-scoped `SeccompProfile` can forge this
lineage. Deleting and recreating a recording with the same name in the
same namespace produces labels indistinguishable from the original. And
the labels prove the profile was recorded from *a* container of that name
under *that* recording — not that the recording observed the specific pod
instance being governed. Heuristic lineage (searching for a profile that
looks right) remains **forbidden**.

Lineage failure is **fatal**. There is no advisory mode.

## Ownership and governed naming

**Model B — copy semantic content into a new governed object.** Unchanged.

| | Source SPO object | Governed object |
|---|---|---|
| Scope | Cluster | Cluster |
| Name | SPO's own generated name | deterministic, cluster-unique — see below |
| Owner | SPO | landlock-genprof, marked as such |
| Mutated by us | **Never** | Created/updated by the governed apply |
| Enabled | Left as SPO left it (`disabled: true` under `disableProfileAfterRecording`) | Enabled — our schema omits `disabled`, so the copy defaults active |
| Role | Candidate source, read once | Reviewed, digested, approved, applied, enforced |

Rejected as before: **A (govern the live SPO object)** — the approved
thing would stay mutable by SPO and others. **C (enable/disable the same
object)** — mutates SPO-owned lifecycle state and makes our approval
depend on a field we do not own. **D (rename/adopt)** — same objection.
The scope change strengthens rather than weakens this: mutating a
cluster-scoped object we did not create has cluster-wide blast radius.

### Governed profile name — normative

The previous revision named the governed copy `<pod>`. That is **unsafe
now**: `SeccompProfile` is cluster-scoped, so `<pod>` in namespace `a` and
`<pod>` in namespace `b` would be the same object, and two teams could
silently overwrite each other's enforcement.

Naive concatenation does not fix it. `<namespace>-<pod>` maps
(`a-b`, `c`) and (`a`, `b-c`) to the same string; adding `<container>`
only adds another ambiguous boundary. For a cluster-wide name space, an
ambiguous encoding is a cross-namespace authority collision.

The algorithm below is **normative identity semantics, not an
implementation detail**. Changing any part of it changes which cluster
object a candidate refers to, and MUST therefore be treated as a scheme
version change (below), never as a refactor.

**Name:**

    lg-v1-<prefix>-<hash>

- `lg-v1-` — fixed literal. `lg` marks the object as this project's;
  `v1` is the **name-scheme version**, not an SPO API version. An explicit
  version is carried because a future scheme must be able to coexist with
  this one: `lg-v2-…` names cannot collide with `lg-v1-…` names, so a
  migration can proceed object by object while the collision rule below
  stays decidable.
- `<prefix>` — the pod name, lowercased, with every character outside
  `[a-z0-9-]` replaced by `-`, runs of `-` collapsed, leading/trailing `-`
  trimmed, then truncated to **40 characters** and re-trimmed. If the
  result is empty, the literal `workload` is used. Readability only; it
  carries no identity weight.
- `<hash>` — the first **8 bytes** of SHA-256 over the canonical input
  below, lowercase hex, exactly **16 characters**.

Total length is at most `6 + 40 + 1 + 16 = 63` characters, within the
DNS-1123 *label* limit and therefore comfortably within the 253-character
subdomain limit for object names. The name always begins with `l` and
always ends with a hex digit, so it is a valid DNS-1123 label and
subdomain for any input.

**Canonical hash input** — length-prefixed, never delimiter-joined, so the
encoding is injective:

    "lg-seccomp-name-v1" || 0x00
    || u32be(len(namespace)) || namespace
    || u32be(len(pod))       || pod
    || u32be(len(container)) || container

where each string is its raw UTF-8 bytes with no normalisation (Kubernetes
names are already a restricted ASCII subset), `u32be` is a 4-byte
big-endian unsigned length, and the leading tag provides domain separation
so this digest can never be confused with another use of SHA-256 in this
project. SHA-256 and this encoding are fixed by this ADR; both are stable
across Go and platform versions.

**Collision probability, and why it is not the safety argument.** A
64-bit digest gives a birthday bound of roughly `n²/2^65`: about `3·10⁻⁸`
even at a million distinct governed profiles in one cluster. That is
negligible, but it is *not* what makes the scheme safe — truncating a hash
is a readability trade, and a safety property must not rest on a
probability. Safety comes from the ownership check below, which compares
the recorded identity tuple and fails closed on any mismatch. A digest
collision therefore produces a refusal, never a silent overwrite of
another workload's profile.

### Ownership marker — normative

Ownership is recorded as a **structured set of annotations on the governed
object**, not a single boolean or string:

| Annotation | Value |
|---|---|
| `landlockgenprof.io/managed-by` | `landlock-genprof` |
| `landlockgenprof.io/name-scheme` | `v1` |
| `landlockgenprof.io/target-namespace` | the governed workload's namespace |
| `landlockgenprof.io/target-pod` | the governed pod |
| `landlockgenprof.io/target-container` | the governed container |

The identity tuple is recorded because the name alone cannot be inverted:
the hash is one-way and the prefix is lossy. Recording the tuple lets the
apply path verify that an existing object is not merely *ours* but ours
*for this exact workload*, which is what turns a hash collision from a
silent overwrite into a refusal.

**What the marker proves, and what it does not.** It proves that, under
the cluster's RBAC trust boundary, the object was created by this project
for that identity tuple under that name scheme. It is **not**
authentication: any principal permitted to write cluster-scoped
`SeccompProfile` objects can write these annotations too. RBAC is the
trust boundary; the marker is collision protection, not provenance
cryptography. This ADR does not claim otherwise, and v0.2 does not attempt
to defend against a principal that already holds cluster-scoped write on
enforcement resources.

### Collision policy

Cluster scope means a name we compute can already be taken by something we
do not own — impossible when the resource was namespaced. Before applying
a governed `SeccompProfile`, the apply path MUST read any existing object
of that name and then:

- **no object exists** → create it;
- **exists, ours, same name-scheme, identity tuple matches** → update it to
  the approved content; this is what makes a retry after a readiness
  timeout safe;
- **exists, ours, but the identity tuple or name scheme differs** →
  **fail closed**. This is the digest-collision and scheme-migration case;
- **exists, not ours** → **fail closed**. Never overwrite a cluster-scoped
  profile this project did not create, even if the enforcement content
  happens to match.

This rule is normative in ADR-0007 ("Cluster-scoped collision safety"),
since it governs the apply path; it is restated here because the naming
scheme is what makes it decidable.

## disableProfileAfterRecording

`disableProfileAfterRecording: true` on the `ProfileRecording` causes the
generated profile to carry `disabled: true` (a `SpecBase` field, not part
of `SeccompProfileSpec` proper), and SPO does not reconcile a disabled
profile onto nodes. The object remains fully readable, and its
enforcement content is unaffected by being disabled.

Its role in this boundary is **a safety property, not a mechanism**: it
guarantees the recording output stays inert regardless of what we do, so
the import path never depends on nobody else having enabled it. We do not
use SPO's enable path — enabling would mutate an SPO-owned object after
approval, which Model B exists to avoid.

`disabled` is **lifecycle state, not enforcement content**. It is the one
field deliberately not carried into the governed copy: the copy must be
active once approved, and inheriting `disabled: true` would produce an
approved artifact that enforces nothing.

## Unsupported fields

**Policy: A — reject. Import fails closed on any enforcement-relevant
field this project cannot represent.**

`internal/exporter/spo` and `pkg/spo` model only `defaultAction`,
`architectures`, and `syscalls[].names` / `syscalls[].action`. Upstream
`SeccompProfileSpec` and its `Syscall` struct carry more, and every
omission changes enforced behavior:

| Field | Effect if silently dropped |
|---|---|
| `baseProfileName` | **Narrows.** SPO unions the named base profile's syscalls in, resolving recursively up to 15 levels. A profile with a base plus three syscalls may really permit hundreds. Dropping it produces a copy far tighter than what was recorded — the CrashLoop class, arriving through the import path instead of the ordering path |
| `syscalls[].args` | **Widens.** A rule permitting a syscall only with specific argument values becomes an unconditional permit. This is a silent privilege escalation in the copy relative to the reviewed source |
| `syscalls[].errnoRet` | Changes the observable failure mode of denied calls |
| `flags` | Changes seccomp(2) filter behavior (e.g. `TSYNC`, `LOG`) |
| `listenerPath` / `listenerMetadata` | Required for `SCMP_ACT_NOTIFY` to function; dropping them breaks any notify-based rule |

Dropping can therefore both narrow and widen authority. Either direction
breaks the property this boundary exists to preserve — that the reviewed
source and the enforced copy mean the same thing.

**Verified against the `v1` schema** (`api/seccompprofile/v1`): the spec
still carries `baseProfileName`, `defaultAction`, `architectures`,
`listenerPath`, `listenerMetadata`, `syscalls` and `flags`; `Syscall`
still carries `names`, `action`, `errnoRet` and `args`; and `Arg` carries
`index`, `value`, `valueTwo`, `op`. The reject list is therefore unchanged
in substance by the move to `v1` — but the rule is deliberately written as
a **closed allow-list**, not an enumeration of known-bad fields: import
accepts `defaultAction`, `architectures`, `syscalls[].names` and
`syscalls[].action`, and refuses a source profile carrying *any* other
enforcement-relevant field, including ones added after this ADR was
written.

Import MUST refuse a source profile carrying any such field, with a
message naming the field. Refusing is honest and actionable; a silent copy is
neither. Extending the supported set later is a schema change plus tests,
and is the correct way to broaden coverage.

Note this also makes the boundary forward-safe: a future SPO field this
project has never seen causes a refusal rather than a silent semantic
loss.

## ProfileRecording contract

The recording side is where D-MIN's source comes from, so its API shape is
part of this boundary even though this ADR does not implement the
importer.

**`security-profiles-operator.x-k8s.io/v1`, kind `ProfileRecording`,
scope `Namespaced`** — verified against SPO's own CRD at v1.0.0. The
scope change that hit `SeccompProfile` did **not** hit `ProfileRecording`;
`v1` is storage, `v1alpha1` is still served.

Field values changed casing between versions and the adapter MUST use the
`v1` forms:

| Field | `v1` values | `v1alpha1` (do not use) |
|---|---|---|
| `recorder` | `Bpf`, `Logs` | `bpf`, `logs` |
| `mergeStrategy` | `None` (default), `Containers` | `none`, `containers` |
| `kind` | `SeccompProfile`, `SelinuxProfile`, `AppArmorProfile` | same |

`disableProfileAfterRecording` is **confirmed present at v1** with the
same meaning: the generated profile is not reconciled onto nodes. The
inert-source property this ADR relies on therefore survives the migration
intact.

The asymmetry worth remembering: **the recording is namespaced, the
profile it generates is not.** That is the whole reason the lineage
contract above reads labels instead of comparing `metadata.namespace`.

## Provenance

Provenance is carried **inside the governed artifact**, as annotations on
the emitted `SeccompProfile`, and is therefore covered by
`CandidateDigest`.

**Digested — because the reviewer was shown it and it shaped the decision:**

- seccomp source kind (`spo`)
- source profile name (cluster-scoped, so there is no namespace to record)
- source recording namespace, from `spo.x-k8s.io/recording-namespace`
- source container, from `spo.x-k8s.io/container-id`
- source recording name
- syscall coverage as reported by SPO, or the explicit token `unknown`

**Not digested — runtime or operational state:**

- SPO's `status` in any form
- `disabled` / installation state
- source object `resourceVersion` or `uid`
- SPO version, import timestamp

The line is: *what the reviewer was shown is bound; what is runtime state
is not.* Placing provenance inside the artifact costs nothing, because the
artifact text is already digested, and it means a falsified provenance
claim invalidates the approval rather than riding alongside it.

**Accepted consequence, stated plainly:** re-importing byte-identical
syscalls from a *different* recording changes the recording name, changes
the digest, and makes the approval stale. That is friction without an
authority change. It is accepted because a new recording is genuinely new
evidence and re-confirming that it supports the same authority is a
reasonable thing to ask of a reviewer. If operational experience shows the
friction is real and the re-review is empty, moving recording identity out
of the artifact into non-digested proposal metadata is a later, narrow
decision — source kind and coverage stay digested regardless.

## TrainingHistory boundary

**Prohibition:** No value originating from an SPO-generated profile may
populate `seenInRuns`, `runsRecorded`, any `Confidence`, any frequency,
any event count, or any timestamp presented as an observation time.

**Enforcement is structural, not conventional.** The import path
terminates in an artifact; it has no route into `internal/evidence` or
`internal/history`, and it never reaches `confidenceFor()`. There is no
code path by which imported policy could acquire a frequency, because it
never becomes an observation. Any future implementation that would make
contamination possible by ordinary refactoring is out of contract.

This is not a general external-source framework and must not become one.
It is one explicit seam for one backend.

## Seccomp source modes

Two modes, always explicit.

**Standalone** — unchanged from today. Our tracer observes filesystem,
network and syscalls; syscall data flows to `TrainingHistory`; our
synthesis produces the seccomp artifact.

**SPO mode** — our tracer observes filesystem and network. SPO observes
syscalls and generates the profile. The governed seccomp artifact is the
imported copy.

In SPO mode, **our syscall observations are not collected at all**
(option A of the choices considered). Collecting them and filtering them
out later would leave the invariant one forgotten branch away from
violation, and would produce a `TrainingHistory` describing syscall
authority that is not the authority being enforced — a semantic lie in the
reviewer's own evidence view. Not collecting is structural.

Consequences accepted: in SPO mode there is no local plain-seccomp output
and no syscall entries in `TrainingHistory`. If diagnostic syscall
collection is later wanted alongside SPO, it must be an explicitly
labelled, non-authoritative output — deferred, not implied.

Filesystem and network remain ours in both modes.

## Source selection

The seccomp source MUST be selected explicitly by the operator.
Auto-detection from CRD presence, namespace existence, or operator
availability is prohibited: it would make the meaning of `explain`'s
syscall section, and of the enforced profile, depend on invisible cluster
state.

**INV-SPO-IMPORT-06** below states the requirement; the concrete surface
(a flag, a subcommand) is an implementation choice. The selected source
MUST be visible in the governed artifact's provenance and therefore
reproducible from the proposal alone.

## CandidateDigest

**`candidate-v1` remains valid. No mechanism change.**

The digest payload is a fixed struct over `container`, `binary`,
`podLock`, `networkPolicy`, `patchedManifest`, `spoSeccompProfile`
(`digest.go:19-33`). An imported artifact occupies `spoSeccompProfile`, a
field that already exists and is already digested. The digest binds the
**governed emitted artifact**, not the original SPO bytes.

Candidate-wide approval of mixed-origin artifacts is **conservative, not
incorrect**: if the SPO-sourced seccomp artifact changes, the digest
changes and the whole candidate requires re-review, including domains
that did not change. That forces *more* review, never less, and cannot
produce a bypass. Only a security-incorrect or semantically invalid
outcome would block v0.2, and neither applies.

Per-domain or composite digests are **REQUIRED POST-v0.2** for ergonomics,
not correctness.

### Path continuity

The workload reference is derived from the governed copy, not from the SPO
source:

    governed profile name
        → operator/<governed-profile-name>.json
        → patched manifest securityContext.seccompProfile.localhostProfile
        → CandidateDigest

Every step is computed before approval, and both ends — the
`SeccompProfile` artifact and the patched manifest that references it —
are digested fields of the same candidate. **INV-SPO-IMPORT-12** below
states the resulting invariant: a reviewer approving a candidate approves
the profile *and* the reference to it as one unit, so they cannot drift
apart. ADR-0007's readiness gate then checks the live
`status.localhostProfile` against exactly that path.

### One backend contract, one digest

`CandidateDigest` MUST NOT depend on which SPO version happens to be
installed when the candidate is generated. Under a dual-support design it
would: a namespaced `v1beta1` cluster yields an artifact carrying
`metadata.namespace` and an `operator/<ns>/<name>.json` reference, while a
cluster-scoped `v1` cluster yields neither — the same workload, the same
observations, two different digests.

That would make content-bound approval a function of cluster
configuration, which is precisely what a content-bound mechanism must not
be. Hence v0.2 fixes **one** canonical backend contract (modern SPO) →
one artifact representation → one digest. This is the recorded reason
dual-version support is deferred rather than merely unscheduled.

## Canonicalization

Import requires **semantic equivalence**, not byte equality with the
source. Passing through `SPO object → seccomp.Profile → ToSeccompProfile
→ ToYAML` may legitimately change key order, quoting and formatting. It
must not change meaning: `defaultAction`, the architecture set, and the
ordered syscall rules with their names and actions must survive
unchanged. The reject-unknown-fields policy is what makes this checkable
rather than aspirational.

## Explainability

A reviewer MUST be able to tell the two epistemic kinds apart without
inference. For filesystem and network: source direct, seen in N runs,
confidence tier. For an imported seccomp domain: source SPO, derived
policy, coverage as reported or `unknown`, and **confidence: not
applicable**.

Presenting SPO-derived syscalls with a landlock-genprof confidence tier is
prohibited. Formatting is an implementation concern; the distinction is
not.

## Coverage

`spo.x-k8s.io/syscall-coverage` is **coverage across recording units per
SPO's own semantics** — how many replicas contributed each syscall within
one recording. It is not frequency across training runs.

Decision: **copy it verbatim into the governed artifact's provenance
annotations, display it in `review`/`explain`, never transform it.**
Forbidden translations: coverage → `seenInRuns`, coverage → any
`Confidence`, coverage → a percentage presented as certainty.

If the annotation is absent, coverage is recorded and displayed as
`unknown`. Absent coverage does **not** block import — SPO does not
guarantee the annotation, and refusing would make the boundary depend on
optional upstream metadata. It must never be defaulted to `0`, to `full`,
or to a confidence tier.

## Mutability and snapshot semantics

Import is a **snapshot**. The governed artifact holds copied content, not
a reference.

- Source profile mutated after import → the governed candidate is
  unchanged. The approval remains valid because what was approved is
  unchanged. ADR-0007's identity recheck separately guards the live
  *governed* object during apply.
- Source profile deleted after import → the governed candidate is
  unchanged and remains appliable.
- Workload re-recorded and re-imported → the governed artifact changes →
  `CandidateDigest` changes → the previous approval goes stale and apply
  fails closed. This is the intended behavior and is exactly the
  `OBSERVED ≠ APPROVED` property, now spanning a project boundary.

## Failure semantics

| Condition | Behavior |
|---|---|
| `partial=true` present | **FAIL-CLOSED** — refuse import |
| `recording-namespace` label ≠ target namespace | **FAIL-CLOSED** |
| `recording-id` or `container-id` label mismatch | **FAIL-CLOSED** |
| Lineage labels absent entirely | **FAIL-CLOSED** — lineage cannot be established |
| Governed name already taken by an object we do not own | **FAIL-CLOSED** — never overwritten |
| Container/recording name mismatch | **FAIL-CLOSED** |
| Source profile not found | **FAIL-CLOSED** |
| Unsupported enforcement-relevant field present | **FAIL-CLOSED**, naming the field |
| Empty or absent `defaultAction` | **FAIL-CLOSED** |
| Coverage annotation absent | **DEGRADED** — recorded as `unknown`, import proceeds |
| Source mutated after import | **SAFE** — snapshot semantics |
| Source deleted after import | **SAFE** — snapshot semantics |
| Multiple plausible candidate profiles cluster-wide | **FAIL-CLOSED** — the operator names the source; ambiguity is never resolved by guessing |
| Duplicate import of the same source | **SAFE** — deterministic, produces the same artifact and the same digest |
| Source switched from SPO to internal after review | **FAIL-CLOSED** — the artifact and its provenance change, so the digest changes and approval goes stale |
| SPO unavailable in SPO mode | **FAIL-CLOSED** — no silent fallback to internal synthesis |
| SPO absent, standalone selected | **SAFE** — unchanged behavior |

## Invariants

- **INV-SPO-IMPORT-01** — An SPO-generated `SeccompProfile` enters as
  derived policy, never as observation evidence.
- **INV-SPO-IMPORT-02** — Imported policy MUST NOT contribute to
  `TrainingHistory` frequency, `runsRecorded`, or any confidence value.
- **INV-SPO-IMPORT-03** — Partial SPO profiles MUST NOT be imported.
- **INV-SPO-IMPORT-04** — Import MUST verify recording namespace,
  recording identity and container from SPO's own labels, sufficiently to
  prevent cross-workload authority substitution; lineage failure is fatal.
- **INV-SPO-IMPORT-05** — Import creates a governed snapshot; mutation or
  deletion of the source object after import cannot alter approved
  authority.
- **INV-SPO-IMPORT-06** — The seccomp source MUST be explicitly selected,
  never inferred from cluster state, and MUST be visible in the governed
  artifact.
- **INV-SPO-IMPORT-07** — No silent fallback between SPO mode and internal
  synthesis, in either direction, at any stage.
- **INV-SPO-IMPORT-08** — Coverage MUST NOT be interpreted or displayed as
  frequency or confidence.
- **INV-SPO-IMPORT-09** — All enforcement semantics of the source profile
  MUST be preserved in the governed artifact, or the import fails. Silent
  field loss is prohibited.
- **INV-SPO-IMPORT-10** — `candidate-v1` `CandidateDigest` binds the
  governed emitted artifact, including its provenance annotations.
- **INV-SPO-IMPORT-11** — landlock-genprof MUST NOT mutate the SPO source
  object.
- **INV-SPO-IMPORT-12** — The governed profile name, the
  `localhostProfile` path derived from it, and the patched manifest that
  references it MUST all be determined before approval and MUST all be
  covered by the same `CandidateDigest`.
- **INV-SPO-IMPORT-13** — The governed profile name MUST be produced by
  the normative algorithm above, from an injective encoding of
  (namespace, pod, container). No random suffix, no server-assigned
  identity. Any change to the tag, hash, lengths or encoding is a
  name-scheme version change, never a refactor.
- **INV-SPO-IMPORT-16** — An existing governed object may be updated only
  when its recorded name scheme and identity tuple both match the
  candidate's. The ownership marker is collision protection under RBAC,
  and MUST NOT be described as authentication.
- **INV-SPO-IMPORT-14** — A cluster-scoped profile this project does not
  own MUST NOT be overwritten, whatever its content.
- **INV-SPO-IMPORT-15** — Lineage MUST be established from SPO's own
  recording labels, never from parsing a profile's name and never by
  searching for a plausible candidate.

## Consequences

**Easier.** SPO integration lands without touching the evidence model, the
history model, the digest, or the approval mechanism. The strongest
product claim becomes demonstrable across a project boundary: SPO learns
syscall authority, a human authorizes exactly that content, SPO enforces
exactly what was authorized. We stop competing with SPO on the one domain
where SPO's instrument is better. Rejecting unknown fields makes the
boundary forward-safe by default.

**Costs.** A second, explicitly-selected mode exists, so "what produced
this seccomp artifact" is now a question with two answers, and the docs,
`review` and `explain` must all carry the distinction. Rejecting
unsupported fields means some real SPO profiles — notably any using
`baseProfileName`, which is common with runtime base profiles — cannot be
imported until the schema is extended. Digesting provenance means
re-recording forces re-review even when syscalls are identical.
Candidate-wide approval means an SPO change invalidates approval for
unrelated domains.

**Foreclosed.** Treating any SPO output as raw or aggregated observation.
Auto-detecting the seccomp source. Governing a live external object.
Generalizing this seam into a plugin framework without a further decision.

## Alternatives rejected

**A — import as raw evidence.** Rejected: SPO's generated profile carries
syscall names only. Synthesizing `Event`s from it requires inventing
timestamps and occurrence counts, and any downstream `seenInRuns` or
confidence derived from them would be fabricated. SPO's raw enricher
output is node-local and not a consumable API, so there is no honest raw
source to import even in principle.

**C — govern the live SPO object directly.** Rejected: the approved object
would stay mutable by SPO and by any other actor, our approval would
depend on a `disabled` field we do not own, and enabling it after approval
would mutate SPO-owned lifecycle state. Model B costs one extra object and
removes all three problems.

**D — generic external-candidate framework.** Rejected for v0.2: with
exactly one external source, a general abstraction would be designed
against a sample size of one. One explicit, named seam is honest about
what it is; generalize when a second source exists and its differences are
known.

## Security considerations

- **Cross-workload authority substitution.** The primary threat. Mitigated
  by required lineage checks; the residual limit (no proof the recording
  observed this specific pod) is stated rather than hidden.
- **Partial recording.** A partial profile is an incomplete union and
  would under-permit; refused outright.
- **Silent field loss.** `args` loss widens authority, `baseProfileName`
  loss narrows it. Both break reviewed-equals-enforced. Refused.
- **Source mutation after import.** Neutralized by snapshot semantics;
  the live *governed* object is separately guarded by ADR-0007's identity
  recheck before binding.
- **Silent fallback.** Prohibited in both directions: a missing backend
  must fail, never quietly change what enforces the workload.
- **Provenance forgery.** Provenance rides inside the digested artifact,
  so altering it invalidates approval.
- **Confidence laundering.** Coverage cannot become confidence; imported
  syscalls carry no tier at all.

## Testing requirements

To be implemented with the decision, not before:

1. A valid completed SPO profile imports successfully.
2. `partial=true` is rejected.
3. A `recording-namespace` label naming another namespace is rejected.
4. A `recording-id` or `container-id` label mismatch is rejected.
4b. A profile carrying no lineage labels is rejected.
4c. A governed name already held by an object we do not own is rejected,
    and the existing object is left untouched.
5. Supported enforcement semantics survive the round trip
   (`defaultAction`, architectures, ordered syscall names and actions).
6. Each unsupported enforcement-relevant field (`baseProfileName`, `args`,
   `errnoRet`, `flags`, `listenerPath`, `listenerMetadata`) is rejected,
   naming the field.
7. Coverage is preserved and displayed, and never converted to confidence.
8. Absent coverage yields `unknown`, not a fabricated value, and does not
   block import.
9. An imported profile never reaches `TrainingHistory`; in SPO mode the
   history carries no syscall entries.
10. Standalone mode is byte-identical to current behavior.
11. SPO mode suppresses internal syscall collection.
12. Source selection is required; no implicit default engages SPO.
13. No fallback occurs when SPO is unavailable in SPO mode.
14. Source mutation after import does not change the governed candidate.
15. Re-import of a changed source changes `CandidateDigest` and makes the
    prior approval stale.
16. `review`/`explain` mark the seccomp domain as SPO-derived with no
    confidence tier.
17. The governed artifact is bound by `candidate-v1`; the mechanism
    version is unchanged.
18. The SPO source object is never mutated by any code path.
19. Authoritative E2E: `ProfileRecording` → generated profile → import →
    review → approve → ADR-0007 governed apply → SPO reconciliation →
    binding → enforcement.

## Migration and compatibility

Standalone behavior is unchanged: no schema change, no digest change, no
approval-mechanism change, no CLI regression. Existing proposals and
existing `approvedCandidateDigest` values remain valid — `candidate-v1` is
untouched.

Existing artifacts produced by internal synthesis are unaffected; they
simply carry no SPO provenance annotations.

## Deferred work

- `candidate-v2` / per-domain or composite digests.
- Per-domain approval.
- Extending the supported seccomp schema (`baseProfileName`, `args`,
  `errnoRet`, `flags`, notify fields) so richer SPO profiles can be
  imported rather than refused.
- Stronger, UID-bound lineage. Not merely unscheduled: an
  `ownerReference` from a cluster-scoped `SeccompProfile` to a namespaced
  `ProfileRecording` is not expressible in Kubernetes, so closing this
  needs an upstream change (a UID recorded in profile metadata) rather
  than work on our side. Structural label lineage is the ceiling until
  then.
- Dual-version SPO support (namespaced `v1beta1` alongside cluster-scoped
  `v1`) — deferred for the digest reason recorded above, not merely
  unscheduled.
- Raw SPO evidence integration, if SPO ever exposes a consumable
  observation API.
- A generic external-source framework.
- Diagnostic-only internal syscall collection alongside SPO mode.
- SELinux and AppArmor domains — entirely SPO's, not in scope here.

## References

- `internal/proposal/types.go:107-116` — artifacts stored as content, not
  references.
- `internal/proposal/digest.go:19-33` — the six digested fields.
- `internal/exporter/spo/export.go` — `ToSeccompProfile`, `ToYAML`,
  `LocalhostProfilePath`.
- `pkg/spo/types.go` — the supported subset and its documented omissions.
- `pkg/seccomp/types.go` — `Profile` and `SyscallRule`.
- `internal/evidence/evidence.go:52-72` — `OriginType` and
  `ProvenanceSource`.
- `cmd/landlock-genprof/trace.go:790` — provenance already carried
  per-event; `advise_seccomp` is `advisory`, not `direct`.
- [ADR-0006](0006-security-profile-proposal-approval-binding.md) —
  approval binds exact candidate content.
- [ADR-0007](0007-governed-apply-ordering-and-enforcement-readiness.md) —
  ordering, readiness and identity recheck at apply.
- [`docs/PROGRESS.md`](../PROGRESS.md) — demonstrated-capability ledger.
- SPO upstream, read at the v1.0.0 tag:
  `deploy/base-crds/crds/seccompprofile.yaml` (scope `Cluster`, `v1`
  storage with `v1beta1` still served),
  `deploy/base-crds/crds/profilerecording.yaml` (scope `Namespaced`, `v1`
  storage), `api/profilerecording/v1/profilerecording_types.go`
  (`spo.x-k8s.io/recording-id`, `spo.x-k8s.io/recording-namespace`,
  `spo.x-k8s.io/container-id`, and the recorder/mergeStrategy enums),
  `api/seccompprofile/v1/seccompprofile_types.go` (spec and `Syscall`
  fields), and `installation-usage.md` (recording lifecycle, merge,
  `disableProfileAfterRecording`, `operator/<name>.json`).
- Scope history, read per tag: v0.8.4 `Namespaced`; **v0.9.0 `Cluster`**;
  v0.10.0 `Cluster`; v1.0.0 `Cluster`.

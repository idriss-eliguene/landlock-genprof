# Demo script — canonical v0.2 recording and presenting guide

**Story:** SPO learns more, and still cannot self-authorize.
**Hook:** approve exactly what you reviewed.
**Principle:** `LEARNED ≠ AUTHORIZED`
**Closing line:** *Learning is automatic. Authority is not.*

This is a runbook, not a transcript. Every command below is real and matches
the current CLI (`cmd/landlock-genprof/`). Digests, syscall counts,
timestamps and profile names depend on what your cluster actually observes.
**Re-capture before recording; never present a pasted digest or syscall count
from this file as if it were your run's output.**

## Why the learner is SPO

Sourcing the drift from security-profiles-operator makes the boundary
unambiguous. SPO is a legitimate upstream system with a better syscall
instrument than this project's. It records the workload, produces a valid
`SeccompProfile`, and that profile still cannot enforce itself. Nobody
attacks anything; nothing malfunctions; the learner is *right*. The refusal
is the product.

## Infrastructure

The canonical demo runs on a **real-node k3s cluster**, not kind. This is
forced: SPO's eBPF recorder resolves container pids through `/proc`, and
under kind the node is itself a container with its own pid namespace, so the
recorder never associates a container. `./demo/setup.sh --with-cluster`
provisions the right cluster and asserts the topology.

## Timing and what to cut

Two costs dominate, and neither is the story:

| | |
|---|---|
| SPO record + generate | **~127 s per recording**, twice |
| Two training runs at `--duration 40s` | ~80 s |
| Everything that carries the argument | seconds |

Both SPO recordings are therefore **pre-baked by `./demo/setup.sh`, before
the camera rolls**. The hero demo consumes the resulting real cluster state.
Speed the two training segments up in post with a visible caption; never
shorten `--duration` silently to make the video fit.

## Prerequisites (not part of the recording)

- `./demo/setup.sh --with-cluster` — real-node cluster, Inspektor Gadget,
  project CRDs, SPO with its recorder enabled, and both real recordings.
- Plugin installed (`make install-plugin`).
- Terminal at **100 columns minimum** — digest lines are 71 characters and
  wrap into noise below that. `demo/record.sh` enforces it.
- `./demo/reset.sh` immediately before recording.

---

# Hero cut — target 5:30, hard cap 6:00

Single terminal throughout. No split screen: there is no race to stage and
no second actor in this story.

Run `./demo/scenario.sh`. Every stage below is a real stage in that script,
and every stage asserts what it claims — if the cluster does not support the
narrative, the script exits non-zero rather than printing it anyway.

---

### [0:00–0:25] Cold open — LEARNED

Two panels of real cluster state, no narration for the first five seconds.

```
┌─ security-profiles-operator observed this workload
│  SeccompProfile   lgdemo-a-tools
│  syscalls         <REAL COUNT>
│  spec.state       Disabled
└─

┌─ landlock-genprof
│  SecurityProfileProposal   <none>
└─
```

**Say:** "SPO observed this workload and produced a valid policy. Nothing is
enforcing it. That is not a bug."

Then the card: **LEARNED ≠ AUTHORIZED**

The tension is real cluster state, not commentary: SPO left the profile
`Disabled` itself, because the recording set `disableProfileAfterRecording`.

---

### [0:25–1:05] Import — derived policy enters as a candidate

`trace --seccomp-source=spo --spo-recording … --spo-profile …`

Filesystem and network authority come from this project's own observation of
the live workload; syscall authority is imported from SPO. One command,
because it is one candidate.

```
┌─ SOURCE vs GOVERNED
│  SPO source profile   lgdemo-a-tools   (state: Disabled, untouched)
│  governed copy        lg-v1-nginx-demo-<hash>
└─
```

**Say:** "SPO's object is untouched. We copied its content into an object we
own, and only that one is a candidate."

*Cut the 40 s training run to ~5 s with a visible caption.*

---

### [1:05–1:20] One workload, three domains

```
┌─ CANDIDATE A
│  filesystem   PodLock LandlockProfile   yes   (observed here)
│  network      NetworkPolicy             yes   (observed here)
│  syscalls     SPO SeccompProfile        yes   (derived by SPO)
│
│        ONE CANDIDATE / ONE DIGEST / ONE DECISION
└─
```

**Say:** "SPO did not observe the filesystem authority. It does not record
it." — This is the line that stops the audience concluding "approval wrapper
around SeccompProfile."

---

### [1:20–1:55] Review candidate A

`review` — real output. What matters on screen:

- the artifacts, across three domains;
- `Source: security-profiles-operator` / `Origin: derived policy`;
- `Coverage: unknown`;
- `Confidence: not applicable`;
- `Candidate digest: sha256:…`

**Say:** "Coverage is unknown because SPO v1.0.0 does not report it. We
record that rather than inventing a number. And confidence does not apply —
derived policy carries no occurrence data, so any tier would be fiction."

That pair of lines is what makes the approval feel like a decision rather
than a ceremony.

---

### [1:55–2:15] Approve exactly A

`approve --expected-digest <A>`

```
┌─ APPROVED
│  reviewed digest   sha256:…
│  approved digest   sha256:…
└─
```

---

### [2:15–2:40] The workload changes

The workload writes a path it never wrote before, and the learner recorded it
calling a service it had never called. Ordinary. Nobody is attacking
anything.

---

### [2:40–3:10] The learner learned more

Trace again, importing **recording B**.

```
┌─ THE PROPOSAL MOVED ON. THE APPROVAL DID NOT.
│  candidate now   sha256:<B>
│  approved        sha256:<A>
└─
```

*Cut the second training run the same way.*

---

### [3:10–3:30] THE MONEY SHOT

```bash
kubectl landlock-genprof apply-proposal nginx-demo -n landlock-genprof-e2e --yes
```

**Visible output — verbatim, do not reformat into a banner:**

```
apply preflight failed: approved candidate digest mismatch: approved=sha256:… computed=sha256:…

  exit status: 1
```

**Caption:** none. Silence. Let it sit for a full five seconds.

Exit 1 is the contract's value for a refused approval (ADR-0001: a
non-blocking finding). ADR-0007's enforcement-readiness refusal is exit 2.
The script asserts the real value; do not narrate a number you did not see.

**Then say:** "SPO learned a better policy. That still did not give it
authority."

---

### [3:30–3:45] Nothing was applied

```
┌─ CLUSTER STATE AFTER THE REFUSAL
│  governed profile in cluster   absent
│  workload seccomp binding      <none>
│  SPO source lgdemo-b-tools     state=Disabled
└─
```

Three facts, three seconds. The refusal was not cosmetic.

---

### [3:45–4:30] What changed?

**Filesystem —** real `diff`, rule by rule, over the two Landlock
candidates. The new path appears. **Say:** "SPO never saw this. It does not
record filesystem authority."

**Seccomp —** provenance, not a semantic diff:

```
┌─ SECCOMP — provenance changed
│  candidate A   recording lgdemo-a   source lgdemo-a-tools   <N> syscalls
│  candidate B   recording lgdemo-b   source lgdemo-b-tools   <M> syscalls
│
│  v0.2 diffs Landlock candidates; seccomp changes are shown by provenance.
└─
```

Be honest about this on stage. `diff` compares Landlock candidates; there is
no seccomp semantic diff in v0.2. Saying so costs four seconds and buys the
credibility of everything else.

---

### [4:30–4:40] Derived policy never became evidence

```
┌─ TrainingHistory tools-curl-<hash>
│  filesystemAccesses   <REAL>
│  syscallAccesses      0
└─
```

**Say:** "SPO's derived syscalls never became our observation evidence."

Six seconds. Do not dump the object.

---

### [4:40–4:55] Approve B

`approve --expected-digest <B>` — the decision that matches what actually
happened.

---

### [4:55–5:20] Governed apply

Four concepts on screen, in order: **digest → approval → readiness →
identity**, and binding **last**.

The apply waits for SPO to reconcile the governed profile, rechecks that what
is live is what was approved, and only then rebinds the workload.

---

### [5:20–5:30] Final proof

```
┌─ WHAT REACHED ENFORCEMENT
│  SPO OBSERVED        ✓   lgdemo-b-tools (<M> syscalls)
│  POLICY DERIVED      ✓   imported as derived policy, not evidence
│  HUMAN APPROVED      ✓   sha256:…
│  BACKEND READY       ✓   operator/lg-v1-….json
│  IDENTITY VERIFIED   ✓   approved content == enforced content
│  WORKLOAD BOUND      ✓   operator/lg-v1-….json
└─

┌─ FINAL STATE
│  SPO source profile   lgdemo-b-tools   Disabled
│  governed profile     lg-v1-…          reconciled
│  workload             nginx-demo       Running, restarts=0
└─
```

**Closing line, on the card, not spoken over:**

> **Learning is automatic. Authority is not.**

---

# Social cut — target 30 s

Cut from a full recording. Three beats: the cold open, the refusal, the
final proof. Nothing else.

| Time | Shot |
|---|---|
| 0:00–0:08 | The two cold-open panels: SPO's profile with its real syscall count and `spec.state: Disabled`, and no proposal. Caption: *"SPO recorded this workload. Nothing is enforcing it."* |
| 0:08–0:14 | The workload changes; SPO records again; candidate digest moves. Caption: *"The learner learned more."* |
| 0:14–0:22 | `apply-proposal … --yes` → the verbatim `approved candidate digest mismatch` line, exit 1. **No caption. Hold.** |
| 0:22–0:27 | The final proof panel: approved, ready, identity verified, bound, `Running restarts=0`. |
| 0:27–0:30 | Title card: `LEARNED ≠ AUTHORIZED` / *"Learning is automatic. Authority is not."* |

Must be understandable with audio muted. No narration; burned-in captions
only. No `explain`, no confidence, no artifact tour — one property.

---

# Presenting live

See [`live-checklist.md`](live-checklist.md).

Use `./demo/scenario.sh --paced` so each stage waits for a keypress. Type
the `apply-proposal` command by hand at stage 11 even if everything else is
scripted — that is the one moment where live execution earns credibility.

# Capture discipline

- Never paste a digest from this file into a recording as if it were live.
- Never reformat the product's error into a banner inside the terminal.
- Never speak a specific event count aloud; it varies per run and per arch.
- If a take is wrong, `./demo/reset.sh` and record it again. Do not edit
  frames to manufacture a successful run.

# Demo script — canonical v0.2 recording and presenting guide

**Story:** from observed behavior to governed authority.
**Hook:** approve exactly what you reviewed.
**Principle:** `OBSERVED ≠ APPROVED`

This is a runbook, not a transcript. Every command below is real and matches
the current CLI (`cmd/landlock-genprof/`). The outputs quoted here were
captured from real runs of `demo/scenario.sh` against a live cluster — but
digests, timestamps and event counts depend on what your cluster actually
observes. **Re-capture before recording; never present a pasted digest from
this file as if it were your run's output.**

## Measured timing

From two consecutive full runs (`./demo/reset.sh && ./demo/scenario.sh`) on a
kind cluster:

| | |
|---|---|
| Total wall clock | **166 s** (2 min 46 s) |
| Four training runs at `--duration 40s` | ~168 s of that |
| Everything else (review, approve, drift, apply, diff, re-approve, apply) | **~7 s combined** |

That ratio is the single most important fact for editing this demo: the
observation is nearly all of the runtime and almost none of the story. Speed
the training segments up in post with a visible caption; never shorten
`--duration` silently to make the video fit.

Both runs produced identical digests, so the scenario is reproducible, not
merely repeatable.

## Prerequisites (not part of the recording)

- Cluster with Inspektor Gadget Ready and the project CRDs applied
  (`./demo/setup.sh --with-cluster`).
- Plugin installed (`make install-plugin`).
- Linux host for the tracer. See `HOW_TO_START.md`.
- Terminal at **100 columns minimum** — digest lines are 71 characters and
  wrap into noise below that. `demo/record.sh` enforces it.
- `./demo/reset.sh` immediately before recording.

---

# Signature cut — target 4:00–4:30 edited

Single terminal throughout. No split screen: there is no race to stage and
no second actor in this story.

### [0:00–0:15] The unknown

```bash
kubectl get pod nginx-demo -n landlock-genprof-e2e \
  -o jsonpath='{.spec.containers[0].securityContext}'
```

**Visible output:** `{"runAsUser":0}`

**Caption:** *"Root. No boundary. Nobody knows what it actually needs."*

**Security property:** the problem — authority is unknown and unbounded.

**Do not claim:** that this pod is unsafe in any specific way.

---

### [0:15–1:00] Observation

The scenario runs three training runs. Show the first in full; the second
and third can be sped up hard.

```bash
kubectl landlock-genprof trace --pod nginx-demo -n landlock-genprof-e2e \
  --container tools --binary /usr/bin/curl --duration 40s --history ...
```

**Visible output:** the product's own `WORKLOAD SECURITY ANALYSIS` block:

```
WORKLOAD SECURITY ANALYSIS
Workload: landlock-genprof-e2e/nginx-demo
Container: tools
Training runs: 3
✓ filesystem: 8 item(s) -> podlock
✓ network: 3 item(s) -> networkpolicy
✓ syscalls: 37 item(s) -> spo
✓ hardening: 1 item(s) -> securitycontext
Overall confidence: 95%
...
SecurityProfileProposal published: nginx-demo
```

**Caption:** *"It watches the workload actually run. 40 seconds, real time."*

**Security property:** authority is derived from observation, not authored.

**Do not claim:** that observation is complete, or that these counts are
stable — they are not, and no number here should be read aloud.

---

### [1:00–1:10] Evidence accumulated

```bash
kubectl get traininghistory -n landlock-genprof-e2e \
  -o custom-columns='NAME:.metadata.name,RUNS:.spec.runsRecorded'
```

**Visible output:**

```
NAME                  RUNS
tools-curl-f0da955d   3
```

**Caption:** *"Three runs of evidence, accumulated in the cluster."*

**Security property:** cross-run evidence is a first-class cluster resource.

---

### [1:10–1:40] Explain — why each rule exists

```bash
kubectl landlock-genprof explain --candidate-file demo/.state/candidate-a.json
```

**Visible output:**

```
/etc
  Confidence: high (seen 5 time(s))
  Rights:
    - read_file        ABI 1 (kernel >= 5.13)
  Evidence (5 observation(s)):
    - observed at 2026-08-15T12:44:21Z
...
/dev
  Confidence: low (seen 1 time(s))
  Rights:
    - write_file       ABI 1 (kernel >= 5.13)
    - truncate         ABI 3 (kernel >= 6.2)
```

**Caption:** *"Every rule says how often it was seen — and which kernel it
needs."*

**Security property:** explainability; confidence is observation frequency
(`low` = 1, `medium` = 2, `high` = 3+).

**Do not claim:** that confidence is a statistical guarantee, or that `high`
means safe to enforce.

---

### [1:40–2:05] Review candidate A

```bash
kubectl landlock-genprof review nginx-demo -n landlock-genprof-e2e
```

**Visible output:**

```
Candidate digest: sha256:306eac30863a2c51299d755504d5a69a9eb4cccf512a815ab9e626efef7c75cc

WORKLOAD SECURITY REVIEW
Proposal: landlock-genprof-e2e/nginx-demo
Container: tools
Binary: /usr/bin/curl
Artifacts available: 4/4
- PodLock: available
- NetworkPolicy: available
- Patched Manifest: available
- SPO SeccompProfile: available
```

**Caption:** *"Four artifacts. One identity."*
**Hold one beat on the digest.**

**Security property:** the whole candidate reduces to one deterministic
digest over its approval-relevant fields.

---

### [2:05–2:25] Approve exactly A

```bash
kubectl landlock-genprof approve nginx-demo -n landlock-genprof-e2e \
  --expected-digest sha256:306eac30... --reason "reviewed with the platform team"
```

**Visible output:**

```
landlock-genprof-e2e/nginx-demo: Approved
  Reason: reviewed with the platform team

  approvalState: Approved
  approvedCandidateDigest: sha256:306eac30863a2c51299d755504d5a69a9eb4cccf512a815ab9e626efef7c75cc
  approvalMechanismVersion: candidate-v1
```

**Caption:** *"A human approved this exact candidate — and the cluster
recorded which one."*

**Security property:** approval is bound to a specific digest and persisted
in the resource status.

---

### [2:25–2:40] The workload changes

**Visible output:**

```
  the workload starts writing a path it has never written before:
    /srv/nginx/data/audit-export

  nobody re-approves anything. This is the whole point.
```

**Caption:** *"Then the workload changes. A new file path."*

**Security property:** none yet — this is the setup.

**Do not claim:** that this is an attack, a compromise, or GitOps. It is a
workload doing something new, which is the most ordinary event in a cluster.

---

### [2:40–3:00] Trace again — routine

```bash
kubectl landlock-genprof trace ... --history ...
```

**Caption:** *"So we trace it again. Standard practice. Nobody re-approves —
why would they? Nothing looks wrong."*

**Security property:** re-tracing updates the proposal spec and **preserves**
the existing approval status. That is how a stale approval arises.

---

### [3:00–3:15] Before the attempt

```bash
kubectl get networkpolicy -n landlock-genprof-e2e
```

**Visible output:** `No resources found in landlock-genprof-e2e namespace.`

**Caption:** *"Nothing applied yet."*

**Why it exists:** establishes the before-state so the after-state means
something.

---

### [3:15–3:30] THE MONEY SHOT

```bash
kubectl landlock-genprof apply-proposal nginx-demo -n landlock-genprof-e2e \
  --yes --skip=podlock,spo-seccompprofile
```

**Visible output — verbatim, do not reformat into a banner:**

```
apply preflight failed: approved candidate digest mismatch: approved=sha256:306eac30863a2c51299d755504d5a69a9eb4cccf512a815ab9e626efef7c75cc computed=sha256:58614d8cf24d261197ca61e851828197ff2f064e275ebe696ca819d180a36643

  exit status: 1
```

**Caption:** none. Silence. Let it sit.

**Security property:** fail-closed approval validation. The digest is
recomputed over the *current* spec and compared against the approved one.

**Do not claim:** "REFUSED" as if the CLI printed it. It does not. If a
caption is wanted, put it outside the terminal frame.

---

### [3:30–3:45] Nothing was applied

```bash
kubectl get networkpolicy -n landlock-genprof-e2e
```

**Visible output:** `No resources found in landlock-genprof-e2e namespace.`

**Caption:** *"It stopped before it touched the cluster."*

**Security property:** in this path the rejection occurs before the first API
application.

**Do not claim:** that landlock-genprof can never partially apply artifacts.
A successful apply is sequential and continues past a failed artifact.

---

### [3:45–4:05] What actually changed

```bash
kubectl landlock-genprof diff demo/.state/candidate-a.json demo/.state/candidate-b.json
```

**Visible output:**

```
+ /srv/nginx/data: [write_file truncate]
```

**Caption:** *"Not just a different hash — a new privilege. It now writes
somewhere it never wrote before."*

**Security property:** the digest mismatch is legible as an authority change.

**Why it exists:** without this, a skeptical viewer dismisses the whole demo
as "two hashes were different."

---

### [4:05–4:30] Governed apply, after re-review

```bash
kubectl landlock-genprof review nginx-demo -n landlock-genprof-e2e
kubectl landlock-genprof approve nginx-demo -n landlock-genprof-e2e --expected-digest sha256:58614d8c...
kubectl landlock-genprof apply-proposal nginx-demo -n landlock-genprof-e2e --yes --skip=podlock,spo-seccompprofile
```

**Visible output:**

```
This will apply 1 artifact(s):
  - NetworkPolicy

Planned artifacts:
  - NetworkPolicy: networking.k8s.io/v1, Kind=NetworkPolicy landlock-genprof-e2e/nginx-demo

applied: NetworkPolicy

Done.
```

**Caption:** *"Review the change, approve the change, and it applies. The
gate isn't friction — it's the point."*

**Security property:** the governed path succeeds for current, legitimate
authority.

**Do not claim:** that the NetworkPolicy is now enforcing anything. It was
applied through the Kubernetes API. Enforcement is the CNI's job and is not
demonstrated here.

---

### Closing card

```
OBSERVED ≠ APPROVED
```

*The workload changed. The approval didn't. So it didn't apply.*

---

# Hero cut — target 30 s

Starts from an already-approved candidate A. Run `./demo/scenario.sh --paced`
and stop after stage 6, or record after a full run and cut.

| Time | Shot |
|---|---|
| 0:00–0:06 | `kubectl get securityprofileproposal nginx-demo -o jsonpath='{.status.approvalState}{"\n"}{.status.approvedCandidateDigest}'` → `Approved` + digest A. Caption: *"A human approved this exact candidate."* |
| 0:06–0:12 | The workload writes a new path; retrace, sped up hard. Caption: *"The workload changed. Someone re-traced it."* |
| 0:12–0:20 | `apply-proposal … --yes` → the verbatim `apply preflight failed: approved candidate digest mismatch: …` line. **No caption. Hold.** |
| 0:20–0:26 | `kubectl get networkpolicy -n landlock-genprof-e2e` → `No resources found`. Caption: *"Nothing was applied."* |
| 0:26–0:30 | Title card: `OBSERVED ≠ APPROVED` / *"Approve exactly what you reviewed."* |

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

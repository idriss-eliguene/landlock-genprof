# Live demo checklist

The climax of this demo is a **refusal**. If anything looks broken, the
audience cannot tell "the demo failed" from "the product refused" — which is
the worst possible ambiguity for this particular story.

**Non-negotiable: never debug live.** Switch to the recording and say
"here's the run I captured this morning." An audience forgives a recording
instantly and forgives live troubleshooting never.

## 30 minutes before

- [ ] Docker running, with enough memory for the kind cluster.
- [ ] `kubectl config current-context` = `kind-landlock-genprof-e2e`
      (or `DEMO_EXPECTED_CONTEXT` is set to whatever you are using).
- [ ] `kubectl get nodes` → Ready.
- [ ] Gadget Ready:
      `kubectl -n gadget get daemonset gadget` → desired == ready.
- [ ] CRDs present:
      `kubectl get crd securityprofileproposals.landlockgenprof.io traininghistories.landlockgenprof.io`
- [ ] Plugin resolves: `kubectl landlock-genprof version`.
- [ ] Images already pulled into the cluster: `hashicorp/http-echo:0.2.3`,
      `curlimages/curl:8.3.0`.
- [ ] `./demo/setup.sh` → "Demo environment ready".
- [ ] **Full rehearsal run**: `./demo/reset.sh && ./demo/scenario.sh`
      → exits 0, takes ~2 min 45 s.
- [ ] `./demo/reset.sh` again, so you present from a clean state.
- [ ] Terminal at **100+ columns**, font large enough that a
      `sha256:` line is readable from the back of the room.
- [ ] Recording of the identical run open in a second terminal, ready to
      `asciinema play demo/recording/signature.cast`.
- [ ] Laptop on mains power; screensaver and notifications off.
- [ ] Network: the demo needs the cluster, not the internet — but image
      pulls do. Confirm nothing still needs pulling.

## Plan A — live

```bash
./demo/scenario.sh --paced
```

Step through with the return key. Narrate from
[`script.md`](script.md). Type the `apply-proposal` command by hand at
stage 11.

Budget ~5 minutes: the four training runs are 40 s each and cannot be
compressed live. Say so out loud — "this is a real 40-second training run"
— rather than apologizing for the wait.

## Plan B — the recording

Trigger immediately, without discussion, on any of:

- the cluster is unreachable, or `kubectl` hangs for more than ~10 s;
- the Gadget DaemonSet is not Ready;
- a training run reports `0 item(s)` across all domains;
- the stale apply *succeeds* (the scenario stops itself and says so);
- the apply is refused for a reason other than the digest mismatch;
- anything at all needs a second attempt.

```bash
asciinema play demo/recording/signature.cast
```

Say: *"The cluster isn't cooperating — here's the identical run I captured
earlier."* Then keep narrating exactly as planned. The recording is real
output from the same scenario, so nothing in your narration changes.

## Plan C — total AV or machine failure

Slides carrying the real captured outputs from `demo/.state/` of a
successful run: the approval status, the verbatim refusal line, the two
`No resources found` checks, and the `diff` output. Keep it in the deck
before you travel.

## After presenting

```bash
./demo/reset.sh
```

Leaves the cluster ready for the next run, with no proposal, no approval and
no accumulated history.

# Step 7 — Optional multi-run history (`--history`)

`Confidence` is meant to reflect how many separate training runs
observed an access ("seen on every run" vs "seen once out of 5 runs"),
but a single `trace` run has no way to know that — it can only measure
how many times something was seen *within* that one run. Pass
`--history` to persist a `TrainingHistory` custom resource
(`internal/history`, no controller — the CLI reads/writes it directly)
that accumulates across every `--history` run for the same
container/binary, so `Confidence` can finally be computed from the real
ratio. Requires the CRD and additional RBAC, applied once:
[`../../deploy/crd-traininghistory.yaml`](../../deploy/crd-traininghistory.yaml),
[`../../deploy/rbac-history.yaml`](../../deploy/rbac-history.yaml). Query the result
directly: `kubectl get traininghistory <container>-<binary-basename> -o
yaml`. `profile.yaml`/`networkpolicy.yaml`/`capabilities.yaml` themselves
show it too — every path/port/capability gets a trailing `# confidence:
...` comment (see Step 4), and with `--history` that comment reflects the
real cross-run ratio instead of the single-run estimate used without it.
`seccomp.json` (Step 8) can't carry a comment — its confidence
is printed to stdout instead.

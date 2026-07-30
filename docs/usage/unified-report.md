# Step 11 — Optional unified review report (`--report-out`)

Pass `--report-out` to also generate one Markdown report combining all
four observed domains — filesystem, network, syscalls, capabilities —
for a single review pass, instead of up to five separate files:

```markdown
# Security Profile Review — nginx-demo

- **Generated:** 2026-07-24T10:00:00Z
- **Namespace/Container:** default/nginx
- **Binary:** /usr/sbin/nginx
- **Training duration:** 1m0s
- **--history used:** no — Confidence below is internal/policy's single-run proxy, not a real cross-run ratio

## Filesystem
| Path | Permissions | Confidence |
|---|---|---|
| `/etc/nginx` | read | high |

## Capabilities
No capability checks observed. Capability checks cluster heavily at
container startup — if this container was already running before this
trace started, there may be nothing left to observe — see
`e2e-demo.md` Finding 5 and re-run with `--restart`.

## Review checklist
- [ ] Re-run with `--history` a few times before trusting any `low`/`medium` entry above.
- [ ] Re-run with `--restart` — capabilities and/or syscalls came back empty...
```

Unlike every other `--*-out` flag, this one is **never skipped** when
passed, even if a domain observed nothing at all — an empty domain is
itself useful review content (usually the startup blind spot from Step
6/Finding 5, worth surfacing directly rather than leaving the reader
to rediscover it). It also works **standalone**, independent of the
other `--*-out` flags: `internal/policy.Synthesize` already populates
all four IR domains every run regardless of which flags were passed
(all six gadgets always run), so the report shows the real data
directly — and additionally links to any of the other files that were
also generated this same run.

# Recording guide

The published Asciinema recording is
[`Y0IHrGK0zYcDbgaw`](https://asciinema.org/a/Y0IHrGK0zYcDbgaw). Any replacement
must show the current SPO-derived-policy, digest-bound approval, stale-authority
rejection, and governed-apply narrative from `demo/scenario.sh`.

The canonical asset is an **asciinema cast**: it is text, it diffs in a pull
request, and it is regenerable from `demo/scenario.sh`. Everything published
(GIF, embedded player) is derived from it. There is no frame editing anywhere
in this pipeline — a bad take is re-run, not repaired.

```
  real cluster + real CLI
        │
        ▼
  demo/scenario.sh            (sequence only, no product logic)
        │
        ▼
  asciinema rec               demo/recording/signature.cast   ← local capture
                              demo/recording/hero.cast        ← local capture
        │
        ├── agg ────────────► demo/recording/hero.gif         ← README embed
        └── asciinema player  (GitHub Pages, below the fold)
```

## Tooling

| Tool | Use | Install |
|---|---|---|
| `asciinema` | capture | <https://docs.asciinema.org/getting-started/> |
| `agg` | cast → GIF | <https://github.com/asciinema/agg> |
| `tmux` | only if you want traffic visible alongside a trace | your package manager |

Deliberately **not** used: VHS and Terminalizer (their simulated typing reads
as synthetic, and asciinema is already the project's format), OBS/ffmpeg/MP4
(a video-production pipeline with no committed venue), and any custom demo
dashboard (maintenance cost, and a panel that displays product state tends to
drift into interpreting it).

## Environment

- Linux host (the tracer is Linux-only). See `HOW_TO_START.md`.
- Terminal **100×32 minimum**. `record.sh` refuses to record narrower:
  a `sha256:` line is 71 characters and wraps into noise below 100 columns.
- A clean demo state: `./demo/reset.sh`.
- Plain prompt, no git branch decorations, no personal paths on screen.

## Recording

```bash
./demo/record.sh signature   # runs reset, then records scenario.sh unattended
./demo/record.sh hero        # records an interactive shell for the short cut
```

`record.sh` verifies the context, terminal width and asciinema availability,
then writes into `demo/recording/`. It prompts before replacing an existing
cast.

For the hero cut, run the scenario to stage 6 first
(`./demo/scenario.sh --paced`, stop after the approval), then record the
short sequence from [`script.md`](script.md).

## Trace-time compression — say it on screen

Four training runs at `--duration 40s` are ~168 s of the ~166 s total
runtime. In the edit, speed those segments up and put a **visible caption**
on them:

> 40 s training run, sped up

Never lower `--duration` just to make the recording shorter and then present
it as the same run. The repo has held this rule since the v0.1 script; keep
holding it.

## Rendering

```bash
agg --cols 100 --rows 32 \
    demo/recording/hero.cast \
    demo/recording/hero.gif
```

Keep the README GIF under ~3 MB — use the **hero** cut for it, not the
signature. GitHub does not autoplay video in a README, so a GIF is the only
format that moves there.

## Retakes

```bash
./demo/reset.sh                  # or --recreate-pod for a fully clean filesystem
./demo/record.sh signature
```

Because the scenario is a script, retakes are cheap and reproducible: two
consecutive runs produced identical candidate digests.

## Verifying that everything on screen is real

Before publishing, check the cast against `demo/.state/` from the same run:

- `review-a.txt` / `review-b.txt` — the two digests shown.
- `apply-stale.err` — the refusal line, verbatim.
- `diff-a-b.txt` — the rule that changed.
- `apply-b.out` — the successful apply.

If a value on screen is not in one of those files, it did not come from the
product, and it does not ship.

## Asset locations

| Asset | Path | Committed |
|---|---|---|
| Signature cast | `demo/recording/signature.cast` | yes |
| Hero cast | `demo/recording/hero.cast` | yes |
| Hero GIF | `demo/recording/hero.gif` | yes (derived, regenerable) |
| Run outputs | `demo/.state/` | no (gitignored) |

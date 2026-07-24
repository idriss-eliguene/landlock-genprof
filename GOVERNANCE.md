# Governance

## Today

`landlock-genprof` has one maintainer (see [`MAINTAINERS.md`](MAINTAINERS.md))
and no formal governing body yet — that's an honest reflection of project
size, not a permanent design choice. This document exists now, before the
project needs it, so the rules are set by precedent rather than by whoever
shows up first once there's something worth disputing.

## Decision-making

- **Day-to-day** (bug fixes, small features, docs): a maintainer merges
  after the required CI checks (`build-and-test`, `security`) and a code
  owner review pass, per [`.github/CODEOWNERS`](.github/CODEOWNERS).
- **Non-trivial changes** (new exporter/backend, a behavior change, a
  breaking CRD/CLI change): open an issue or draft PR first — see
  [`CONTRIBUTING.md`](CONTRIBUTING.md#before-you-start). Maintainers decide
  by consensus; if there's disagreement, the maintainer with the most
  context on the affected area gets the final call, documented in the
  issue/PR itself.
- **Project direction** (roadmap, scope, positioning against
  PodLock/SPO/Cilium): see
  [`docs/product-definition-v1.md`](docs/product-definition-v1.md) and
  [`docs/roadmap.md`](docs/roadmap.md). Changes to either go through a PR
  like any other change, reviewable and diffable.

## Becoming a maintainer

No fixed contribution count or tenure — the bar is demonstrated, sustained
judgment on this specific codebase: several substantive PRs merged, review
comments that catch real issues (not just style), and familiarity with the
project's own conventions (see `CONTRIBUTING.md`'s "Code conventions" and
the "confirmed live" discipline running through `docs/roadmap.md`). An
existing maintainer proposes it; other maintainers have a chance to object
before it's final.

## Code of conduct

[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) — the CNCF Code of Conduct,
adopted as-is given this project's trajectory.

## Changing this document

This file is itself governed by the "non-trivial changes" rule above: a
PR, reviewed like any other.

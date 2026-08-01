# Versioned docs site

The mdBook site (`idriss-eliguene.github.io/landlock-genprof/`) is
published per-version, with a selector to jump between them — not just
whatever `master` currently looks like.

## How it works

- **`gh-pages` branch**, not the Actions-artifact Pages deployment method
  — each version is built once and persists as static files, cheap to
  serve on every subsequent request. Repo Pages settings: "Deploy from a
  branch" → `gh-pages` (`build_type: legacy`, checked via
  `gh api repos/.../pages`).
- One subdirectory per version: `/v0.1.1/`, `/v0.1.2/`, `/master/` (the
  unreleased, currently-in-development docs). The site root `/` is a
  redirect to the latest tagged version, not `/master/`.
- `versions.json` at the `gh-pages` root lists every published version;
  `version-selector.js`/`.css` (also at the root, so every version's
  pages can reference the same copy) read it and inject a dropdown into
  mdBook's own menu bar (`#menu-bar .right-buttons`).

## Building a version

```bash
hack/build-versioned-docs.sh <git-ref> <output-dir>
```

Builds `<git-ref>`'s `book/` (via a git worktree, so it doesn't disturb
your actual checkout) using **this** checkout's mdbook/mdbook-mermaid/Go
tooling — not whatever was pinned in that ref's own CI config. Older
tags don't need a working toolchain of their own kept alive forever, and
the version selector doesn't need to have existed in that tag's source
at all: it gets injected into the built HTML afterward, unconditionally.

Skips silently (not an error) if the ref has no `book/` directory at
all — true for `v0.1.0`, before the mdBook site existed.

## How publishing works now (automated, staging-first)

Three workflows, each touching only its own path — a change never
reaches a production path without a human having reviewed a live
preview of it first:

- **`docs-preview.yml`** — every PR touching doc-relevant paths gets
  its own build, deployed to `gh-pages` under
  `pr-preview/pr-<number>/`, with a comment on the PR linking to it.
  Cleaned up automatically when the PR closes.
- **`docs.yml`** — only on an actual push to `master` (i.e., after a
  PR has merged): rebuilds `master` and deploys to the real `/master/`
  production path.
- **`docs-release.yml`** — only on a `vX.Y.Z` tag push: builds that
  tag, deploys to its own `/vX.Y.Z/`, and updates `versions.json` +
  the root redirect to point at it as latest.

None of the three ever writes to a path another one owns. This exists
specifically because the first rollout of this whole mechanism skipped
straight to editing production directly (see the gotchas below) — twice.

## Adding a version manually (rare — normally the workflows above do this)

1. `hack/build-versioned-docs.sh <ref> <tmpdir>`
2. `git worktree add /tmp/landlock-gh-pages gh-pages`
3. Copy `<tmpdir>` to `gh-pages/<version>/`
4. For a new release specifically (not `master`): also run
   `hack/update-doc-versions-manifest.py gh-pages/versions.json <version>`
5. Commit and push directly to `gh-pages` — this branch holds generated
   output, not reviewed source, so it doesn't go through a PR. **The
   two scripts above are source and do.**

## Gotchas that actually broke production (twice, before the staging
## workflow above existed)

Both hit during the very first rollout, both fixed by pushing straight
to `gh-pages` at the time — which is exactly the shortcut the workflows
above now exist to stop happening again.

**1. Every absolute path needs the `/landlock-genprof/` prefix.** This
is served from `idriss-eliguene.github.io/landlock-genprof/` — a
**project** page, not a user/org root page
(`idriss-eliguene.github.io/`). `version-selector.js`, the injected
`<link>`/`<script>` tags, and the root `index.html` redirect all assumed
domain-root hosting at first — a bare `/v0.1.2/` resolves to the wrong
path entirely and 404s. Verified with `curl` at the time, which doesn't
follow `<meta http-equiv="refresh">` or execute JS, so the bug was
invisible to that check and only showed up in an actual browser.

**2. `gh-pages` needs a `.nojekyll` file at its root, not just inside
each version subdirectory.** mdBook generates one per build (inside
`book/dist/`), which lands inside `v0.1.2/`, `master/`, etc. — but
GitHub Pages only skips Jekyll processing for the whole site if that
file sits at the branch root. Without it, GitHub Pages ran everything
through Jekyll, which is why `index.html` files that clearly existed in
the branch (confirmed via the contents API) still 404'd on the live
site. Also discovered mid-incident: the *old* Actions-artifact-based
`docs.yml` was still running and "succeeding" on every push even after
the repo's Pages source was switched to branch-based (`legacy`) — dead
weight at best, a second confusing deployment path at worst. Removed
entirely as part of adding the staging workflow above.

Verify any future change to these paths (or to Pages settings) by
actually navigating the live site, not just curling individual files
for a 200 — that's exactly what missed both of these.

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

## Adding a new version to the live site

1. `hack/build-versioned-docs.sh <ref> <tmpdir>`
2. Check out the `gh-pages` branch (a `git worktree` is cleanest —
   see the command history in this project's session notes, or just
   `git worktree add /tmp/landlock-gh-pages gh-pages`)
3. Copy `<tmpdir>` to `gh-pages/<version>/`
4. For a new release specifically (not `master`): also run
   `hack/update-doc-versions-manifest.py gh-pages/versions.json <version>`
   to update the manifest and regenerate the root redirect
5. Commit and push directly to `gh-pages` — this branch holds generated
   output, not reviewed source, so it doesn't go through a PR. **The
   two scripts above are source and do** (see `hack/` in the normal
   PR-reviewed tree).

Not yet automated in CI — currently a manual step per release. Wiring
this into `.github/workflows/docs.yml` (for `master`) and a new
tag-triggered workflow (for releases) is the next step, not done yet.

## The one gotcha that actually broke production once

This is served from `idriss-eliguene.github.io/landlock-genprof/` — a
**project** page, not a user/org root page
(`idriss-eliguene.github.io/`). Every absolute path in
`version-selector.js`, the injected `<link>`/`<script>` tags, and the
root `index.html` redirect **must** be prefixed with `/landlock-genprof/`
— a bare `/v0.1.2/` resolves to the wrong domain path entirely and 404s.
Hit this for real on the first rollout (verified with `curl`, which
doesn't follow `<meta http-equiv="refresh">` or execute JS, so the bug
was invisible to that check and only showed up in an actual browser).
Verify any future change to these paths by actually navigating the site,
not just curling individual files for a 200.

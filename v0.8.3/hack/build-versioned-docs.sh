#!/usr/bin/env bash
# Builds the mdBook site for one git ref into a standalone output
# directory, then injects the cross-version selector (version-selector.js/
# .css, served from the gh-pages branch root) into every generated HTML
# page.
#
# Uses THIS checkout's mdbook/mdbook-mermaid tooling and this script
# itself against the TARGET ref's book/docs content (via a git worktree)
# — so older tags don't need their own working CI toolchain pinned
# forever, and the selector doesn't need to have existed in that tag's
# source at all. See docs/versioned-docs.md for the full design.
#
# Usage: hack/build-versioned-docs.sh <git-ref> <output-dir>
set -euo pipefail

ref="${1:?usage: build-versioned-docs.sh <git-ref> <output-dir>}"
outdir="${2:?usage: build-versioned-docs.sh <git-ref> <output-dir>}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="$(mktemp -d)"
trap 'git -C "$repo_root" worktree remove --force "$workdir" 2>/dev/null || rm -rf "$workdir"' EXIT

if ! git -C "$repo_root" ls-tree -d "$ref" -- book >/dev/null 2>&1; then
	echo "warning: $ref has no book/ directory — nothing to build, skipping" >&2
	exit 0
fi

git -C "$repo_root" worktree add --detach "$workdir" "$ref" >&2

(
	cd "$workdir"
	make docs-cli
	mdbook build book
)

mkdir -p "$outdir"
rm -rf "${outdir:?}"/*
cp -r "$workdir/book/dist/." "$outdir/"

python3 - "$outdir" <<'PYEOF'
import pathlib
import sys

outdir = pathlib.Path(sys.argv[1])
inject = (
    '<link rel="stylesheet" href="/landlock-genprof/version-selector.css">'
    '<script src="/landlock-genprof/version-selector.js" defer></script>\n</head>'
)
for html_file in outdir.rglob("*.html"):
    content = html_file.read_text(encoding="utf-8")
    if "version-selector.js" not in content:
        html_file.write_text(content.replace("</head>", inject, 1), encoding="utf-8")
PYEOF

echo "built $ref -> $outdir" >&2

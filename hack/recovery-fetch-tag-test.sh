#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
fetch_tag="$root_dir/hack/recovery-fetch-tag.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

remote_dir="$tmp_dir/remote.git"
seed_dir="$tmp_dir/seed"
clone_dir="$tmp_dir/clone"
git init --bare "$remote_dir" >/dev/null
git init "$seed_dir" >/dev/null
printf 'release\n' > "$seed_dir/file"
git -C "$seed_dir" add file
git -C "$seed_dir" -c user.email=recovery-test@example.invalid -c user.name=recovery-test commit -m release >/dev/null
product_sha="$(git -C "$seed_dir" rev-parse HEAD)"
git -C "$seed_dir" tag -a v0.8.1 -m v0.8.1
git -C "$seed_dir" tag v0.8.2 "$product_sha"
git -C "$seed_dir" remote add origin "$remote_dir"
git -C "$seed_dir" push origin HEAD:refs/heads/master --tags >/dev/null

git clone "$remote_dir" "$clone_dir" >/dev/null
resolved="$(cd "$clone_dir" && "$fetch_tag" origin v0.8.1 refs/tags/recovery-v0.8.1 2>/dev/null)"
[ "$resolved" = "$product_sha" ]

# A local stale tag must not be trusted: the helper refreshes the namespaced
# remote ref before resolving it.
git -C "$clone_dir" -c user.email=recovery-test@example.invalid -c user.name=recovery-test commit --allow-empty -m stale >/dev/null
git -C "$clone_dir" tag -f v0.8.1 HEAD >/dev/null
resolved="$(cd "$clone_dir" && "$fetch_tag" origin v0.8.1 refs/tags/recovery-v0.8.1 2>/dev/null)"
[ "$resolved" = "$product_sha" ]

expect_fail() {
  if "$@" >/dev/null 2>&1; then
    echo "unexpected pass: $*" >&2
    exit 1
  fi
}

run_fetch() { (cd "$clone_dir" && "$fetch_tag" "$@"); }
expect_fail run_fetch origin v9.9.9 refs/tags/recovery-v9.9.9
expect_fail run_fetch origin v0.8.1 refs/tags/recovery-v0.8.2
expect_fail run_fetch origin -v0.8.1 refs/tags/recovery-v0.8.1
expect_fail run_fetch origin 'v0.8.1:refs/heads/master' refs/tags/recovery-v0.8.1
expect_fail run_fetch other v0.8.1 refs/tags/recovery-v0.8.1

# Regression for the incident: the remote name must not also be passed as a
# refspec (the old equivalent was `git fetch origin <tag-ref> origin master`).
if git -C "$clone_dir" fetch --no-tags origin "refs/tags/v0.8.1:refs/tags/recovery-v0.8.1" origin master >/dev/null 2>&1; then
  echo "malformed duplicate-remote fetch unexpectedly passed" >&2
  exit 1
fi

echo "recovery remote-tag resolution tests passed"

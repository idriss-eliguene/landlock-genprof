#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 REMOTE TAG DESTINATION_REF" >&2
  exit 2
}

[ "$#" -eq 3 ] || usage
remote=$1
tag=$2
destination=$3

[[ "$remote" == "origin" ]] || { echo "recovery remote must be origin" >&2; exit 1; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "invalid recovery tag" >&2; exit 1; }
[[ "$destination" =~ ^refs/tags/recovery-v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "invalid recovery destination ref" >&2
  exit 1
}
[ "$destination" = "refs/tags/recovery-$tag" ] || {
  echo "destination ref does not match recovery tag" >&2
  exit 1
}

git fetch --no-tags -- "$remote" "refs/tags/$tag:$destination"
git rev-parse "$destination^{commit}"

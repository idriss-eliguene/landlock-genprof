#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-idriss-eliguene/landlock-genprof}"
readme="${README_FILE:-README.md}"
urls="$(grep -Eo 'https://github\.com/[^ )]+/releases/download/[^ )]+' "$readme" | sort -u || true)"
[ -n "$urls" ] || { echo "No public release-download URLs found in $readme" >&2; exit 2; }

status=0
while IFS= read -r url; do
  [ -n "$url" ] || continue
  echo "Checking $url"
  if ! curl --fail --silent --show-error --location --head --max-time "${CURL_MAX_TIME:-20}" "$url" >/dev/null; then
    echo "PUBLIC_ASSET_UNREACHABLE: $url" >&2
    status=1
  fi
done <<< "$urls"

tag="${RELEASE_TAG:-}"
asset="${RELEASE_ASSET:-}"
if [ -n "$tag" ] && [ -n "$asset" ]; then
  api="https://api.github.com/repos/$repo/releases/tags/$tag"
  if ! curl --fail --silent --show-error --location --max-time "${CURL_MAX_TIME:-20}" "$api" | grep -F '"name": "'"$asset"'"' >/dev/null; then
    echo "RELEASE_ASSET_MISSING: $repo@$tag $asset" >&2
    status=1
  fi
fi

exit "$status"

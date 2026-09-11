#!/usr/bin/env bash

# Pure validation and reference derivation shared by the published-release
# Lima harness and its non-publishing tests. This file never contacts a
# registry or Kubernetes and never performs a mutation.

published_release_fail() {
  echo "ERROR: $*" >&2
  return 1
}

published_release_validate_version() {
  local version=${1:-}
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
    published_release_fail "RELEASE_VERSION must match vMAJOR.MINOR.PATCH"
}

published_release_validate_tag_ref() {
  local version=${1:-}
  published_release_validate_version "$version" || return
  [[ "$version" != *:* && "$version" != *" "* && "$version" != *$'\n'* ]] ||
    published_release_fail "release version contains refspec/whitespace characters"
}

published_release_image_reference() {
  local version=${1:-}
  published_release_validate_tag_ref "$version" || return
  printf 'ghcr.io/idriss-eliguene/landlock-genprof-operations-center:%s\n' "$version"
}

published_release_chart_reference() {
  local version=${1:-}
  published_release_validate_tag_ref "$version" || return
  printf 'oci://ghcr.io/idriss-eliguene/charts/landlock-genprof\n'
}

published_release_chart_version() {
  local version=${1:-}
  published_release_validate_tag_ref "$version" || return
  printf '%s\n' "${version#v}"
}

published_release_require_full_digest() {
  local digest=${1:-}
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] ||
    published_release_fail "expected a full immutable sha256 digest"
}

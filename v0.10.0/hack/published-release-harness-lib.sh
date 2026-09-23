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

published_release_port_is_free() {
  local port=${1:-}
  ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

published_release_normalize_digest() {
  local value=${1:-}
  value="$(printf '%s' "$value" | sed -e $'s/\r//g' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ "$value" == *@* ]]; then
    value="${value##*@}"
  fi
  [[ "$value" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    published_release_fail "expected exactly one full sha256 digest"
    return 1
  }
  printf '%s\n' "$value"
}

published_release_namespace_selector_set_arg() {
  local namespace=${1:-}
  printf 'operationsCenter.networkPolicy.trustedProxy.namespaceSelector.matchLabels.kubernetes\\.io/metadata\\.name=%s\n' "$namespace"
}

published_release_select_containerd_source() {
  local digest=${1:-}
  local image_names=${2:-}
  local imported direct
  imported="$(awk -v digest="$digest" '
    index($0, "import-") == 1 && substr($0, length($0) - length(digest) + 1) == digest {
      print
    }
  ' <<<"$image_names" | sort -u | awk 'NR == 1 { print; exit }')"
  if [ -n "$imported" ]; then
    printf '%s\n' "$imported"
    return 0
  fi
  direct="$(awk -v digest="$digest" '
    index($0, "@") > 0 && substr($0, length($0) - length(digest) + 1) == digest {
      print
    }
  ' <<<"$image_names" | sort -u | awk 'NR == 1 { print; exit }')"
  [ -n "$direct" ] || return 1
  printf '%s\n' "$direct"
}

published_release_verify_containerd_reference() {
  local expected_reference=${1:-}
  local image_names=${2:-}
  awk -v expected="$expected_reference" '$0 == expected { found = 1 } END { exit found ? 0 : 1 }' <<<"$image_names"
}

published_release_platform_manifest_digest() {
  local inspect_output=${1:-}
  local platform=${2:-}
  local digest
  digest="$(awk -v platform="$platform" '
    /application\/vnd\.oci\.image\.manifest\.v1\+json @sha256:/ {
      digest = $0
      sub(/^.*@/, "", digest)
      sub(/[[:space:]].*$/, "", digest)
    }
    index($0, "Platform: " platform) { print digest; exit }
  ' <<<"$inspect_output")"
  [ -n "$digest" ] || return 1
  printf '%s\n' "$digest"
}

published_release_oci_platform_manifest_digest() {
  local index_json=${1:-}
  local platform=${2:-}
  jq -er --arg platform "$platform" \
    '.manifests[] | select((.platform.os + "/" + .platform.architecture) == $platform) | .digest' \
    <<<"$index_json"
}

published_release_verify_oci_custody() {
  local canonical_index=${1:-}
  local runtime_index=${2:-}
  local canonical_manifest=${3:-}
  local runtime_manifest=${4:-}
  local platform=${5:-}
  local canonical_platform_digest runtime_platform_digest
  canonical_platform_digest="$(published_release_oci_platform_manifest_digest "$canonical_index" "$platform")" || return 1
  runtime_platform_digest="$(published_release_oci_platform_manifest_digest "$runtime_index" "$platform")" || return 1
  [ "$canonical_platform_digest" = "$runtime_platform_digest" ] || return 1
  jq -e --arg digest "$canonical_platform_digest" \
    --argjson runtime "$runtime_manifest" \
    '.config.digest == ($runtime.config.digest) and
     ([.layers[].digest] == [$runtime.layers[].digest])' \
    <<<"$canonical_manifest" >/dev/null
}

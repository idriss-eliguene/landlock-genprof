#!/usr/bin/env bash

# Keep this helper compatible with the oldest Bash that may launch the
# contributor entrypoints. The feature requiring Bash 4.0 is mapfile in the
# Core project-layer installation path.

bash_version_supported() {
  local major="$1" minor="$2" required_major required_minor
  case "$major:$minor" in
    ''|*[!0-9:]*|*:*:*) return 1 ;;
  esac
  required_major="${BASH_MIN_VERSION%%.*}"
  required_minor="${BASH_MIN_VERSION#*.}"
  [ "$major" -gt "$required_major" ] || {
    [ "$major" -eq "$required_major" ] && [ "$minor" -ge "$required_minor" ]
  }
}

check_bash_version() {
  local major="${1:-${BASH_VERSINFO[0]}}" minor="${2:-${BASH_VERSINFO[1]}}"
  if ! bash_version_supported "$major" "$minor"; then
    echo "ERROR: Bash ${BASH_MIN_VERSION} or newer is required for the v0.6.1 contributor path; found ${major}.${minor} (${BASH:-unknown}). Install/select a modern Bash and invoke the entrypoint with that interpreter." >&2
    return 1
  fi
  BASH_BIN="${BASH:-$(command -v bash)}"
  export BASH_BIN
}

require_modern_bash() {
  check_bash_version "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"
}

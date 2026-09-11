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

bash_candidate_version() {
  local candidate="$1" version
  version="$("$candidate" -c 'printf "%s.%s\n" "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"' 2>/dev/null)" || return 1
  case "$version" in
    [0-9]*.[0-9]*) printf '%s\n' "$version" ;;
    *) return 1 ;;
  esac
}

select_bash_candidate() {
  local candidate version
  candidate="$1"
  [ -x "$candidate" ] || return 1
  version="$(bash_candidate_version "$candidate")" || return 1
  if check_bash_version "${version%%.*}" "${version#*.}" >/dev/null 2>&1; then
    BASH_BIN="$candidate"
    export BASH_BIN
    return 0
  fi
  return 1
}

resolve_bash() {
  local allow_install="${1:-0}" brew_prefix candidate
  if require_modern_bash >/dev/null 2>&1; then
    return 0
  fi

  if [ "${BOOTSTRAP_OS:-$(uname -s)}" = Darwin ]; then
    for candidate in /opt/homebrew/bin/bash /usr/local/bin/bash; do
      select_bash_candidate "$candidate" && return 0
    done
    if command -v brew >/dev/null 2>&1; then
      brew_prefix="$(brew --prefix bash 2>/dev/null || true)"
      if [ -n "$brew_prefix" ]; then
        select_bash_candidate "$brew_prefix/bin/bash" && return 0
      fi
      if [ "$allow_install" = 1 ]; then
        echo "Bash ${BASH_MIN_VERSION}+ not found; installing the Homebrew bash formula" >&2
        brew install bash
        brew_prefix="$(brew --prefix bash 2>/dev/null || true)"
        select_bash_candidate "$brew_prefix/bin/bash" && return 0
      fi
    fi
  fi

  echo "ERROR: Bash ${BASH_MIN_VERSION} or newer is required for the v0.6.1 contributor path; no supported Bash interpreter was found. Run ./hack/bootstrap.sh with Homebrew available on macOS, or install/select modern Bash explicitly." >&2
  return 1
}

ensure_bash_interpreter() {
  local allow_install="$1" script="$2"
  shift 2
  resolve_bash "$allow_install" || return 1
  if ! require_modern_bash >/dev/null 2>&1; then
    exec "$BASH_BIN" "$script" "$@"
  fi
}

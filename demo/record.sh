#!/usr/bin/env bash
# record.sh — capture the canonical demo as an asciinema cast.
#
# The signature cut is orchestrated end to end: establish a clean workload,
# start asciinema, and run the real canonical scenario. Presentation beats
# remain human-controlled through scenario.sh --paced; pacing never changes
# the product commands or security assertions being demonstrated.
#
# It records real command output and nothing else. There is no post-hoc
# frame editing anywhere in this pipeline — if a take is wrong, reset and
# record it again.
#
#   ./demo/record.sh signature   # full canonical scenario (~4-5 min)
#   ./demo/record.sh hero        # short cut, from an already-approved A

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

CUT="${1:-signature}"
OUT_DIR="${DEMO_ROOT}/recording"
COLS="${DEMO_COLS:-100}"
ROWS="${DEMO_ROWS:-32}"

case "$CUT" in
  signature|hero) ;;
  *) demo_err "unknown cut '${CUT}' (expected: signature | hero)"; exit 2 ;;
esac

demo_stage "Recording: ${CUT}"

demo_require_cmd asciinema kubectl || {
  demo_err "install asciinema first: https://docs.asciinema.org/getting-started/"
  exit 1
}
demo_check_context || exit 1

# A recording made at the wrong terminal size is unusable: the digest lines
# are 71 characters and wrap into noise below ~100 columns.
actual_cols="$(tput cols 2>/dev/null || echo 0)"
if [ "$actual_cols" -lt "$COLS" ]; then
  demo_err "terminal is ${actual_cols} columns; need at least ${COLS}."
  demo_err "Resize, or set DEMO_COLS to override (digest lines will wrap)."
  exit 1
fi

mkdir -p "$OUT_DIR"
CAST="${OUT_DIR}/${CUT}.cast"

if [ -e "$CAST" ]; then
  demo_note "an existing recording will be replaced: ${CAST}"
  printf '  continue? [y/N] '
  read -r answer
  case "$answer" in
    y|Y|yes) ;;
    *) demo_note "aborted; nothing recorded"; exit 0 ;;
  esac
fi

demo_note "recording to ${CAST}"
demo_note "terminal: ${COLS}x${ROWS}"
printf '\n'

if [ "$CUT" = "signature" ]; then
  demo_note "The scenario runs unattended. Two training runs of ${DEMO_DURATION}"
  demo_note "each are real time — speed those segments up in post, with a"
  demo_note "visible caption saying so. Do not shorten --duration silently."
  printf '\n'
  demo_note "Resetting demo state to a fully clean workload..."
  "${DEMO_ROOT}/reset.sh" --recreate-pod >/dev/null
  asciinema rec "$CAST" \
    --cols "$COLS" --rows "$ROWS" \
    --title "landlock-genprof v0.2 — from observed behavior to governed authority" \
    --command "${DEMO_ROOT}/scenario.sh --paced"
else
  demo_note "The hero cut starts from an approved candidate A and shows only:"
  demo_note "  behavior changed -> retrace -> apply refused -> nothing applied -> diff"
  demo_note "Run ./demo/scenario.sh up to stage 6 first, or use --paced and stop there."
  printf '\n'
  demo_note "Recording an interactive shell — run the hero commands from"
  demo_note "demo/script.md, then exit to stop."
  asciinema rec "$CAST" \
    --cols "$COLS" --rows "$ROWS" \
    --title "landlock-genprof v0.2 — approve exactly what you reviewed"
fi

demo_stage "Recorded"
demo_note "cast: ${CAST}"
printf '\n'
demo_note "Render a GIF (see demo/recording.md):"
demo_note "  agg --cols ${COLS} --rows ${ROWS} ${CAST} ${OUT_DIR}/${CUT}.gif"

#!/usr/bin/env bash
# shellcheck disable=SC2317  # seam stubs are invoked indirectly by the sourced helper
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export ENGINE_REPO="$TMP/repo" BIN="$TMP/twiceshy" ENGINE_BUILD_MARKER="$TMP/built-commit"
# shellcheck source=/dev/null
source ./lib/ensure-engine-fresh.sh
set +e

HEAD=abc123
BUILD_RC=0
BUILDS=0
READY=0           # engine_build_ready rc: 0 = on clean main (safe to build), 1 = not
ALERTS=""
LOGS=""
engine_target_commit() { printf '%s\n' "$HEAD"; }
engine_build_ready() { return "$READY"; }
build_engine() { BUILDS=$((BUILDS + 1)); [ "$BUILD_RC" -eq 0 ] && : > "$BIN"; return "$BUILD_RC"; }
alert() { ALERTS="${ALERTS}|$*"; }
log() { LOGS="${LOGS}|$*"; }

# Fresh marker and present binary: no rebuild.
printf '%s\n' "$HEAD" > "$ENGINE_BUILD_MARKER"
: > "$BIN"
ensure_engine_fresh; rc=$?
check "fresh engine returns 0" "$rc" "0"
check "fresh engine is not rebuilt" "$BUILDS" "0"

# Stale marker: rebuild, advance marker, and announce it.
printf '%s\n' old456 > "$ENGINE_BUILD_MARKER"
BUILDS=0; ALERTS=""; LOGS=""
ensure_engine_fresh; rc=$?
check "stale engine returns 0" "$rc" "0"
check "stale engine is rebuilt" "$BUILDS" "1"
check "marker advances after rebuild" "$(cat "$ENGINE_BUILD_MARKER")" "$HEAD"
if contains "$ALERTS$LOGS" "rebuilt engine"; then ok "rebuild is announced"; else bad "rebuild must be announced: $ALERTS$LOGS"; fi

# Failed rebuild: return non-zero, alert, and retain the stale marker.
printf '%s\n' old789 > "$ENGINE_BUILD_MARKER"
BUILD_RC=1; BUILDS=0; ALERTS=""; LOGS=""
ensure_engine_fresh; rc=$?
if [ "$rc" -ne 0 ]; then ok "failed rebuild returns non-zero"; else bad "failed rebuild must return non-zero"; fi
check "failed rebuild was attempted" "$BUILDS" "1"
check "failed rebuild does not advance marker" "$(cat "$ENGINE_BUILD_MARKER")" "old789"
if [ -n "$ALERTS" ]; then ok "failed rebuild alerts"; else bad "failed rebuild must alert"; fi

# Stale binary but checkout NOT on clean main (dev feature branch): refuse loudly,
# never build unmerged code, keep the marker — the deploy stays the operator's call.
printf '%s\n' old999 > "$ENGINE_BUILD_MARKER"
: > "$BIN"
READY=1; BUILD_RC=0; BUILDS=0; ALERTS=""; LOGS=""
ensure_engine_fresh; rc=$?
if [ "$rc" -ne 0 ]; then ok "stale + not-on-main returns non-zero"; else bad "stale + not-on-main must return non-zero"; fi
check "stale + not-on-main does not build" "$BUILDS" "0"
check "stale + not-on-main keeps marker" "$(cat "$ENGINE_BUILD_MARKER")" "old999"
if contains "$ALERTS" "not on clean main"; then ok "stale + not-on-main alerts loudly"; else bad "stale + not-on-main must alert: $ALERTS"; fi
READY=0

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

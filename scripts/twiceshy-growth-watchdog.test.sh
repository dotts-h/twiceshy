#!/usr/bin/env bash
# shellcheck disable=SC2317  # seam stubs are invoked indirectly by the sourced script
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export TWICESHY_GROWTH_STATE="$TMP/growth.tsv" TWICESHY_MAX_STALE_HOURS=12
# shellcheck source=/dev/null
source ./twiceshy-growth-watchdog.sh
set +e

COUNT=10
NOW=$((20 * 3600))
STALE_FILES=""
ALERTS=""
read_validated_count() { printf '%s\n' "$COUNT"; }
now() { printf '%s\n' "$NOW"; }
stale_queue_files() { printf '%s' "$STALE_FILES"; }
timer_is_healthy() { return 0; }
alert() { ALERTS="${ALERTS}|$*"; }
log() { :; }
reset() { rm -f "$STATE"; STALE_FILES=""; ALERTS=""; }

# A stale retro payload is a dead capture path and must alert.
reset; STALE_FILES="/queue/old.json"
main >/dev/null; rc=$?
if [ "$rc" -eq 0 ]; then ok "stale queue check completes"; else bad "stale queue check returned $rc"; fi
if contains "$ALERTS" "queue"; then ok "stale queue alerts"; else bad "stale queue must alert: $ALERTS"; fi

# Empty/all-fresh queues produce no queue alert.
reset
main >/dev/null
if contains "$ALERTS" "queue"; then bad "fresh queue must not alert: $ALERTS"; else ok "fresh queue does not alert"; fi

# The retro timer belongs to the core watched set.
if contains " ${TIMERS[*]} " " twiceshy-retro.timer "; then ok "retro timer is watched"; else bad "retro timer missing: ${TIMERS[*]}"; fi

# Existing growth behavior: growth is healthy; a long-flat count alerts.
reset; printf '9\t0\n' > "$STATE"
main >/dev/null
if [ -z "$ALERTS" ]; then ok "validated growth does not alert"; else bad "growth must not alert: $ALERTS"; fi

reset; printf '10\t0\n' > "$STATE"
main >/dev/null
if contains "$ALERTS" "NOT growing"; then ok "flat count past threshold alerts"; else bad "flat count must alert: $ALERTS"; fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

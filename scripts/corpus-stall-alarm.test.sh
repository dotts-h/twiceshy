#!/usr/bin/env bash
# shellcheck disable=SC2317  # stub fns (list_pipeline_prs/now/notify) are invoked indirectly by the sourced script
# Tests for corpus-stall-alarm.sh — sources the script (source-guard stops main) and
# stubs the seams. The alarm is the "never silent again" guard (#0072): it fires when
# an import/* or validate/* PR sits open past a threshold (the 12h-freeze pattern) or
# is left open-and-red. Run: bash corpus-stall-alarm.test.sh
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

export TWICESHY_STALL_THRESHOLD_MIN=60 TWICESHY_STALL_COOLDOWN=600
STATE_FILE="$(mktemp)"; rm -f "$STATE_FILE"
export TWICESHY_STALL_STATE="$STATE_FILE"
# shellcheck source=/dev/null
source ./corpus-stall-alarm.sh
set +e

PRS=""             # one PR per line: number|branch|age_min|ci_state
NOW=1000000        # fixed clock (seconds)
ALERTS=""
reset() { PRS=""; ALERTS=""; rm -f "$STATE_FILE"; }

list_pipeline_prs() { printf '%s' "$PRS"; }
now() { echo "$NOW"; }
notify() { ALERTS="${ALERTS}|$1"; }

# ---- no open pipeline PRs: healthy, no alarm -------------------------------------
reset
main; rc=$?
check "no PRs returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "no PRs => no alarm"; else bad "no PRs must not alarm: $ALERTS"; fi

# ---- a young green PR: healthy, it will merge soon -------------------------------
reset; PRS="312|validate/run-x|30|success"
main; rc=$?
check "young green PR returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "young green PR => no alarm"; else bad "young green PR must not alarm: $ALERTS"; fi

# ---- an OLD PR (open past threshold): the freeze pattern -> alarm ----------------
reset; PRS="309|validate/run-y|180|success"
main; rc=$?
check "stalled PR returns non-zero" "$rc" "1"
if contains "$ALERTS" "309"; then ok "stalled PR is alarmed (by number)"; else bad "stalled PR must alarm: $ALERTS"; fi

# ---- a RED PR (failing CI), even if young -> alarm ------------------------------
reset; PRS="301|import/osv-live-z|10|failure"
main; rc=$?
check "red PR returns non-zero" "$rc" "1"
if contains "$ALERTS" "301"; then ok "open-and-red PR is alarmed"; else bad "red PR must alarm: $ALERTS"; fi

# ---- many stalled PRs (the real backlog) -> one alarm naming the count ----------
reset; PRS="$(printf '309|validate/a|180|success\n301|validate/b|200|failure\n296|validate/c|220|success')"
main; rc=$?
check "backlog returns non-zero" "$rc" "1"
if contains "$ALERTS" "3"; then ok "backlog alarm reports the count"; else bad "backlog alarm must report count: $ALERTS"; fi

# ---- cooldown: a second tick within the window does NOT re-alarm -----------------
reset; PRS="309|validate/run-y|180|success"
main >/dev/null 2>&1            # first alarm, writes cooldown state
ALERTS=""
main; rc=$?
check "still-stalled tick returns non-zero" "$rc" "1"
if [ -z "$ALERTS" ]; then ok "within cooldown => no re-alarm (no spam)"; else bad "must not re-alarm within cooldown: $ALERTS"; fi

# ---- cooldown expires: re-alarm so a persistent stall stays visible --------------
NOW=$((NOW + TWICESHY_STALL_COOLDOWN + 1))
ALERTS=""
main; rc=$?
if contains "$ALERTS" "309"; then ok "re-alarms after cooldown"; else bad "must re-alarm after cooldown: $ALERTS"; fi
NOW=1000000

# ---- recovery: the stall clears -> healthy, no alarm ----------------------------
reset; PRS="309|validate/run-y|180|success"
main >/dev/null 2>&1            # alarmed, cooldown set
PRS=""; ALERTS=""               # backlog drained
main; rc=$?
check "drained backlog returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "drained backlog => no alarm"; else bad "drained backlog must not alarm: $ALERTS"; fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

#!/usr/bin/env bash
# shellcheck disable=SC2317  # stub fns (ssh_nas/alert) are invoked indirectly by the sourced script
# Tests for sync-decisions-from-nas.sh — sources the script (its source-guard stops
# main from running) and stubs the seams (ssh_nas/alert) to drive the pull logic.
# No real ssh/docker. Run: bash sync-decisions-from-nas.test.sh
#
# What this gate locks (#0098): the brain-side NAS→brain pull of the serve's #0067
# decision log MUST (1) read the log via the volume-cat idiom (uid-65532 safe), write
# it atomically to the brain-local path; (2) on a fetch failure NEVER clobber a good
# local copy with nothing, and alert; (3) leave no .tmp turd behind on failure.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

# Drive a fresh local path per run; the volume name is the prod default we assert on.
LOCAL_DIR="$(mktemp -d)"
export TWICESHY_DECISIONS_LOCAL="$LOCAL_DIR/gate-decisions.jsonl"
export TWICESHY_VOLUME="twiceshy-data"
export TWICESHY_DECISIONS_REMOTE="/data/gate-decisions.jsonl"

# shellcheck source=/dev/null
source ./sync-decisions-from-nas.sh
set +e  # the script enabled -e on source; the harness drives explicitly

# ---- stub seams ------------------------------------------------------------------
SSH_OUT=""; SSH_RC=0; SSH_LOG=""; ALERTS=""
ssh_nas() { SSH_LOG="${SSH_LOG}::$*"; printf '%s' "$SSH_OUT"; return "$SSH_RC"; }
alert()   { ALERTS="${ALERTS}|$1"; }
reset()   { SSH_OUT=""; SSH_RC=0; SSH_LOG=""; ALERTS=""; rm -f "$TWICESHY_DECISIONS_LOCAL" "$TWICESHY_DECISIONS_LOCAL".tmp* 2>/dev/null; }

# ---- happy path: fetch rc 0 → log written atomically, content exact ---------------
reset
SSH_OUT=$'{"a":1}\n{"b":2}'; SSH_RC=0
sync_decisions; rc=$?
check "happy path returns 0" "$rc" "0"
if [ -f "$TWICESHY_DECISIONS_LOCAL" ]; then ok "local log written"; else bad "local log must be written"; fi
check "local content is exactly the fetched bytes" "$(cat "$TWICESHY_DECISIONS_LOCAL")" "$SSH_OUT"
if contains "$SSH_LOG" "docker run --rm -v twiceshy-data:/data"; then ok "reads via the volume-cat idiom (uid-65532 safe)"; else bad "must read the log via 'docker run --rm -v twiceshy-data:/data alpine cat'"; fi
if contains "$SSH_LOG" "/data/gate-decisions.jsonl"; then ok "reads the configured remote log path"; else bad "must cat the configured remote path"; fi
if ls "$TWICESHY_DECISIONS_LOCAL".tmp* >/dev/null 2>&1; then bad "no .tmp may remain after success"; else ok "no .tmp turd after success"; fi

# ---- fetch failure: rc nonzero → preserve the good local copy, alert, no clobber --
reset
printf 'GOOD-EXISTING' > "$TWICESHY_DECISIONS_LOCAL"
SSH_OUT=""; SSH_RC=1
sync_decisions; rc=$?
if [ "$rc" -ne 0 ]; then ok "fetch failure returns nonzero"; else bad "fetch failure must return nonzero"; fi
check "good local copy is preserved on fetch failure" "$(cat "$TWICESHY_DECISIONS_LOCAL")" "GOOD-EXISTING"
if [ -n "$ALERTS" ]; then ok "fetch failure alerts"; else bad "fetch failure must alert"; fi
if ls "$TWICESHY_DECISIONS_LOCAL".tmp* >/dev/null 2>&1; then bad "no .tmp may remain after failure"; else ok "no .tmp turd after failure"; fi

# ---- empty-but-ok remote (rc 0, no records yet) is a legitimate write -------------
reset
SSH_OUT=""; SSH_RC=0
sync_decisions; rc=$?
check "empty-but-ok fetch returns 0" "$rc" "0"
if [ -f "$TWICESHY_DECISIONS_LOCAL" ]; then ok "empty-but-ok writes an (empty) local log"; else bad "rc 0 must write even when empty"; fi

rm -rf "$LOCAL_DIR"
echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

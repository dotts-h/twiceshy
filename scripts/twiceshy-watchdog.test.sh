#!/usr/bin/env bash
# shellcheck disable=SC2317  # stub fns (ssh_nas/health_probe) are invoked indirectly by the sourced script
# Tests for twiceshy-watchdog.sh — sources the script (source-guard stops main) and
# stubs the seams. Real `date`, HEALTH_TIMEOUT=0 so healthz_wait returns on the first
# probe; the breaker is driven by writing real timestamps. Run: bash twiceshy-watchdog.test.sh
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

export TWICESHY_HEALTH_TIMEOUT=0 TWICESHY_POLL_SLEEP=0
BREAKER_FILE="$(mktemp)"; rm -f "$BREAKER_FILE"
export TWICESHY_WATCHDOG_BREAKER="$BREAKER_FILE"
# shellcheck source=/dev/null
source ./twiceshy-watchdog.sh
set +e

HEALTHY=0          # 0 = /healthz 200, 1 = down
START_HEALS=1      # does `docker start` bring it back to healthy?
SSH_LOG=""
ALERTS=""
reset() { HEALTHY=0; START_HEALS=1; SSH_LOG=""; ALERTS=""; rm -f "$BREAKER_FILE"; }

health_probe() { return "$HEALTHY"; }
alert() { ALERTS="${ALERTS}|$1"; }
ssh_nas() {
	local cmd="$*"; SSH_LOG="${SSH_LOG}::${cmd}"
	case "$cmd" in
		*"docker start"*) [ "$START_HEALS" = "1" ] && HEALTHY=0 ;;
		*"ExitCode"*) echo 2 ;;
		*) : ;;
	esac
}

# ---- healthy tick does nothing ---------------------------------------------------
reset
main; rc=$?
check "healthy tick returns 0" "$rc" "0"
if contains "$SSH_LOG" "docker start"; then bad "healthy tick must not restart"; else ok "healthy tick does not restart"; fi

# ---- down + recoverable: restart, recover, alert that a drop happened ------------
reset; HEALTHY=1; START_HEALS=1
main; rc=$?
check "down+recoverable returns 0" "$rc" "0"
if contains "$SSH_LOG" "docker start"; then ok "down container is restarted"; else bad "down container must be restarted"; fi
if contains "$ALERTS" "now healthy"; then ok "recovery is alerted (drop made visible)"; else bad "a recovery must alert"; fi

# ---- down + restart fails: alert, set breaker ------------------------------------
reset; HEALTHY=1; START_HEALS=0
main; rc=$?
check "unrecoverable down returns 1" "$rc" "1"
if contains "$ALERTS" "did NOT recover"; then ok "failed recovery alerts"; else bad "failed recovery must alert"; fi

# ---- crash-loop: down within cooldown does NOT restart and does NOT re-alert ------
reset; HEALTHY=1; START_HEALS=0
now > "$BREAKER_FILE"   # a restart just happened
main; rc=$?
check "crash-loop (within cooldown) returns 1" "$rc" "1"
if contains "$SSH_LOG" "docker start"; then bad "breaker must suppress the restart"; else ok "breaker suppresses restart-storm"; fi
if [ -z "$ALERTS" ]; then ok "crash-loop tick does not re-alert (log only)"; else bad "crash-loop tick must not re-alert: $ALERTS"; fi

# ---- breaker resets after cooldown -----------------------------------------------
reset; HEALTHY=1; START_HEALS=0
echo "$(( $(now) - BREAKER_COOLDOWN - 10 ))" > "$BREAKER_FILE"
main >/dev/null 2>&1
if contains "$SSH_LOG" "docker start"; then ok "breaker resets after cooldown"; else bad "after cooldown a new down must restart"; fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

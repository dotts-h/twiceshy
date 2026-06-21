#!/usr/bin/env bash
# shellcheck disable=SC2317  # stub fns (ssh_nas/git/health_probe) are invoked indirectly by the sourced script
# Tests for sync-corpus-to-nas.sh — sources the script (its source-guard stops main
# from running) and stubs the seams (ssh_nas/health_probe/git) to drive the
# self-healing logic. No real ssh/docker/curl. Run: bash sync-corpus-to-nas.test.sh
#
# Time is real `date +%s`, with HEALTH_TIMEOUT=0 so healthz_wait returns on the first
# probe (no spinning, no flaky sleeps); the circuit breaker is driven by writing real
# timestamps into its file.
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
export TWICESHY_BREAKER_FILE="$BREAKER_FILE"
# shellcheck source=/dev/null
source ./sync-corpus-to-nas.sh
set +e # the script enabled -e on source; the harness drives explicitly

# ---- stub state ------------------------------------------------------------------
RUNNING=true       # docker State.Running
HEALTHY=0          # 0 = /healthz 200, 1 = down
START_HEALS=1      # does `docker start` bring it back to healthy?
SSH_LOG=""
ALERTS=""
MARKER_STAMPED=0
reset() { RUNNING=true; HEALTHY=0; START_HEALS=1; SSH_LOG=""; ALERTS=""; MARKER_STAMPED=0; rm -f "$BREAKER_FILE"; }

health_probe() { return "$HEALTHY"; }
alert() { ALERTS="${ALERTS}|$1"; }
git() {
	case "$*" in
		*"rev-parse origin/main:experience"*) echo "NEWSHA" ;;
		*"archive"*) printf 'TARBYTES' ;;
		*) : ;;
	esac
}
# Default ssh_nas: a `docker start` heals iff START_HEALS=1; HUP leaves state as-is
# (each main scenario overrides HUP behavior). Marker reads return OLDSHA.
ssh_nas() {
	local cmd="$*"; SSH_LOG="${SSH_LOG}::${cmd}"
	case "$cmd" in
		*"State.Running"*) echo "$RUNNING" ;;
		*"docker start"*) [ "$START_HEALS" = "1" ] && { RUNNING=true; HEALTHY=0; } ;;
		*".experience-tree-sha 2>/dev/null"*|*"cat /data/corpus/.experience-tree-sha"*) echo "OLDSHA" ;;
		*"printf %s"*) MARKER_STAMPED=1 ;;
		*"ExitCode"*) echo 2 ;;
		*"docker logs"*) echo boom ;;
		*) : ;;
	esac
}

# ---- ensure_healthy: happy path does nothing -------------------------------------
reset
ensure_healthy; rc=$?
check "healthy tick returns 0" "$rc" "0"
if contains "$SSH_LOG" "docker start"; then bad "healthy tick must not restart"; else ok "healthy tick does not restart"; fi

# ---- ensure_healthy: down → recover (restart heals it) ---------------------------
reset; RUNNING=false; HEALTHY=1; START_HEALS=1
ensure_healthy; rc=$?
check "down+recoverable returns 0" "$rc" "0"
if contains "$SSH_LOG" "docker start"; then ok "down container is restarted"; else bad "down container must be restarted"; fi

# ---- circuit breaker: a death within cooldown of a restart does NOT restart ------
reset; RUNNING=false; HEALTHY=1; START_HEALS=0   # restart will NOT heal (stays down)
ensure_healthy; rc1=$?                            # 1st: writes a recent breaker ts, restarts, stays down → alert+1
SSH_LOG=""; ALERTS=""                             # observe only the 2nd tick
ensure_healthy; rc2=$?                            # 2nd: breaker ts is recent → must NOT restart
check "1st unrecoverable down returns 1" "$rc1" "1"
check "2nd down within cooldown returns 1" "$rc2" "1"
if contains "$SSH_LOG" "docker start"; then bad "breaker must suppress the 2nd restart"; else ok "breaker suppresses restart-storm"; fi
if contains "$ALERTS" "crash-loop"; then ok "breaker alerts crash-loop"; else bad "breaker must alert crash-loop"; fi

# ---- breaker resets after cooldown: an old breaker ts allows a restart again -----
reset; RUNNING=false; HEALTHY=1; START_HEALS=0
echo "$(( $(date +%s) - BREAKER_COOLDOWN - 10 ))" > "$BREAKER_FILE"  # last restart was long ago (script's BREAKER_COOLDOWN)
ensure_healthy >/dev/null 2>&1
if contains "$SSH_LOG" "docker start"; then ok "breaker resets after cooldown"; else bad "after cooldown a new death must restart"; fi

# ---- set -e safety: a failing health_probe must not abort the script -------------
reset; RUNNING=true; HEALTHY=1; START_HEALS=1
( ensure_healthy ) >/dev/null 2>&1
check "failing probe does not abort (set -e safe)" "$?" "0"

# ---- main: reload that KILLS the old binary recovers AND stamps the marker -------
reset
ssh_nas() {
	local cmd="$*"; SSH_LOG="${SSH_LOG}::${cmd}"
	case "$cmd" in
		*"State.Running"*) echo "$RUNNING" ;;
		*"docker kill -s HUP"*) RUNNING=false; HEALTHY=1 ;;   # old binary dies on HUP
		*"docker start"*) RUNNING=true; HEALTHY=0 ;;          # restart heals it (good corpus)
		*".experience-tree-sha 2>/dev/null"*|*"cat /data/corpus/.experience-tree-sha"*) echo "OLDSHA" ;;
		*"printf %s"*) MARKER_STAMPED=1 ;;
		*) : ;;
	esac
}
main >/dev/null 2>&1; rc=$?
check "reload-kills-old-binary main exits 0" "$rc" "0"
check "marker stamped after verified reload" "$MARKER_STAMPED" "1"
if contains "$SSH_LOG" "docker start"; then ok "old binary recovered via restart"; else bad "old binary must be restarted after HUP kill"; fi

# ---- main: reload that leaves it DOWN (unbuildable corpus) must NOT stamp marker --
reset; RUNNING=true; HEALTHY=0
ssh_nas() {
	local cmd="$*"; SSH_LOG="${SSH_LOG}::${cmd}"
	case "$cmd" in
		*"State.Running"*) echo "$RUNNING" ;;
		*"docker kill -s HUP"*) RUNNING=false; HEALTHY=1 ;;
		*"docker start"*) : ;;                                # restart does NOT heal (bad corpus)
		*".experience-tree-sha 2>/dev/null"*|*"cat /data/corpus/.experience-tree-sha"*) echo "OLDSHA" ;;
		*"printf %s"*) MARKER_STAMPED=1 ;;
		*"ExitCode"*) echo 2 ;;
		*"docker logs"*) echo boom ;;
		*) : ;;
	esac
}
main >/dev/null 2>&1; rc=$?
check "failed reload exits nonzero" "$rc" "1"
check "marker NOT stamped on failed reload" "$MARKER_STAMPED" "0"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

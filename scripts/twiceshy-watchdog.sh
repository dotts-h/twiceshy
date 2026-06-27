#!/usr/bin/env bash
# twiceshy-watchdog.sh — keep the twiceshy MCP service up, INDEPENDENT of corpus-sync.
#
# The SIGHUP deploy outage showed the only recovery path was the corpus-sync timer,
# and a signal-kill bypassed the serve-fatal alert — so a drop could be both
# unrecovered (if corpus-sync was wedged) and silent. This is the decoupled safety
# net: every tick it probes /healthz; if the service is down it restarts the
# container and ALERTS (a successful recovery means a drop DID happen — worth
# knowing). A genuine crash-loop trips a breaker → one alert, no restart-storm.
#
# Runs on the brain as `ori` (probes the NAS over the LAN, `docker start` via ssh).
# Idempotent and side-effect-light: a healthy tick does nothing.
set -uo pipefail

HEALTH_URL="${TWICESHY_HEALTH_URL:-http://192.168.50.244:8722/healthz}"
NAS="${TWICESHY_NAS:-Claude@192.168.50.244}"
NAS_PORT="${TWICESHY_NAS_PORT:-2222}"
CONTAINER="${TWICESHY_CONTAINER:-twiceshy}"
ALERT_URL="${TWICESHY_ALERT_URL:-}"                   # ntfy topic; unset = log-only
NTFY_TOKEN="${NTFY_TOKEN:-}"
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}" # non-root cannot write /run (see corpus-sync)
BREAKER_FILE="${TWICESHY_WATCHDOG_BREAKER:-$RUNDIR/twiceshy-watchdog.breaker}"
BREAKER_COOLDOWN="${TWICESHY_WATCHDOG_COOLDOWN:-600}" # crash-loop guard: don't restart faster than this
HEALTH_TIMEOUT="${TWICESHY_HEALTH_TIMEOUT:-25}"       # seconds to wait for /healthz after a restart
POLL_SLEEP="${TWICESHY_POLL_SLEEP:-2}"

# --- seams (overridable in tests) -------------------------------------------------
ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=8 "$NAS" "$@"; }
now() { date +%s; }
health_probe() { curl -fsS -m 6 "$HEALTH_URL" >/dev/null 2>&1; }
alert() {
	logger -t twiceshy-watchdog "$1" 2>/dev/null || true
	# Discover the alert channel from the container's own env (same ntfy topic the
	# service uses) so no secret lives in git. `docker inspect` works on a stopped
	# container too, so this resolves even when we are alerting about a down service.
	if [ -z "$ALERT_URL" ]; then
		ALERT_URL="$(ssh_nas "docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $CONTAINER" 2>/dev/null | sed -n 's/^TWICESHY_ALERT_URL=//p' | head -1)"
	fi
	[ -n "$ALERT_URL" ] || return 0
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "twiceshy-watchdog: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# healthz_wait: poll health_probe until success or HEALTH_TIMEOUT elapses.
healthz_wait() {
	local deadline
	deadline="$(( $(now) + HEALTH_TIMEOUT ))"
	while :; do
		if health_probe; then return 0; fi
		[ "$(now)" -ge "$deadline" ] && return 1
		sleep "$POLL_SLEEP"
	done
}

main() {
	# Fast path: serving → nothing to do.
	if health_probe; then
		return 0
	fi

	# Down. Crash-loop breaker: if we restarted within the cooldown and it is down
	# AGAIN, restarting won't help — log and bail WITHOUT re-alerting (we already
	# alerted when the breaker was set; it re-alerts once per cooldown if still broken).
	local last=0
	[ -f "$BREAKER_FILE" ] && last="$(cat "$BREAKER_FILE" 2>/dev/null || echo 0)"
	if [ "$(( $(now) - last ))" -lt "$BREAKER_COOLDOWN" ]; then
		logger -t twiceshy-watchdog "$CONTAINER still down within ${BREAKER_COOLDOWN}s of a restart — crash-loop, not restarting" 2>/dev/null || true
		return 1
	fi
	now > "$BREAKER_FILE" 2>/dev/null || true

	logger -t twiceshy-watchdog "$CONTAINER down at $HEALTH_URL — restarting" 2>/dev/null || true
	ssh_nas "docker start $CONTAINER >/dev/null 2>&1" || true
	if healthz_wait; then
		# A drop happened and we recovered it — make it visible (this is the alert a
		# signal-kill bypassed during the outage).
		alert "$CONTAINER was DOWN and was auto-restarted — now healthy"
		return 0
	fi
	alert "$CONTAINER is DOWN and did NOT recover after a restart — manual attention needed ($(ssh_nas "docker inspect -f '{{.State.ExitCode}}' $CONTAINER" 2>/dev/null || echo 'exit?'))"
	return 1
}

# Run main only when executed, not when sourced (the test harness sources this file
# to stub the seams). The script exits with main's status (it is the last command).
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	main "$@"
fi

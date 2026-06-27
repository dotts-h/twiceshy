#!/usr/bin/env bash
# sync-corpus-to-nas.sh — keep the twiceshy NAS service HEALTHY and its corpus in
# sync with origin/main, on every timer tick (#0060). Self-healing by design.
#
# Ordering of a tick:
#   0. ensure_healthy — recover a DOWN/killed container BEFORE anything else. This
#      runs every tick (not only on a corpus change), so an outage is bounded to one
#      timer interval, never hours.
#   1. change-gate on the experience tree SHA (no-op if the volume already matches).
#   2. mirror origin/main:experience to the volume (wholesale, idempotent) — but do
#      NOT advance the marker yet.
#   3. reload in place via SIGHUP, then VERIFY liveness; recover if the signal killed
#      the process (a binary with no SIGHUP handler dies — Go default-terminates —
#      and `restart=unless-stopped` does NOT restart a *killed* container; a restart
#      rebuilds from the now-updated volume, so it is the correct fallback).
#   4. only after a verified-healthy reload, stamp the marker. A failed reload leaves
#      the marker old → the next tick re-mirrors and retries (no silent stuck state).
#
# This exists because an earlier change sent `docker kill -s HUP` to a binary
# without a SIGHUP handler and the service stayed down for hours, unalerted and
# un-restarted. Now: liveness is checked every tick, a one-off kill is recovered, a
# genuine crash-loop trips a circuit breaker and alerts instead of restarting forever.
#
# Tunables (env): TWICESHY_REPO, TWICESHY_NAS, TWICESHY_NAS_PORT, TWICESHY_VOLUME,
# TWICESHY_CONTAINER, TWICESHY_UID, TWICESHY_HEALTH_URL, TWICESHY_ALERT_URL (ntfy;
# unset = no-op), TWICESHY_HEALTH_TIMEOUT, TWICESHY_BREAKER_FILE, TWICESHY_BREAKER_COOLDOWN.
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-import}"
NAS="${TWICESHY_NAS:-Claude@192.168.50.244}"
NAS_PORT="${TWICESHY_NAS_PORT:-2222}"
VOL="${TWICESHY_VOLUME:-twiceshy-data}"
CONTAINER="${TWICESHY_CONTAINER:-twiceshy}"
NONROOT_UID="${TWICESHY_UID:-65532}"          # distroless nonroot (DEPLOY.md)
HEALTH_URL="${TWICESHY_HEALTH_URL:-http://192.168.50.244:8722/healthz}"
ALERT_URL="${TWICESHY_ALERT_URL:-}"           # ntfy topic; unset = no-op (fail-open)
NTFY_TOKEN="${NTFY_TOKEN:-}"
HEALTH_TIMEOUT="${TWICESHY_HEALTH_TIMEOUT:-30}"     # seconds to wait for /healthz after a reload/start
POLL_SLEEP="${TWICESHY_POLL_SLEEP:-2}"               # seconds between health polls
# State files default to a per-user-writable dir: the timer runs as a non-root user
# (User=ori) that cannot write /run, so /run/* silently failed — the breaker never
# persisted and the lock could not open. XDG_RUNTIME_DIR (/run/user/UID) when set, else /tmp.
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"
BREAKER_FILE="${TWICESHY_BREAKER_FILE:-$RUNDIR/twiceshy-corpus-sync.breaker}"
BREAKER_COOLDOWN="${TWICESHY_BREAKER_COOLDOWN:-900}" # an UNPROVOKED death within this window of a restart = crash-loop

# --- seams (overridable in tests) -------------------------------------------------

ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$NAS" "$@"; }

# now: current unix time. A seam so the breaker is testable without sleeping.
now() { date +%s; }

# health_probe: 0 iff GET HEALTH_URL returns 2xx. Never aborts the script.
health_probe() { curl -fsS -m 5 "$HEALTH_URL" >/dev/null 2>&1; }

# alert: log + best-effort ntfy. Never aborts (fail-open), mirrors scheduled-validate.sh.
alert() {
	echo "ALERT: $1" >&2
	[ -n "$ALERT_URL" ] || return 0
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "twiceshy-corpus-sync: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# --- liveness ---------------------------------------------------------------------

# container_running: 0 iff docker reports State.Running=true. Never aborts.
container_running() {
	local s
	s="$(ssh_nas "docker inspect -f '{{.State.Running}}' $CONTAINER" 2>/dev/null || echo false)"
	[ "$s" = "true" ]
}

# healthz_wait: poll health_probe until it succeeds or HEALTH_TIMEOUT elapses.
# /healthz is liveness (process serving HTTP), so it is the right post-reload signal
# — it does not flap while the index rebuilds the way /readyz can.
healthz_wait() {
	local deadline
	deadline="$(( $(now) + HEALTH_TIMEOUT ))"
	while :; do
		if health_probe; then return 0; fi
		[ "$(now)" -ge "$deadline" ] && return 1
		sleep "$POLL_SLEEP"
	done
}

# _diag: a short one-line diagnostic for an alert (exit code + last log lines).
_diag() {
	local ec logs
	ec="$(ssh_nas "docker inspect -f '{{.State.ExitCode}}' $CONTAINER" 2>/dev/null || echo '?')"
	logs="$(ssh_nas "docker logs --tail 5 $CONTAINER 2>&1" 2>/dev/null | tr '\n' '|' || true)"
	echo "exit=$ec logs=[$logs]"
}

# restart_container: docker start + wait for healthy. Returns 0 if healthy after.
# No breaker logic — the caller decides whether the death was expected.
restart_container() {
	echo "restart: starting $CONTAINER"
	ssh_nas "docker start $CONTAINER >/dev/null 2>&1" || true
	healthz_wait
}

# ensure_healthy: the per-tick safety net. If the service is up and serving, return 0.
# If it is DOWN (an UNPROVOKED death — we did not signal it), recover it behind a
# circuit breaker: a death within BREAKER_COOLDOWN of our last restart is a crash-loop
# (e.g. an unbuildable corpus), so we alert and refuse to restart-storm. Returns 0 if
# healthy, 1 if down (already alerted).
ensure_healthy() {
	if container_running && health_probe; then
		return 0
	fi
	local last=0
	[ -f "$BREAKER_FILE" ] && last="$(cat "$BREAKER_FILE" 2>/dev/null || echo 0)"
	if [ "$(( $(now) - last ))" -lt "$BREAKER_COOLDOWN" ]; then
		alert "$CONTAINER down again within ${BREAKER_COOLDOWN}s of a restart — likely crash-loop, NOT auto-restarting. $(_diag)"
		return 1
	fi
	now > "$BREAKER_FILE" 2>/dev/null || true
	if restart_container; then
		echo "ensure_healthy: $CONTAINER recovered"
		return 0
	fi
	alert "$CONTAINER did not become healthy after restart (corpus may be unbuildable). $(_diag)"
	return 1
}

# --- corpus sync ------------------------------------------------------------------

main() {
	# 0) Safety net first: recover a down service regardless of corpus state.
	if ! ensure_healthy; then
		return 1 # down and unrecoverable (alerted); do not sync onto a dead service.
	fi

	# 1) Change-gate: compare main's experience tree SHA to the volume marker.
	git -C "$REPO" fetch -q origin main
	local new_sha cur_sha
	new_sha="$(git -C "$REPO" rev-parse origin/main:experience)"
	cur_sha="$(ssh_nas "docker run --rm -v $VOL:/data alpine cat /data/corpus/.experience-tree-sha 2>/dev/null" 2>/dev/null || true)"
	if [ "$new_sha" = "$cur_sha" ]; then
		echo "corpus up to date ($new_sha)"
		return 0
	fi

	# 2) Mirror origin/main:experience to the volume (wholesale = no orphan/colliding
	#    records, ADR-0001 §1). Marker is NOT stamped here — only a verified reload
	#    advances it, so a failed reload retries on the next tick.
	echo "syncing corpus ${cur_sha:-<none>} -> $new_sha"
	git -C "$REPO" archive --format=tar origin/main experience | ssh_nas \
		"docker run --rm -i -v $VOL:/data alpine sh -c '
		   rm -rf /data/corpus/experience &&
		   mkdir -p /data/corpus &&
		   tar xf - -C /data/corpus &&
		   chown -R $NONROOT_UID:$NONROOT_UID /data/corpus'"

	# 3) Reload in place via SIGHUP, then VERIFY. A binary that handles SIGHUP
	#    hot-reloads and stays up; one that does not dies — so if it goes down, the
	#    death is EXPECTED: recover it (a restart rebuilds from the updated volume =
	#    same new corpus loaded). recover failing here = the new corpus is unbuildable.
	ssh_nas "docker kill -s HUP $CONTAINER >/dev/null 2>&1" || true
	if ! healthz_wait; then
		echo "reload: $CONTAINER not healthy after SIGHUP; recovering (restart fallback)"
		if ! restart_container; then
			alert "$CONTAINER down after corpus reload to $new_sha and restart failed — corpus likely unbuildable; marker NOT advanced (will retry). $(_diag)"
			return 1
		fi
	fi

	# 4) Verified healthy on the new corpus → advance the marker.
	if ! ssh_nas "docker run --rm -v $VOL:/data alpine sh -c 'printf %s \"$new_sha\" > /data/corpus/.experience-tree-sha'"; then
		alert "reload of $CONTAINER to $new_sha succeeded but stamping the marker failed (will re-mirror next tick)"
		return 1
	fi
	echo "synced + reloaded $CONTAINER at corpus $new_sha"
}

# Run main only when executed, not when sourced (the test harness sources this file
# to stub the seams). The flock prevents overlapping ticks but is FAIL-OPEN: if the
# lock file cannot be opened, proceed without it rather than skip the sync (an
# unwritable lock path must never silently disable syncing, as /run/ did).
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	if { exec 9>"${TWICESHY_LOCK_FILE:-$RUNDIR/twiceshy-corpus-sync.lock}"; } 2>/dev/null; then
		flock -n 9 || { echo "another sync is already running; skipping"; exit 0; }
	else
		echo "warning: lock unavailable; proceeding without overlap lock" >&2
	fi
	main "$@"
fi

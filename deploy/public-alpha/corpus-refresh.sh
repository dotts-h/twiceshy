#!/usr/bin/env bash
# corpus-refresh.sh — public-alpha corpus refresh (#0129, ADR-0030 blast-radius
# rule): pull-only, one-way sync from the PUBLIC corpus repo (git.radulescu.app,
# already public via CF tunnel) into the running twiceshy-data volume, then
# hot-reload via SIGHUP (#0060) so a corpus update never blips the service. No
# SSH, no LAN reachability — the only network egress this script needs is the
# HTTPS clone/fetch below.
#
# Ordering of a tick (mirrors scripts/sync-corpus-to-nas.sh, adapted local-only
# — this host runs docker directly, no ssh_nas hop):
#   1. change-gate on the experience tree SHA (no-op if the volume already matches).
#   2. mirror the corpus repo's experience/ tree into the volume (wholesale,
#      idempotent) — but do NOT advance the marker yet.
#   3. reload in place via SIGHUP, then VERIFY container health. A binary with
#      no SIGHUP handler dies (Go default-terminates) and `restart:
#      unless-stopped` does NOT restart a *killed* container — so a reload that
#      does not come back healthy falls back to `docker start`, which rebuilds
#      the index from the now-updated volume on boot.
#   4. only a verified-healthy reload advances the marker; a failed one leaves
#      it stale so the next tick re-mirrors and retries (no silent stuck state).
#
# Tunables (env): TWICESHY_CORPUS_CLONE, TWICESHY_CORPUS_REPO, TWICESHY_VOLUME,
# TWICESHY_CONTAINER, TWICESHY_UID, TWICESHY_ALERT_URL (ntfy; unset = no-op),
# NTFY_TOKEN, TWICESHY_HEALTH_TIMEOUT, TWICESHY_POLL_SLEEP, TWICESHY_RUNDIR,
# TWICESHY_LOCK_FILE.
set -euo pipefail

CORPUS_CLONE="${TWICESHY_CORPUS_CLONE:-/opt/twiceshy-public-alpha/corpus-src}"
CORPUS_REPO="${TWICESHY_CORPUS_REPO:-https://git.radulescu.app/claude/twiceshy-corpus.git}"
VOL="${TWICESHY_VOLUME:-twiceshy-data}"
CONTAINER="${TWICESHY_CONTAINER:-twiceshy}"
NONROOT_UID="${TWICESHY_UID:-65532}"          # distroless nonroot (DEPLOY.md)
ALERT_URL="${TWICESHY_ALERT_URL:-}"           # ntfy topic; unset = no-op (fail-open)
NTFY_TOKEN="${NTFY_TOKEN:-}"
HEALTH_TIMEOUT="${TWICESHY_HEALTH_TIMEOUT:-30}" # seconds to wait for a healthy container after reload/start
POLL_SLEEP="${TWICESHY_POLL_SLEEP:-2}"          # seconds between health polls
# Non-root cannot write /run (see scripts/sync-corpus-to-nas.sh) — prefer
# XDG_RUNTIME_DIR, else /tmp.
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"

# --- seams -------------------------------------------------------------------

now() { date +%s; }

# alert: log + best-effort ntfy. Never aborts (fail-open), mirrors sync-corpus-to-nas.sh.
alert() {
	echo "ALERT: $1" >&2
	[ -n "$ALERT_URL" ] || return 0
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "twiceshy-corpus-refresh: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# container_health: the container's Docker HEALTHCHECK status (relies on the
# image's own /healthz self-probe — see Dockerfile), or "unknown" if unreadable.
container_health() {
	docker inspect -f '{{.State.Health.Status}}' "$CONTAINER" 2>/dev/null || echo "unknown"
}

# healthy_wait: poll container_health until "healthy" or HEALTH_TIMEOUT elapses.
healthy_wait() {
	local deadline
	deadline="$(( $(now) + HEALTH_TIMEOUT ))"
	while :; do
		[ "$(container_health)" = "healthy" ] && return 0
		[ "$(now)" -ge "$deadline" ] && return 1
		sleep "$POLL_SLEEP"
	done
}

# restart_container: docker start + wait for healthy. Returns 0 if healthy after.
restart_container() {
	echo "restart: starting $CONTAINER"
	docker start "$CONTAINER" >/dev/null 2>&1 || true
	healthy_wait
}

# --- corpus refresh ------------------------------------------------------------

main() {
	if [ ! -d "$CORPUS_CLONE/.git" ]; then
		alert "corpus clone missing at $CORPUS_CLONE — run: git clone $CORPUS_REPO $CORPUS_CLONE"
		return 1
	fi

	# 1) Change-gate: compare the corpus repo's experience tree SHA to the volume marker.
	git -C "$CORPUS_CLONE" fetch -q origin main
	local new_sha cur_sha
	new_sha="$(git -C "$CORPUS_CLONE" rev-parse origin/main:experience)"
	cur_sha="$(docker run --rm -v "$VOL":/data alpine cat /data/corpus/.experience-tree-sha 2>/dev/null || true)"
	if [ "$new_sha" = "$cur_sha" ]; then
		echo "corpus up to date ($new_sha)"
		return 0
	fi

	# 2) Mirror the corpus repo's experience/ tree into the volume (wholesale =
	#    no orphan/colliding records, ADR-0001 §1). Marker is NOT stamped here —
	#    only a verified reload advances it, so a failed reload retries next tick.
	echo "syncing corpus ${cur_sha:-<none>} -> $new_sha"
	git -C "$CORPUS_CLONE" archive --format=tar origin/main experience | docker run --rm -i -v "$VOL":/data alpine sh -c '
		rm -rf /data/corpus/experience &&
		mkdir -p /data/corpus &&
		tar xf - -C /data/corpus &&
		chown -R '"$NONROOT_UID:$NONROOT_UID"' /data/corpus'

	# 3) Reload in place via SIGHUP, then VERIFY. A binary that handles SIGHUP
	#    hot-reloads and stays up; one that does not dies — so if it goes down,
	#    the death is EXPECTED: recover it (a restart rebuilds from the updated
	#    volume = same new corpus loaded). recover failing here = unbuildable.
	docker kill -s HUP "$CONTAINER" >/dev/null 2>&1 || true
	if ! healthy_wait; then
		echo "reload: $CONTAINER not healthy after SIGHUP; recovering (restart fallback)"
		if ! restart_container; then
			alert "$CONTAINER down after corpus reload to $new_sha and restart failed — corpus likely unbuildable; marker NOT advanced (will retry)"
			return 1
		fi
	fi

	# 4) Verified healthy on the new corpus → advance the marker.
	if ! docker run --rm -v "$VOL":/data alpine sh -c "printf %s \"$new_sha\" > /data/corpus/.experience-tree-sha"; then
		alert "reload of $CONTAINER to $new_sha succeeded but stamping the marker failed (will re-mirror next tick)"
		return 1
	fi
	echo "synced + reloaded $CONTAINER at corpus $new_sha"
}

# Run main only when executed, not when sourced (so a test harness can stub the
# seams). The flock prevents overlapping ticks but is FAIL-OPEN: if the lock
# file cannot be opened, proceed without it rather than skip the refresh.
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	if { exec 9>"${TWICESHY_LOCK_FILE:-$RUNDIR/twiceshy-corpus-refresh.lock}"; } 2>/dev/null; then
		flock -n 9 || { echo "another refresh is already running; skipping"; exit 0; }
	else
		echo "warning: lock unavailable; proceeding without overlap lock" >&2
	fi
	main "$@"
fi

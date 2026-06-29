#!/usr/bin/env bash
# sync-decisions-from-nas.sh — pull the serve's #0067 gate-decision log from the NAS
# to a brain-local path, so the brain's retro drain can run the #0069 served-vs-used
# helpfulness join (issue 0098). Sibling of sync-corpus-to-nas.sh with the same seam
# and lock conventions, but the OPPOSITE direction (NAS → brain, a read-only pull).
#
# The serve writes /data/gate-decisions.jsonl onto the NAS volume, owned by the
# distroless uid 65532. We read it with the uid-safe volume-cat idiom
# (`docker run --rm -v <vol>:/data alpine cat …`) — the same trick sync-corpus-to-nas.sh
# uses for the corpus marker — and write it ATOMICALLY to the brain. A fetch failure
# never clobbers a good local copy with nothing, so the retro join degrades to "no new
# attribution this tick", never to "lost the log".
#
# Tunables (env): TWICESHY_NAS, TWICESHY_NAS_PORT, TWICESHY_VOLUME,
# TWICESHY_DECISIONS_REMOTE, TWICESHY_DECISIONS_LOCAL, TWICESHY_ALERT_URL (ntfy topic;
# unset = no-op, fail-open), NTFY_TOKEN, TWICESHY_RUNDIR.
set -euo pipefail

NAS="${TWICESHY_NAS:-Claude@192.168.50.244}"
NAS_PORT="${TWICESHY_NAS_PORT:-2222}"
VOL="${TWICESHY_VOLUME:-twiceshy-data}"
REMOTE="${TWICESHY_DECISIONS_REMOTE:-/data/gate-decisions.jsonl}"   # path INSIDE the volume
LOCAL="${TWICESHY_DECISIONS_LOCAL:-/home/ori/twiceshy-telemetry/gate-decisions.jsonl}"
ALERT_URL="${TWICESHY_ALERT_URL:-}"           # ntfy topic; unset = no-op (fail-open)
NTFY_TOKEN="${NTFY_TOKEN:-}"
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"

# --- seams (overridable in tests) -------------------------------------------------

ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$NAS" "$@"; }

# alert: log + best-effort ntfy. Never aborts (fail-open), mirrors sync-corpus-to-nas.sh.
alert() {
	echo "ALERT: $1" >&2
	[ -n "$ALERT_URL" ] || return 0
	# shellcheck disable=SC2086  # intentional word-split of the optional auth header
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "twiceshy-decisions-sync: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# --- pull -------------------------------------------------------------------------

# sync_decisions: read the remote decision log via the uid-65532-safe volume-cat idiom
# and write it ATOMICALLY to LOCAL. A fetch failure preserves the existing local copy
# (never replace good data with nothing) and returns nonzero. An rc-0 fetch that is
# empty is still a legitimate write (the log can be legitimately empty).
sync_decisions() {
	mkdir -p "$(dirname "$LOCAL")"
	local tmp="$LOCAL.tmp.$$"
	if ssh_nas "docker run --rm -v $VOL:/data alpine cat $REMOTE" > "$tmp" 2>/dev/null; then
		mv -f "$tmp" "$LOCAL"
		echo "synced decision log → $LOCAL ($(wc -l < "$LOCAL" | tr -d ' ') records)"
		return 0
	fi
	rm -f "$tmp"
	alert "failed to pull $REMOTE from $NAS (volume $VOL); kept existing $LOCAL"
	return 1
}

main() { sync_decisions; }

# Run main only when executed, not when sourced (the test harness sources this file to
# stub the seams). The flock prevents overlapping ticks but is FAIL-OPEN: an unwritable
# lock path must never silently disable the sync.
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	if { exec 9>"${TWICESHY_LOCK_FILE:-$RUNDIR/twiceshy-decisions-sync.lock}"; } 2>/dev/null; then
		flock -n 9 || { echo "another decisions-sync is already running; skipping"; exit 0; }
	else
		echo "warning: lock unavailable; proceeding without overlap lock" >&2
	fi
	main "$@"
fi

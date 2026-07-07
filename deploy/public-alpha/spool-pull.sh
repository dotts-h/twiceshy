#!/usr/bin/env bash
# spool-pull.sh — pull contribution spools from public-alpha VPS to the brain (#0139)
#
# A brain-side one-way puller mirroring the trust shape of the backup pull:
# no LAN credentials live on the VPS; the brain ssh'es to the VPS as root.
# Idempotent and safe: claims files to a subdirectory before transfer, streams
# via tar, and deletes ONLY successfully transferred files by exact name on the VPS.

set -euo pipefail

VPS_HOST="${VPS_HOST:-167.233.126.249}"
BRAIN_QUEUE_ROOT="${BRAIN_QUEUE_ROOT:-/home/ori/twiceshy-hosted-spool}"
ALERT_URL="${TWICESHY_ALERT_URL:-}"
NTFY_TOKEN="${NTFY_TOKEN:-}"

# alert: log + best-effort ntfy. Never aborts (fail-open), mirrors other deploy scripts.
alert() {
	echo "ALERT: $1" >&2
	[ -n "$ALERT_URL" ] || return 0
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "twiceshy-spool-pull: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

trap 'alert "spool-pull failed"' ERR

for q in records reports issues; do
	# (a) claim: move completed json files to a claimed/ subdirectory to avoid racing with new enqueues
	# shellcheck disable=SC2029
	if ! ssh root@"$VPS_HOST" "docker run --rm -v twiceshy-data:/data alpine sh -c 'mkdir -p /data/spool/$q/claimed && find /data/spool/$q -maxdepth 1 -name \"*.json\" -exec mv {} /data/spool/$q/claimed/ \;'" ; then
		alert "failed to claim queue $q on $VPS_HOST"
		exit 1
	fi

	# List claimed files
	# shellcheck disable=SC2029
	files=$(ssh root@"$VPS_HOST" "docker run --rm -v twiceshy-data:/data alpine sh -c 'find /data/spool/$q/claimed -maxdepth 1 -name \"*.json\" -exec basename {} \; 2>/dev/null'")

	if [ -z "$files" ]; then
		# (d) idempotent when queues are empty (tar of an empty dir is fine; guard so empty = quiet no-op)
		echo "spool-pull: $q pulled 0"
		continue
	fi

	# Count files
	n_files=$(echo "$files" | grep -c '\.json$')

	# Ensure local directory exists
	mkdir -p "$BRAIN_QUEUE_ROOT/$q"

	# (b) transfer: stream tar over ssh
	# shellcheck disable=SC2029
	if ! ssh root@"$VPS_HOST" "docker run --rm -v twiceshy-data:/data alpine tar -cf - -C /data/spool/$q/claimed ." | tar -xf - -C "$BRAIN_QUEUE_ROOT/$q"; then
		alert "failed to transfer queue $q from $VPS_HOST"
		exit 1
	fi

	# (c) delete on the VPS ONLY the exact filenames that now exist locally
	rm_list=""
	while IFS= read -r file; do
		[ -n "$file" ] || continue
		if [ -f "$BRAIN_QUEUE_ROOT/$q/$file" ]; then
			rm_list="$rm_list \"/data/spool/$q/claimed/$file\""
		fi
	done <<< "$files"

	if [ -n "$rm_list" ]; then
		# shellcheck disable=SC2029
		if ! ssh root@"$VPS_HOST" "docker run --rm -v twiceshy-data:/data alpine rm -f $rm_list"; then
			alert "failed to delete pulled files for queue $q on $VPS_HOST"
			exit 1
		fi
	fi

	echo "spool-pull: $q pulled $n_files"
done

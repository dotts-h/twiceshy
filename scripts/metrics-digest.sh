#!/usr/bin/env bash
# metrics-digest.sh — daily push/join/promote health digest to ntfy (#0116).
#
# "Never silent again" (exp-0746/#0072 lesson): a collection failure in any one
# section MUST NOT silence the whole digest — it posts a PARTIAL digest with the
# failing section marked ERROR, never nothing at all. Sections:
#   (a) push/search served-rate (+ split by trigger, top-served ids, query
#       samples) from the #0067 gate-decision log, docker cp'd off the NAS
#       container. Pure computation lives in metrics-digest-aggregate.py (this
#       script only collects + composes, so the numeric core is unit-testable
#       without ssh/docker/a Go toolchain — scripts/metrics-digest.test.sh).
#   (b) usage totals (pushed/retrieved/confirmed_helpful) from the derived
#       SQLite index, same docker-cp + aggregate.py path.
#   (c) local journalctl counts (the brain runs the drains): confirmed-helpful
#       vs unprocessable session-retro outcomes, and nightly validate run/
#       anomaly counts (no per-record promoted/held counts are logged to
#       journal today — see docs note below — so that line is marked n/a).
#
# Env: TWICESHY_NAS_SSH (ssh target), TWICESHY_NAS_SSH_PORT, TWICESHY_CONTAINER
# (the deployed container name), TWICESHY_NTFY_ENV (sourced for NTFY_URL/
# NTFY_TOKEN if present — same idiom as twiceshy-growth-watchdog.sh),
# DIGEST_HOURS (the trailing window).
set -uo pipefail

SSH="${TWICESHY_NAS_SSH:-Claude@192.168.50.244}"
SSH_PORT="${TWICESHY_NAS_SSH_PORT:-2222}"
CONTAINER="${TWICESHY_CONTAINER:-twiceshy}"
NTFY_ENV="${TWICESHY_NTFY_ENV:-/etc/twiceshy/ntfy.env}"
HOURS="${DIGEST_HOURS:-24}"
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"
AGGREGATE="${TWICESHY_METRICS_AGGREGATE:-$(dirname "${BASH_SOURCE[0]}")/metrics-digest-aggregate.py}"
PYTHON="${PYTHON:-python3}"

# The number this digest exists to move: served rate was ~70% before the #0067/
# #0108 discriminative-gate fixes — kept here for at-a-glance contrast.
BASELINE_NOTE="baseline: served rate was ~70% before the #0067/#0108 gate fixes"

log() { logger -t twiceshy-metrics-digest "$*" 2>/dev/null || true; echo "$*"; }

# --- seams (overridable in tests) -------------------------------------------------
ssh_nas() { ssh -p "$SSH_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$SSH" "$@"; }

# fetch_gate_decisions / fetch_usage_db: docker cp the file out of the running
# container over ssh, writing it to OUT. On any failure OUT is removed (not left
# empty) so aggregate.py can tell "collection failed" apart from "legitimately
# empty" — an empty-but-PRESENT file is a valid answer, a MISSING one is not.
fetch_gate_decisions() {
	local out="$1"
	if ssh_nas "docker cp $CONTAINER:/data/gate-decisions.jsonl /tmp/gd.jsonl >/dev/null && cat /tmp/gd.jsonl" >"$out" 2>/dev/null; then
		return 0
	fi
	rm -f "$out"
	return 1
}
fetch_usage_db() {
	local out="$1"
	if ssh_nas "docker cp $CONTAINER:/data/twiceshy.db /tmp/twiceshy-metrics.db >/dev/null && cat /tmp/twiceshy-metrics.db" >"$out" 2>/dev/null; then
		return 0
	fi
	rm -f "$out"
	return 1
}

journal_retro() { journalctl -u twiceshy-retro --since "${HOURS} hours ago" --no-pager 2>/dev/null; }
journal_validate() { journalctl -u twiceshy-validate --since "${HOURS} hours ago" --no-pager 2>/dev/null; }

notify() {
	local msg="$1"
	log "digest composed (${#msg} bytes)"
	# shellcheck disable=SC1090
	[ -r "$NTFY_ENV" ] && { set -a; . "$NTFY_ENV"; set +a; }
	[ -n "${NTFY_URL:-}" ] || { log "no NTFY_URL — digest not sent"; return 0; }
	curl -fsS -m 15 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} \
		-H "Title: twiceshy daily digest" -H "Tags: bar_chart" \
		-d "$msg" "$NTFY_URL" >/dev/null 2>&1 \
		|| log "ntfy POST failed (digest not delivered)"
}

# retro_section: count session-retro drain outcomes from the journal (cmd/twiceshy/
# retro.go's own log lines: "confirmed N helpful (from BASE)" and "skip BASE:
# unprocessable after N attempts"). N=0 confirmations are split out from N>0 —
# both are "processed", only unprocessable is a real failure to surface.
retro_section() {
	local log_text helpful=0 zero=0 unprocessable=0 line n
	log_text="$(journal_retro)"
	while IFS= read -r line; do
		case "$line" in
		*"confirmed "*" helpful"*)
			n="$(printf '%s\n' "$line" | sed -n 's/.*confirmed \([0-9][0-9]*\) helpful.*/\1/p')"
			case "$n" in
			'') : ;;
			0) zero=$((zero + 1)) ;;
			*) helpful=$((helpful + 1)) ;;
			esac
			;;
		esac
		case "$line" in
		*"unprocessable after"*) unprocessable=$((unprocessable + 1)) ;;
		esac
	done <<<"$log_text"
	echo "retro: ${helpful} confirmed-helpful run(s), ${zero} confirmed-zero run(s), ${unprocessable} unprocessable-after-retries (journalctl -u twiceshy-retro, last ${HOURS}h)"
}

# validate_section: nightly validate driver logs only its OWN bash echo/notify
# lines to journal (promote/adapt's per-record verdicts go to runs/*.json on the
# validate clone, not journal) — so the only greppable markers today are the
# "done: RUNID, PR #N, anomaly=0|1" line and the "flagged ANOMALY — held for
# human review" line. Per-record promoted/held counts are NOT greppable here;
# say so rather than fabricate a number (#0116 spec: note n/a when nothing
# greppable exists).
validate_section() {
	local log_text runs=0 anomalies=0 line
	log_text="$(journal_validate)"
	while IFS= read -r line; do
		case "$line" in
		*"done: "*", anomaly="*)
			runs=$((runs + 1))
			case "$line" in *"anomaly=1"*) anomalies=$((anomalies + 1)) ;; esac
			;;
		esac
	done <<<"$log_text"
	echo "validate: ${runs} run(s) logged, ${anomalies} anomaly-flagged (journalctl -u twiceshy-validate, last ${HOURS}h); per-record promoted/held counts: n/a (not greppable in journal — see runs/*.json on the validate clone)"
}

main() {
	local tmp gd_path db_path push_usage_block body
	tmp="$(mktemp -d "$RUNDIR/twiceshy-metrics-digest.XXXXXX")" || { log "mktemp failed"; return 1; }
	trap 'rm -rf "$tmp"' RETURN
	gd_path="$tmp/gate-decisions.jsonl"
	db_path="$tmp/twiceshy.db"

	fetch_gate_decisions "$gd_path" || log "gate-decisions collection FAILED (ssh/docker cp) — section will read ERROR"
	fetch_usage_db "$db_path" || log "usage-db collection FAILED (ssh/docker cp) — section will read ERROR"

	push_usage_block="$("$PYTHON" "$AGGREGATE" --gate-decisions "$gd_path" --usage-db "$db_path" --hours "$HOURS" 2>&1)"

	body="twiceshy daily digest (last ${HOURS}h)
${BASELINE_NOTE}

${push_usage_block}

$(retro_section)
$(validate_section)"

	echo "$body"
	notify "$body"
}

# Run main only when executed, not when sourced (the test harness sources this
# file to stub the seams).
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	main "$@"
fi

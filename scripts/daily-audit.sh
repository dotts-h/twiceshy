#!/usr/bin/env bash
# daily-audit.sh — morning second-opinion audit of the latest promote run (#0044,
# ADR-0013 escape hatch for a compromised judge). Reads the newest runs/*-promote.json
# manifest from the validate clone, re-judges each promotion with a high-reasoning
# auditor (default Opus 4.8), queues disagreements via `twiceshy report` for the
# next intake/adapt cycle, and posts an ntfy digest.
#
# Env knobs:
#   TWICESHY_REPO          validate clone on main (default /home/ori/twiceshy-validate).
#                          Read-only audit — this script `git reset --hard`s.
#   TWICESHY_REPORT_QUEUE  report intake queue (required to queue disputes; must match
#                          serve -report-queue / TWICESHY_REPORT_QUEUE on validate).
#   AUDIT_CMD              auditor invocation (default "claude -p").
#   AUDIT_MODEL            auditor model id (default claude-opus-4-8).
#   NTFY_URL               ntfy topic for the morning digest (optional).
#   TWICESHY_PAUSE         emergency stop — any truthy value skips the whole run.
#   TWICESHY_AUDIT_DRYRUN  1 = audit + digest, but do NOT queue disputes or notify.
#   GO                     go toolchain (default /usr/local/go/bin/go).
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-validate}"
GO="${GO:-/usr/local/go/bin/go}"
QUEUE="${TWICESHY_REPORT_QUEUE:-}"
AUDIT_CMD="${AUDIT_CMD:-claude -p}"
AUDIT_MODEL="${AUDIT_MODEL:-claude-opus-4-8}"
DRYRUN="${TWICESHY_AUDIT_DRYRUN:-0}"
NTFY_URL="${NTFY_URL:-}"

notify() {
	[ -n "$NTFY_URL" ] || return 0
	curl -fsS -d "$1" "$NTFY_URL" >/dev/null 2>&1 || true
}

# Emergency stop: short-circuit BEFORE any work.
case "${TWICESHY_PAUSE:-}" in
1 | true | TRUE | yes | on)
	echo "audit: TWICESHY_PAUSE set — paused, no run"
	exit 0
	;;
esac

cd "$REPO"
git fetch origin -q
git checkout main -q
git reset --hard origin/main -q

manifest=""
if [ -d "$REPO/runs" ]; then
	manifest="$(find "$REPO/runs" -maxdepth 1 -type f -name '*-promote.json' 2>/dev/null | LC_ALL=C sort -r | head -1 || true)"
fi
if [ -z "$manifest" ] || [ ! -f "$manifest" ]; then
	echo "no run to audit"
	exit 0
fi

bindir="$(mktemp -d)"
trap 'rm -rf "$bindir"' EXIT
bin="$bindir/twiceshy"
"$GO" build -o "$bin" ./cmd/twiceshy

run_id="$(jq -r '.run_id // "unknown"' "$manifest")"
promoted="$(jq -c '.actions[] | select(.outcome == "promoted")' "$manifest")"
if [ -z "$promoted" ]; then
	echo "audit: ${run_id} — no promotions to review"
	exit 0
fi

read -r -a audit_argv <<<"$AUDIT_CMD"

agree=0
disagree=0
lines=()

while IFS= read -r action; do
	[ -n "$action" ] || continue
	id="$(jq -r '.id' <<<"$action")"

	rec_body=""
	if [ -d "$REPO/experience" ]; then
		rec_file="$(grep -rl --include='*.md' "^id: ${id}$" "$REPO/experience" 2>/dev/null | head -1 || true)"
		if [ -n "${rec_file:-}" ] && [ -f "$rec_file" ]; then
			rec_body="$(<"$rec_file")"
		fi
	fi

	prompt="$(cat <<EOF
You are auditing an autonomous promotion to validated status. The nightly promote
run judged this record execution-provable and flipped it quarantined→validated.
Re-read the record and the promotion evidence; answer whether that promotion was
correct.

Record id: ${id}

Promotion action (JSON):
${action}

Record markdown:
${rec_body:-<not found in experience/>}

Reply with a first line of exactly AGREE or DISAGREE, then a one-line reason on
the same line after a space. Example:
DISAGREE the repro only checks compilation, not the claimed runtime behavior
EOF
)"

	response="$(printf '%s' "$prompt" | "${audit_argv[@]}" --model "$AUDIT_MODEL" 2>/dev/null || true)"
	verdict="$(printf '%s' "$response" | head -1 | awk '{print $1}')"
	reason="$(printf '%s' "$response" | head -1 | cut -d' ' -f2- | sed 's/^[[:space:]]*//')"

	case "$verdict" in
	AGREE)
		agree=$((agree + 1))
		lines+=("${id}: AGREE — ${reason:-no reason given}")
		;;
	DISAGREE)
		disagree=$((disagree + 1))
		lines+=("${id}: DISAGREE — ${reason:-no reason given}")
		if [ "$DRYRUN" != "1" ]; then
			if [ -z "$QUEUE" ]; then
				echo "TWICESHY_REPORT_QUEUE must be set to queue audit disagreements" >&2
				exit 1
			fi
			"$bin" report -id "$id" -outcome audit-disagreement \
				-evidence "${reason:-audit disagreed with promotion}" \
				-queue "$QUEUE"
		fi
		;;
	*)
		lines+=("${id}: UNKNOWN — auditor returned: $(printf '%s' "$response" | head -1 | tr -d '\n')")
		;;
	esac
done < <(printf '%s\n' "$promoted")

digest="twiceshy audit: ${run_id} — ${agree} agree, ${disagree} disagree"
for line in "${lines[@]}"; do
	digest="${digest}
${line}"
done

echo "$digest"
if [ "$DRYRUN" != "1" ]; then
	notify "$digest"
fi
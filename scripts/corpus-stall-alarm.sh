#!/usr/bin/env bash
# corpus-stall-alarm.sh — "never silent again" guard for the corpus pipeline (#0072).
#
# The 12h freeze (root cause #302/#0071, lesson exp-0746) was invisible: imports went
# red, forgejo-ci-merge correctly refused, ~15 PRs piled up, and NOTHING alerted —
# the importer swallowed the merge result and no watcher noticed the backlog. This is
# the decoupled alarm: every tick it lists the open import/* and validate/* PRs and
# fires ntfy if any has sat open past a threshold (a healthy pipeline merges them in
# minutes — open for hours means the corpus is frozen) OR is left open-and-red. A
# cooldown turns a persistent stall into a periodic reminder, not a per-tick storm.
#
# Runs on the brain as `ori` (queries the Forgejo API over the LAN). Idempotent and
# side-effect-light: a healthy tick (no stalled PRs) does nothing but clear its state.
set -uo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-import}"          # any clone — only used to read the API token from its remote
API="${TWICESHY_FORGEJO_API:-http://192.168.50.244:3030/api/v1/repos/claude/twiceshy}"
ALERT_URL="${TWICESHY_ALERT_URL:-${NTFY_URL:-}}"            # ntfy topic; unset = log-only
THRESHOLD_MIN="${TWICESHY_STALL_THRESHOLD_MIN:-120}"        # open longer than this = stalled (healthy pipeline merges in minutes)
COOLDOWN="${TWICESHY_STALL_COOLDOWN:-21600}"               # re-alarm at most this often (s); a persistent stall reminds, never spams
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"      # non-root cannot write /run
STATE_FILE="${TWICESHY_STALL_STATE:-$RUNDIR/twiceshy-stall-alarm.state}"

# --- seams (overridable in tests) -------------------------------------------------
now() { date +%s; }
notify() {
	logger -t corpus-stall-alarm "$1" 2>/dev/null || true
	[ -n "$ALERT_URL" ] || return 0
	curl -fsS -m 10 -d "corpus-stall-alarm: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# list_pipeline_prs: emit one line per OPEN import/* or validate/* PR, normalized to
# `number|branch|age_min|ci_state` (ci_state ∈ success|failure|pending|unknown). The
# whole API conversation lives here so the test can stub it wholesale.
list_pipeline_prs() {
	local token
	token="${FORGEJO_TOKEN:-$(git -C "$REPO" config --get remote.origin.url 2>/dev/null | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')}"
	[ -n "$token" ] || { logger -t corpus-stall-alarm "no Forgejo token — cannot check for stalls" 2>/dev/null || true; return 0; }
	API="$API" TOKEN="$token" THRESHOLD_MIN="$THRESHOLD_MIN" python3 - <<'PY' 2>/dev/null || true
import os, json, urllib.request, datetime
api, tok = os.environ["API"], os.environ["TOKEN"]
def get(path):
    r = urllib.request.Request(api + path); r.add_header("Authorization", f"token {tok}")
    return json.load(urllib.request.urlopen(r, timeout=15))
now = datetime.datetime.now(datetime.timezone.utc)
for pr in get("/pulls?state=open&limit=50&type=pulls"):
    ref = (pr.get("head") or {}).get("ref", "")
    if not (ref.startswith("import/") or ref.startswith("validate/")):
        continue
    created = pr.get("created_at", "")
    try:
        c = datetime.datetime.fromisoformat(created.replace("Z", "+00:00"))
        age_min = int((now - c).total_seconds() // 60)
    except Exception:
        age_min = 0
    sha = (pr.get("head") or {}).get("sha", "")
    state = "unknown"
    if sha:
        try:
            state = get(f"/commits/{sha}/status").get("state") or "unknown"
        except Exception:
            state = "unknown"
    print(f'{pr["number"]}|{ref}|{age_min}|{state}')
PY
}

# main: alarm if any pipeline PR is stalled (open past threshold) or open-and-red.
main() {
	local prs stalled="" num branch age state nowts last=0 count
	prs="$(list_pipeline_prs)"
	while IFS='|' read -r num branch age state; do
		[ -n "$num" ] || continue
		case "$age" in *[!0-9]*) age=0 ;; esac
		if [ "$age" -ge "$THRESHOLD_MIN" ] || [ "$state" = "failure" ]; then
			stalled="${stalled}${stalled:+
}#${num} ${branch} (age ${age}m, ci=${state})"
		fi
	done <<EOF
$prs
EOF

	if [ -z "$stalled" ]; then
		rm -f "$STATE_FILE" 2>/dev/null || true   # healthy → reset the cooldown
		return 0
	fi

	count="$(printf '%s\n' "$stalled" | grep -c '^#')"
	# Cooldown: alarm at most once per COOLDOWN so a days-long stall reminds, not storms.
	nowts="$(now)"
	[ -f "$STATE_FILE" ] && last="$(cat "$STATE_FILE" 2>/dev/null || echo 0)"
	if [ "$(( nowts - last ))" -ge "$COOLDOWN" ]; then
		notify "corpus pipeline STALLED — ${count} import/validate PR(s) open past ${THRESHOLD_MIN}m or red; the corpus may be frozen. Investigate + drain:
${stalled}"
		echo "$nowts" > "$STATE_FILE" 2>/dev/null || true
	fi
	return 1
}

# Run main only when executed, not when sourced (the test sources this to stub seams).
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	main "$@"
fi

#!/usr/bin/env bash
# promotions-liveness-alarm.sh — the missing net for a SILENT promotions freeze (#0122).
#
# During the #0122 incident (lesson exp-4454) the diverse-model judge froze for ~5 days:
# every validate run finished GREEN (anomaly=0, PR opened, auto-merged) yet promoted 0
# records, because the judge shim's output schema drifted and the strict parser fail-safed
# every verdict into a hold. Nothing alerted — the stall alarm watches PR/CI stalls, and
# the digest reports counts without judging them. A green run is not a productive run.
#
# This is the decoupled net: every tick it reads the last K committed promote manifests
# and fires ntfy iff ALL K promoted 0 AND the corpus still holds quarantined records that
# are eligible for judging (the runner-local hold-cooldown set is excluded — otherwise an
# idle-but-healthy system, or a backlog legitimately all in cooldown, false-positives).
#
# Runs on the brain as `ori` against the validate clone (it alone has BOTH the committed
# run manifests AND the runner-local promote.holds.json AND the corpus records). A cooldown
# turns a persistent freeze into a periodic reminder, not a per-tick storm.
set -uo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-validate}"           # the validate clone
LIVENESS_K="${TWICESHY_LIVENESS_K:-3}"                          # consecutive promoted=0 runs before alarming (≈12h at the 4h cadence)
COOLDOWN="${TWICESHY_LIVENESS_COOLDOWN:-21600}"                # re-alarm at most this often (s)
RUNDIR="${TWICESHY_RUNDIR:-${XDG_RUNTIME_DIR:-/tmp}}"          # non-root cannot write /run
STATE_FILE="${TWICESHY_LIVENESS_STATE:-$RUNDIR/twiceshy-liveness-alarm.state}"
ALERT_URL="${TWICESHY_ALERT_URL:-${NTFY_URL:-}}"              # ntfy topic; unset = log-only

# --- seams (overridable in tests) -------------------------------------------------

# promoted_counts: print `.counts.promoted` (one int per line) for the last K committed
# promote manifests (newest first). Missing/garbled manifests count as 0 (defensive).
# shellcheck disable=SC2317
promoted_counts() {
	local files
	# shellcheck disable=SC2012  # ls -t for mtime order is intentional; filenames are run ids (no spaces)
	files="$(ls -t "$REPO"/runs/run-*-promote.json 2>/dev/null | head -n "$LIVENESS_K")"
	[ -n "$files" ] || return 0
	printf '%s\n' "$files" | python3 -c '
import sys, json
for line in sys.stdin:
    path = line.strip()
    if not path:
        continue
    try:
        with open(path) as f:
            print(int(json.load(f).get("counts", {}).get("promoted", 0)))
    except Exception:
        print(0)
'
}

# eligible_quarantine_count: print the number of records under <repo>/experience whose
# frontmatter status is `quarantined` MINUS those whose id is a key in the runner-local
# hold ledger (<repo>/runs/promote.holds.json, a map id->heldAt; absent => nothing held).
# Fresh quarantined records not yet in the ledger are the "eligible for judging" backlog.
# shellcheck disable=SC2317
eligible_quarantine_count() {
	REPO="$REPO" python3 - <<'PY'
import os, json

repo = os.environ.get("REPO", "")
holds = {}
holds_path = os.path.join(repo, "runs", "promote.holds.json")
if os.path.exists(holds_path):
    try:
        with open(holds_path, encoding="utf-8") as f:
            loaded = json.load(f)
            if isinstance(loaded, dict):
                holds = loaded
    except Exception:
        pass


def id_and_status(path):
    try:
        with open(path, encoding="utf-8", errors="ignore") as f:
            if f.readline().strip() != "---":
                return None, None
            rid = status = None
            for _ in range(200):
                line = f.readline()
                if not line or line.strip() == "---":
                    break
                if ":" not in line or line[:1] in " \t":  # top-level scalar keys only
                    continue
                k, v = line.split(":", 1)
                v = v.split("#", 1)[0].strip().strip("'\"")
                if k.strip() == "id":
                    rid = v
                elif k.strip() == "status":
                    status = v
            return rid, status
    except Exception:
        return None, None


eligible = 0
exp_dir = os.path.join(repo, "experience")
for root, _dirs, files in os.walk(exp_dir):
    for name in files:
        if not name.endswith(".md"):
            continue
        rid, status = id_and_status(os.path.join(root, name))
        if status == "quarantined" and rid and rid not in holds:
            eligible += 1
print(eligible)
PY
}

# shellcheck disable=SC2317
now() { date +%s; }

# shellcheck disable=SC2317
notify() {
	logger -t promotions-liveness-alarm "$1" 2>/dev/null || true
	[ -n "$ALERT_URL" ] || return 0
	# ntfy needs a topic path (scheme://host/<topic>); a bare host 400s and the alert is
	# silently dropped — warn LOUDLY rather than POST into the void (#0093).
	case "$ALERT_URL" in
		*://*/?*) : ;;
		*) printf 'promotions-liveness-alarm: WARN ALERT_URL %s has no ntfy topic — POST will 400 and the alert is silently dropped (#0093); set NTFY_URL/TWICESHY_ALERT_URL to https://host/<topic>\n' "$ALERT_URL" >&2 ;;
	esac
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "promotions-liveness-alarm: $1" "$ALERT_URL" >/dev/null 2>&1 || true
}

# main: alarm iff the last K promote manifests ALL promoted 0 while an eligible (non-
# cooldown) quarantine backlog exists. Returns 1 when frozen, 0 when healthy.
main() {
	local counts=() n=0 line
	# `|| [ -n "$line" ]` processes a final line lacking a trailing newline.
	while IFS= read -r line || [ -n "$line" ]; do
		[ -n "$line" ] || continue
		case "$line" in *[!0-9]*) line=0 ;; esac
		counts+=("$line")
		n=$((n + 1))
	done < <(promoted_counts)

	# Not enough history to judge liveness yet.
	if [ "$n" -lt "$LIVENESS_K" ]; then
		rm -f "$STATE_FILE" 2>/dev/null || true
		return 0
	fi

	# Any promotion in the window => the loop is alive.
	local c
	for c in "${counts[@]}"; do
		if [ "$c" -gt 0 ]; then
			rm -f "$STATE_FILE" 2>/dev/null || true
			return 0
		fi
	done

	# All K promoted 0. Idle-but-healthy (empty/all-in-cooldown backlog) => no alarm.
	local elig
	elig="$(eligible_quarantine_count)"
	case "$elig" in *[!0-9]*) elig=0 ;; esac
	if [ "$elig" -le 0 ]; then
		rm -f "$STATE_FILE" 2>/dev/null || true
		return 0
	fi

	# Frozen: cooldown-gate the alarm so a persistent freeze reminds, never storms.
	local nowts last=0
	nowts="$(now)"
	[ -f "$STATE_FILE" ] && last="$(cat "$STATE_FILE" 2>/dev/null || echo 0)"
	case "$last" in *[!0-9]*) last=0 ;; esac
	if [ "$((nowts - last))" -ge "$COOLDOWN" ]; then
		notify "${LIVENESS_K} consecutive validate runs promoted 0 while ${elig} quarantined records are eligible for judging — judge pipeline likely broken (#0122)"
		echo "$nowts" >"$STATE_FILE" 2>/dev/null || true
	fi
	return 1
}

# Run main only when executed, not when sourced (the test sources this to stub seams).
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	main "$@"
fi

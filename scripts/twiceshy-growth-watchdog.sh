#!/usr/bin/env bash
# twiceshy-growth-watchdog.sh — the GROWTH-side watch on the autonomous corpus loop.
#
# The stall-alarm watches for PRs piling up unmerged. This watches the complementary
# failure: the served corpus (main) not GROWING — which is what actually happened
# 2026-06-26 (drain-merge timer manually disabled 2026-06-23 → promotions computed
# but never merged → main frozen ~4 days, and the muted ntfy alarm never said so).
#
# Each run: refresh the corpus mirror, count `status: validated` records on main, and
# alert (ntfy, Bearer-authed) if that count has not increased in >MAX_STALE_HOURS, or
# if any core loop timer is down. Idempotent + log-only when healthy.
set -uo pipefail

CLONE="${TWICESHY_REPO:-/home/ori/twiceshy-corpus}"
STATE="${TWICESHY_GROWTH_STATE:-/home/ori/.local/state/twiceshy-growth.tsv}"
MAX_STALE_HOURS="${TWICESHY_MAX_STALE_HOURS:-24}"
TIMERS=(twiceshy-validate.timer twiceshy-drain-merge.timer twiceshy-stall-alarm.timer)
NTFY_ENV="${TWICESHY_NTFY_ENV:-/etc/twiceshy/ntfy.env}"

log() { logger -t twiceshy-growth-watchdog "$*" 2>/dev/null || true; echo "$*"; }

# ntfy alert (Bearer-authed against the deny-all server — see #0093).
alert() {
  local msg="$1"
  log "ALERT: $msg"
  # shellcheck disable=SC1090
  [ -r "$NTFY_ENV" ] && { set -a; . "$NTFY_ENV"; set +a; }
  [ -n "${NTFY_URL:-}" ] || { log "no NTFY_URL — alert not sent"; return 0; }
  curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} \
       -H "Title: twiceshy: corpus growth watchdog" -H "Tags: rotating_light" \
       -d "$msg" "$NTFY_URL" >/dev/null 2>&1 \
    || log "ntfy POST failed (alert not delivered)"
}

mkdir -p "$(dirname "$STATE")"

# --- refresh mirror + count validated on main ---
if ! git -C "$CLONE" fetch -q origin main 2>/dev/null; then
  alert "cannot fetch corpus repo at $CLONE — watchdog blind"; exit 1
fi
git -C "$CLONE" checkout -q -B main origin/main 2>/dev/null
count="$(git -C "$CLONE" grep -lI 'status: validated' origin/main -- 'experience/*.md' 2>/dev/null | wc -l | tr -d ' ')"
now="$(date +%s)"
[ "${count:-0}" -gt 0 ] || { alert "validated count read as 0 — suspect; watchdog aborting"; exit 1; }

# --- compare against the last recorded GROWTH point ---
prev_count=0; prev_grow_ts="$now"
if [ -s "$STATE" ]; then
  read -r prev_count prev_grow_ts < <(tail -n1 "$STATE")
fi
if [ "$count" -gt "${prev_count:-0}" ]; then
  printf '%s\t%s\n' "$count" "$now" >> "$STATE"           # grew → new growth point
  log "healthy: validated=$count (+$((count - prev_count)) since last point)"
else
  stale_h=$(( (now - prev_grow_ts) / 3600 ))
  if [ "$stale_h" -ge "$MAX_STALE_HOURS" ]; then
    alert "corpus NOT growing: validated stuck at $count for ${stale_h}h (≥${MAX_STALE_HOURS}h). Check validate/drain-merge timers + open PRs."
  else
    log "flat: validated=$count, ${stale_h}h since last growth (<${MAX_STALE_HOURS}h, ok)"
  fi
fi

# --- core loop timers must be enabled AND active ---
down=""
for t in "${TIMERS[@]}"; do
  systemctl is-enabled --quiet "$t" 2>/dev/null && systemctl is-active --quiet "$t" 2>/dev/null || down="$down $t"
done
[ -n "$down" ] && alert "loop timer(s) DOWN:$down — corpus will freeze"
log "done: validated=$count, timers_down=[${down# }]"

#!/usr/bin/env bash
# twiceshy SessionEnd hook — fail-open session-retro capture client (#0065, ADR-0018).
#
# Ships a bounded session transcript to the /retro endpoint at the once-per-session
# lifecycle seam. The expensive off-pool analysis happens SERVER-side (retro-intake),
# so capture never depends on the agent remembering to submit. The hook is dumb and
# deterministic: it bounds, screens locally, and POSTs — it never blocks the agent.
#
# Register under hooks.SessionEnd in settings.json (see hooks/README.md).
set -uo pipefail

fail_open() { exit 0; } # never block or error the session

command -v jq   >/dev/null 2>&1 || fail_open
command -v curl >/dev/null 2>&1 || fail_open

[ -n "${TWICESHY_TOKEN:-}" ] || fail_open

TWICESHY_URL="${TWICESHY_URL:-http://192.168.50.244:8722}"
# Bound the transcript well under the server's 256 KiB body cap (JSON headroom).
RETRO_MAX_BYTES="${TWICESHY_RETRO_MAX_BYTES:-200000}"

input="$(cat)" || fail_open

session="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)" || fail_open
reason="$(printf '%s' "$input" | jq -r '.reason // empty' 2>/dev/null)" || reason=""
tpath="$(printf '%s' "$input" | jq -r '.transcript_path // empty' 2>/dev/null)" || fail_open
[ -n "$tpath" ] && [ -r "$tpath" ] || fail_open

# The recent tail carries the session's resolved traps; bound it so the POST stays
# under the body cap.
transcript="$(tail -c "$RETRO_MAX_BYTES" "$tpath" 2>/dev/null)" || fail_open
[ -n "$transcript" ] || fail_open

# Screen client-side with the SAME tested screen, before anything leaves the machine.
# A secret → skip the send (fail-safe); the server re-screens at the edge regardless.
# If the binary is absent, skip the local screen — the server is the authoritative gate.
if command -v twiceshy >/dev/null 2>&1; then
  printf '%s' "$transcript" | twiceshy screen >/dev/null 2>&1 || fail_open
fi

payload="$(jq -n \
  --arg t "$transcript" \
  --arg s "$session" \
  --arg r "$reason" \
  '{transcript: $t, session: $s, reason: $r, author: "claude"}')" || fail_open

curl -sfS --max-time 15 \
  -H "Authorization: Bearer ${TWICESHY_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$payload" \
  "${TWICESHY_URL%/}/retro" >/dev/null 2>&1 || fail_open

exit 0

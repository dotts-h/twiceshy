#!/usr/bin/env bash
# twiceshy UserPromptSubmit hook — fail-open push channel client.
set -uo pipefail

fail_open() { exit 0; }

command -v jq >/dev/null 2>&1 || fail_open
command -v curl >/dev/null 2>&1 || fail_open

[ -n "${TWICESHY_TOKEN:-}" ] || fail_open

TWICESHY_URL="${TWICESHY_URL:-http://192.168.50.244:8722}"

input="$(cat)" || fail_open

prompt="$(printf '%s' "$input" | jq -r '.prompt // empty' 2>/dev/null)" || fail_open
[ -n "$prompt" ] || fail_open

# Skip harness-generated pseudo-prompts (task notifications, local-command echoes):
# they are not user intent, and their dev-flavored text false-fires the gate
# (observed live 2026-07-01: the only post-v0.2.9 serves were <task-notification>
# blobs corroborating unrelated trap cards).
case "$prompt" in
  "<task-notification>"*|"<local-command-"*|"<command-name>"*) fail_open ;;
esac

# Forward the session id so the gate-decision log attributes pushed cards to this
# session for the retro served->used helpfulness join (#0069, ADR-0025). The
# SessionEnd capture hook ships the SAME id, so transcript-vs-decisions joins on it.
session="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null || true)"

payload="$(jq -n --arg q "$prompt" --arg s "$session" '{query: $q, session: $s, trigger: "prompt"}')" || fail_open

response="$(
  curl -sfS --max-time 10 \
    -H "Authorization: Bearer ${TWICESHY_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "${TWICESHY_URL%/}/push" 2>/dev/null
)" || fail_open

if printf '%s' "$response" | jq -e \
  '.count > 0 and (.context | type) == "string" and (.context | length) > 0' \
  >/dev/null 2>&1; then
  context="$(printf '%s' "$response" | jq -r '.context')"
  jq -n --arg ctx "$context" \
    '{hookSpecificOutput: {hookEventName: "UserPromptSubmit", additionalContext: $ctx}}'
fi

exit 0
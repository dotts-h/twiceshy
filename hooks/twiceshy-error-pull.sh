#!/usr/bin/env bash
# twiceshy PostToolUse hook — error-scoped pull client (#0087, prototype).
#
# The reliable retrieval trigger is "an error appeared in tool output", NOT "a
# prompt was submitted": per-prompt push is ~0% precision on real traffic (exp-0622),
# and a judgment-based "search before debugging" instruction loses to "I'll just read
# the error". This moves the trigger onto the moment pull actually targets and hands
# the query for free — the verbatim error line, which is what twiceshy indexes as
# error_signatures.
#
# It fires on the SECOND distinct appearance of an error signature (the "before
# retrying what just failed" tripwire — a first failure may be a typo; a repeat means
# stuck), dedupes per session+signature, and queries the existing /push retrieval with
# the error line (the same gate the push hook uses; an error line's rare identifiers
# are exactly the discriminative tokens the gate wants). Fail-open throughout; an empty
# result injects nothing.
#
# Tunables: TWICESHY_ERROR_PULL_ON_FIRST=1 fires on the first occurrence (testing/
# aggressive mode). TWICESHY_TOKEN / TWICESHY_URL as for twiceshy-push.sh.
set -uo pipefail
fail_open() { exit 0; }

command -v jq   >/dev/null 2>&1 || fail_open
command -v curl >/dev/null 2>&1 || fail_open
[ -n "${TWICESHY_TOKEN:-}" ] || fail_open
TWICESHY_URL="${TWICESHY_URL:-http://192.168.50.244:8722}"

input="$(cat)" || fail_open
sid="$(printf '%s' "$input" | jq -r '.session_id // "nosession"' 2>/dev/null)" || fail_open

# Render the tool output to text: Bash stdout+stderr, or a stringy tool_response.
text="$(printf '%s' "$input" | jq -r '
  [ (.tool_response.stdout // empty),
    (.tool_response.stderr // empty),
    (.tool_response | if type=="string" then . else empty end) ] | join("\n")
' 2>/dev/null)" || fail_open
[ -n "$text" ] || fail_open

# Error-signature tripwire — take the FIRST matching line as the query.
errline="$(printf '%s\n' "$text" | grep -m1 -aE \
  'Traceback \(most recent|panic:|[A-Z][A-Za-z]*Error\b|error:|\bERROR\b|\[!\]|fatal:|Exception|command not found|No such file or directory' \
  2>/dev/null || true)"
errline="$(printf '%s' "$errline" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' | cut -c1-300)"
[ -n "$errline" ] || fail_open

# 2nd-occurrence + dedup gate, per session+signature (normalize for the key).
key="$(printf '%s' "$errline" | tr '[:upper:]' '[:lower:]' | tr -s ' ' | sha256sum 2>/dev/null | cut -c1-16)" || fail_open
[ -n "$key" ] || fail_open
dir="${TMPDIR:-/tmp}/twiceshy-error-pull/${sid}"
mkdir -p "$dir" 2>/dev/null || fail_open
seen="$dir/$key.seen"; done="$dir/$key.done"
[ -f "$done" ] && fail_open   # already queried this signature this session
case "${TWICESHY_ERROR_PULL_ON_FIRST:-0}" in
1 | true | yes) ;;            # aggressive: query on first occurrence
*)
  if [ ! -f "$seen" ]; then
    : > "$seen"               # first occurrence — arm the tripwire, do not query
    fail_open
  fi
  ;;
esac
: > "$done"                   # query once for this signature

payload="$(jq -n --arg q "$errline" '{query: $q}')" || fail_open
response="$(curl -sfS --max-time 10 \
  -H "Authorization: Bearer ${TWICESHY_TOKEN}" -H "Content-Type: application/json" \
  -d "$payload" "${TWICESHY_URL%/}/push" 2>/dev/null)" || fail_open

if printf '%s' "$response" | jq -e \
  '.count > 0 and (.context | type) == "string" and (.context | length) > 0' \
  >/dev/null 2>&1; then
  context="$(printf '%s' "$response" | jq -r '.context')"
  jq -n --arg ctx "$context" \
    '{hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext: $ctx}}'
fi
exit 0

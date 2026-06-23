#!/usr/bin/env bash
# Tests for twiceshy-error-pull.sh (#0087) — the error-scoped PostToolUse pull
# hook. Black-box: the hook shells out to curl/jq by bareword, so we shadow ONLY
# curl with a stub on PATH (the network seam) and let the real jq run, then drive
# the real script end-to-end over stdin. No real network. Run: bash twiceshy-error-pull.test.sh
#
# The stub curl echoes $STUB_RESPONSE and exits $STUB_EXIT, so a test fixes the
# server's reply (a card, nothing, or a transport failure) without a server.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

HOOK="$PWD/twiceshy-error-pull.sh"
PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
contains()  { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }
empty_ok()  { if [ -z "$2" ]; then ok "$1"; else bad "$1 (got [$2])"; fi; }

command -v jq >/dev/null 2>&1 || { echo "SKIP: jq not installed"; exit 0; }

# ---- stub curl on PATH (only the network seam) -----------------------------------
STUBDIR="$(mktemp -d)"
trap 'rm -rf "$STUBDIR"' EXIT
cat > "$STUBDIR/curl" <<'STUB'
#!/usr/bin/env bash
# Stub: ignore the request, emit the canned response, exit the canned code.
printf '%s' "${STUB_RESPONSE:-}"
exit "${STUB_EXIT:-0}"
STUB
chmod +x "$STUBDIR/curl"

# run_hook <stdin-json> — invoke the hook with the stub curl ahead on PATH and a
# per-call session sandbox (TMPDIR) unless the caller pinned one for a sequence.
run_hook() {
	printf '%s' "$1" | PATH="$STUBDIR:$PATH" \
		TWICESHY_TOKEN="${TWICESHY_TOKEN-tkn}" \
		TWICESHY_URL="http://stub.invalid" \
		TMPDIR="${TMPDIR_PIN:-$(mktemp -d "$STUBDIR/sess.XXXXXX")}" \
		STUB_RESPONSE="${STUB_RESPONSE-}" STUB_EXIT="${STUB_EXIT-0}" \
		TWICESHY_ERROR_PULL_ON_FIRST="${TWICESHY_ERROR_PULL_ON_FIRST-0}" \
		bash "$HOOK"
}

ERRJSON='{"session_id":"s1","tool_response":{"stderr":"TypeError: Cannot read property '\''lngLat'\'' of null\n"}}'
CARD='{"count":1,"context":"=== twiceshy ===\nexp-9999"}'

# 1) No token -> fail-open before anything else. Arm the signature first (with a
#    token) and set STUB_RESPONSE=$CARD, so a tokenless run that did NOT fail open
#    would reach curl and emit the card on this 2nd occurrence — turning the
#    empty assertion red. That makes the case prove the token gate, not just arming.
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq0.XXXXXX")"
STUB_RESPONSE="$CARD"
run_hook "$ERRJSON" >/dev/null                       # arm (a 2nd run would query)
out="$(TWICESHY_TOKEN='' run_hook "$ERRJSON")"; rc=$? # tokenless: the gate must stop it
if [ "$rc" = 0 ] && [ -z "$out" ]; then ok "no token: fail-open silent"; else bad "no token (rc=$rc out=[$out])"; fi
unset TMPDIR_PIN STUB_RESPONSE

# 2) First occurrence (default mode) -> arm the tripwire, do NOT query, emit nothing.
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq.XXXXXX")"
STUB_RESPONSE="$CARD"
empty_ok "first occurrence: armed, no query" "$(run_hook "$ERRJSON")"

# 3) Second occurrence, server returns a card -> inject it as additionalContext.
out="$(run_hook "$ERRJSON")"
if contains "$out" "additionalContext" && contains "$out" "exp-9999" && contains "$out" "PostToolUse"; then
	ok "second occurrence: card injected"
else
	bad "second occurrence missing card [$out]"
fi

# 4) Third occurrence of the SAME signature -> deduped (.done), no re-query.
empty_ok "dedup: same signature queried once" "$(run_hook "$ERRJSON")"
unset TMPDIR_PIN

# 5) Benign prose ("no errors found") -> no error signature, so even a pinned 2nd
#    pass never arms or queries. STUB_RESPONSE=$CARD means a grep that WRONGLY
#    matched benign prose would arm then emit the card on the 2nd call, failing this.
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq5.XXXXXX")"
STUB_RESPONSE="$CARD"
benign='{"session_id":"s2","tool_response":{"stdout":"no errors found, all good\n"}}'
run_hook "$benign" >/dev/null                                     # would arm IF it matched
empty_ok "benign prose: no false trigger" "$(run_hook "$benign")" # would emit IF it matched
unset TMPDIR_PIN

# 6) ON_FIRST=1 -> query on the very first occurrence (aggressive mode).
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq2.XXXXXX")"
STUB_RESPONSE="$CARD"
out="$(TWICESHY_ERROR_PULL_ON_FIRST=1 run_hook "$ERRJSON")"
if contains "$out" "exp-9999"; then ok "ON_FIRST: queries on first occurrence"; else bad "ON_FIRST silent [$out]"; fi
unset TMPDIR_PIN

# 7) Empty result (count=0) on the 2nd occurrence -> inject nothing.
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq3.XXXXXX")"
STUB_RESPONSE='{"count":0,"context":""}'
run_hook "$ERRJSON" >/dev/null    # arm
empty_ok "empty result: injects nothing" "$(run_hook "$ERRJSON")"
unset TMPDIR_PIN

# 8) curl transport failure on the 2nd occurrence -> fail-open silent.
TMPDIR_PIN="$(mktemp -d "$STUBDIR/seq4.XXXXXX")"
STUB_RESPONSE="$CARD"; STUB_EXIT=22
run_hook "$ERRJSON" >/dev/null    # arm (curl not yet called)
empty_ok "curl failure: fail-open silent" "$(run_hook "$ERRJSON")"
unset TMPDIR_PIN STUB_EXIT

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

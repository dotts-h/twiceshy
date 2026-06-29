#!/usr/bin/env bash
# Hermetic contract tests for retro-analyzer-shim.py — extraction candidates AND
# usage-judge verdicts (#0099). Run from repo root: bash scripts/retro-analyzer-shim.test.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

pick_port() {
	python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'
}

wait_health() {
	local port=$1 attempts=${2:-50}
	local i=0
	while [ "$i" -lt "$attempts" ]; do
		if python3 -c "
import urllib.request
try:
    with urllib.request.urlopen('http://127.0.0.1:${port}/health', timeout=1) as r:
        exit(0 if r.status == 200 else 1)
except Exception:
    exit(1)
" 2>/dev/null; then
			return 0
		fi
		sleep 0.1
		i=$((i + 1))
	done
	return 1
}

post_shim() {
	local port=$1
	python3 -c "
import json, urllib.error, urllib.request, sys
req = urllib.request.Request(
    'http://127.0.0.1:${port}/',
    data=json.dumps({'prompt': 'x'}).encode(),
    headers={'Content-Type': 'application/json'},
    method='POST',
)
try:
    with urllib.request.urlopen(req, timeout=10) as r:
        print(r.status)
        print(r.read().decode())
except urllib.error.HTTPError as e:
    print(e.code)
    sys.stdout.write(e.read().decode())
"
}

STUB_PORT="$(pick_port)"
SHIM_PORT="$(pick_port)"
CONTENT_FILE="$(mktemp)"
STUB_PID=""
SHIM_PID=""

cleanup() {
	[ -n "$SHIM_PID" ] && kill "$SHIM_PID" 2>/dev/null || true
	[ -n "$STUB_PID" ] && kill "$STUB_PID" 2>/dev/null || true
	wait "$SHIM_PID" 2>/dev/null || true
	wait "$STUB_PID" 2>/dev/null || true
	rm -f "$CONTENT_FILE"
}
trap cleanup EXIT

export STUB_PORT STUB_CONTENT_FILE="$CONTENT_FILE"
python3 -u - <<'PY' &
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

CONTENT_FILE = os.environ["STUB_CONTENT_FILE"]
PORT = int(os.environ["STUB_PORT"])


class StubHandler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        self.rfile.read(n)
        with open(CONTENT_FILE, encoding="utf-8") as f:
            content = f.read().strip()
        body = json.dumps({"choices": [{"message": {"content": content}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


HTTPServer(("127.0.0.1", PORT), StubHandler).serve_forever()
PY
STUB_PID=$!

OPENROUTER_ENDPOINT="http://127.0.0.1:${STUB_PORT}/v1/chat/completions" \
	RETRO_PORT="$SHIM_PORT" \
	OPENROUTER_MODEL=stub \
	python3 -u scripts/retro-analyzer-shim.py >/dev/null 2>&1 &
SHIM_PID=$!

if ! wait_health "$SHIM_PORT"; then
	bad "shim /health never became ready"
	echo "----"
	echo "PASS=$PASS FAIL=$FAIL"
	exit 1
fi
ok "shim /health ready"

run_case() {
	local name=$1 content=$2 want_code=$3
	local body code out

	printf '%s' "$content" > "$CONTENT_FILE"
	out="$(post_shim "$SHIM_PORT")"
	code="$(printf '%s\n' "$out" | head -n1)"
	body="$(printf '%s\n' "$out" | tail -n +2)"

	check "${name} HTTP status" "$code" "$want_code"
	case "$name" in
		"A usage verdicts")
			if contains "$body" "verdicts" && contains "$body" "exp-0149"; then
				ok "${name} body contains verdicts and exp-0149"
			else
				bad "${name} body must contain verdicts and exp-0149: $body"
			fi
			;;
		"B extraction candidates")
			if contains "$body" "candidates"; then
				ok "${name} body contains candidates"
			else
				bad "${name} body must contain candidates: $body"
			fi
			;;
		"C neither schema")
			if contains "$body" "candidates" || contains "$body" "verdicts"; then
				ok "${name} error mentions candidates or verdicts"
			else
				bad "${name} error must mention candidates/verdicts: $body"
			fi
			;;
	esac
}

run_case "A usage verdicts" '{"verdicts":[{"id":"exp-0149","used":true}]}' "200"
run_case "B extraction candidates" '{"candidates":[{"kind":"trap","title":"t"}]}' "200"
run_case "C neither schema" '{"foo":1}' "502"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

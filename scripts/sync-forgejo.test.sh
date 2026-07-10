#!/usr/bin/env bash
# Hermetic regression tests for safe origin-to-API derivation (#0149).
# Run from repo root: bash scripts/sync-forgejo.test.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/sync-forgejo.sh"

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

run_case() {
	local name=$1 origin=$2 want_api=$3 want_derive_failure=$4 secret=${5:-}
	local tmp bindir curl_log out status
	tmp="$(mktemp -d)"
	bindir="$tmp/bin"
	curl_log="$tmp/curl.log"
	mkdir -p "$bindir"

	git -C "$tmp" init -q
	git -C "$tmp" remote add origin "$origin"
	cat >"$bindir/curl" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$CURL_LOG"
exit 1
SH
	chmod +x "$bindir/curl"

	set +e
	out="$(cd "$tmp" && env \
		-u FORGEJO_API -u FORGEJO_REPO \
		PATH="$bindir:/usr/bin:/bin" \
		CURL_LOG="$curl_log" FORGEJO_TOKEN=test-token \
		bash "$SCRIPT" 2>&1)"
	status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		ok "$name fails open"
	else
		bad "$name exit status $status, want 0"
	fi

	if [ "$want_derive_failure" = true ]; then
		if contains "$out" "could not safely derive Forgejo API from origin" && contains "$out" "skipping mirror"; then
			ok "$name reports a clear derivation skip"
		else
			bad "$name missing clear derivation skip: $out"
		fi
		if [ ! -s "$curl_log" ]; then
			ok "$name does not call curl"
		else
			bad "$name unexpectedly called curl: $(tr '\n' ' ' <"$curl_log")"
		fi
	else
		if [ -s "$curl_log" ] && grep -Fxq "$want_api/repos/owner/repo" "$curl_log"; then
			ok "$name derives $want_api"
		else
			bad "$name curl target did not contain $want_api/repos/owner/repo"
		fi
	fi

	if contains "$out" "Traceback"; then
		bad "$name emitted a traceback"
	else
		ok "$name emits no traceback"
	fi
	if [ -n "$secret" ] && { contains "$out" "$secret" || { [ -s "$curl_log" ] && grep -Fq "$secret" "$curl_log"; }; }; then
		bad "$name exposed embedded credentials"
	else
		ok "$name keeps embedded credentials secret"
	fi

	rm -rf "$tmp"
}

run_case \
	"token-embedded HTTP origin" \
	"http://user:super-secret@forge.example:3030/owner/repo.git" \
	"http://forge.example:3030/api/v1" false "super-secret"

run_case \
	"ordinary HTTPS origin" \
	"https://forge.example/owner/repo.git" \
	"https://forge.example/api/v1" false

run_case \
	"malformed origin" \
	"https://user:super-secret@forge.example:notaport/owner/repo.git" \
	"" true "super-secret"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

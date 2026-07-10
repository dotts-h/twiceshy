#!/usr/bin/env bash
# Hermetic durability regression tests for scheduled-validate.sh (#0148).
# Uses local bare git remotes and fake engine/curl binaries; no network, judge,
# Docker, or Forgejo is contacted.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAKEBIN="$TMP/bin"
mkdir -p "$FAKEBIN"
REAL_JQ="$(command -v jq)"
ln -s "$REAL_JQ" "$FAKEBIN/jq"
cat >"$FAKEBIN/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CURL_LOG"
cat >/dev/null
printf '{"number":42}\n'
EOF
chmod +x "$FAKEBIN/curl"

new_clone() {
	local name="$1" bare seed clone
	bare="$TMP/${name}.git"
	seed="$TMP/${name}-seed"
	clone="$TMP/${name}-clone"
	git init -q --bare "$bare"
	git init -q -b main "$seed"
	git -C "$seed" config user.email test@example.invalid
	git -C "$seed" config user.name test
	mkdir -p "$seed/experience/2026" "$seed/runs"
	printf 'base\n' >"$seed/experience/2026/0001-test.md"
	printf 'runs/*.journal.json\n' >"$seed/.gitignore"
	git -C "$seed" add .
	git -C "$seed" commit -qm base
	git -C "$seed" remote add origin "$bare"
	git -C "$seed" push -q -u origin main
	git --git-dir="$bare" symbolic-ref HEAD refs/heads/main
	git clone -q "$bare" "$clone"
	git -C "$clone" config user.email test@example.invalid
	git -C "$clone" config user.name test
	printf '%s\n' "$clone"
}

run_driver() {
	local repo="$1" engine="$2" log="$3" top shared
	[ -n "$repo" ] || { echo "unsafe test repo: empty" >&2; return 99; }
	top="$(git -C "$repo" rev-parse --show-toplevel 2>/dev/null)" || { printf 'unsafe test repo: not a git checkout: %q\n' "$repo" >&2; return 99; }
	shared="$(git -C "$PWD" rev-parse --show-toplevel)"
	case "$top" in "$TMP"/*-clone) ;; *) printf 'unsafe test repo outside temp root: %q\n' "$top" >&2; return 99 ;; esac
	[ "$top" != "$shared" ] || { echo "unsafe test repo aliases shared checkout" >&2; return 99; }
	PATH="$FAKEBIN:/usr/bin:/bin" \
	CURL_LOG="$log" \
	TWICESHY_REPO="$repo" \
	TWICESHY_BIN="$engine" \
	TWICESHY_SKIP_ENGINE_FRESH=1 \
	TWICESHY_JUDGE_URL=http://judge.invalid \
	TWICESHY_AUTOMERGE=0 \
	TWICESHY_VALIDATE_DRYRUN="${TWICESHY_VALIDATE_DRYRUN:-0}" \
	TWICESHY_RECORD_QUEUE='' \
	TWICESHY_FORGEJO_REPO=test/corpus \
	bash "$PWD/scripts/scheduled-validate.sh"
}

# A prior SIGKILL/reboot leaves a validate branch dirty. The next invocation
# must preserve only validation artifacts in a commit+remote branch/PR boundary
# before any checkout/reset can erase them.
repo="$(new_clone startup)"
git -C "$repo" checkout -qb validate/run-killed
printf 'promoted\n' >"$repo/experience/2026/0001-test.md"
mkdir -p "$repo/runs"
printf '{"actions":[{"id":"exp-0001"}]}\n' >"$repo/runs/promote.journal.json"
printf 'operator scratch\n' >"$repo/unrelated.txt"
engine="$TMP/noop-engine"
cat >"$engine" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$engine"
log="$TMP/startup-curl.log"; : >"$log"
out="$(run_driver "$repo" "$engine" "$log" 2>&1)"; rc=$?
check "startup recovery exits successfully" "$rc" "0"
check "startup recovery commits promoted edit" "$(git -C "$repo" show HEAD:experience/2026/0001-test.md)" "promoted"
if git -C "$repo" show HEAD:unrelated.txt >/dev/null 2>&1; then bad "startup recovery must not commit unrelated files"; else ok "startup recovery excludes unrelated files"; fi
if git --git-dir="$TMP/startup.git" show-ref --verify -q refs/heads/validate/run-killed; then ok "startup recovery pushes durable branch"; else bad "startup recovery must push durable branch"; fi
if contains "$(cat "$log")" "/pulls"; then ok "startup recovery opens recovery PR"; else bad "startup recovery must open recovery PR"; fi
if contains "$out" "recovered"; then ok "startup recovery is announced"; else bad "startup recovery must be announced: $out"; fi

# A verification/dry-run must never turn recovery into external state. It also
# must refuse the destructive reset, leaving the partial edit recoverable for a
# later real invocation.
repo="$(new_clone dryrun)"
git -C "$repo" checkout -qb validate/run-dryrun-killed
printf 'promoted-dryrun\n' >"$repo/experience/2026/0001-test.md"
log="$TMP/dryrun-curl.log"; : >"$log"
set +e
out="$(TWICESHY_VALIDATE_DRYRUN=1 run_driver "$repo" "$engine" "$log" 2>&1)"; rc=$?
set -e
check "dry-run recovery refuses reset" "$rc" "1"
check "dry-run leaves partial edit untouched" "$(cat "$repo/experience/2026/0001-test.md")" "promoted-dryrun"
check "dry-run creates no commit" "$(git -C "$repo" rev-list --count origin/main..HEAD)" "0"
if [ -s "$log" ]; then bad "dry-run recovery must not call Forgejo"; else ok "dry-run recovery makes no Forgejo call"; fi

# SIGTERM during promote is the graceful out-of-band path. The fake engine
# persists one promoted edit + journal action, then signals its parent driver.
repo="$(new_clone term)"
printf 'operator scratch\n' >"$repo/unrelated.txt"
engine="$TMP/term-engine"
cat >"$engine" <<'EOF'
#!/usr/bin/env bash
set -eu
case "${1:-}" in
promote)
	printf 'promoted-before-term\n' >"$TWICESHY_REPO/experience/2026/0001-test.md"
	mkdir -p "$TWICESHY_REPO/runs"
	printf '{"actions":[{"id":"exp-0001"}]}\n' >"$TWICESHY_REPO/runs/promote.journal.json"
	kill -TERM "$PPID"
	;;
esac
exit 0
EOF
chmod +x "$engine"
log="$TMP/term-curl.log"; : >"$log"
set +e
out="$(run_driver "$repo" "$engine" "$log" 2>&1)"; rc=$?
set -e
check "TERM path exits with signal status" "$rc" "143"
branch="$(git -C "$repo" branch --show-current)"
case "$branch" in validate/*) ok "TERM leaves recovery branch checked out" ;; *) bad "TERM must leave validate branch, got $branch" ;; esac
check "TERM commits promoted edit" "$(git -C "$repo" show HEAD:experience/2026/0001-test.md)" "promoted-before-term"
if git -C "$repo" show HEAD:unrelated.txt >/dev/null 2>&1; then bad "TERM must not commit unrelated files"; else ok "TERM excludes unrelated files"; fi
if git --git-dir="$TMP/term.git" show-ref --verify -q "refs/heads/$branch"; then ok "TERM pushes durable branch"; else bad "TERM must push durable branch"; fi
if contains "$(cat "$log")" "/pulls"; then ok "TERM opens recovery PR"; else bad "TERM must open recovery PR"; fi

# Ordinary promote failures retain the existing abort policy: no commit, push,
# or PR. TERM recovery must not accidentally convert every error into a batch.
repo="$(new_clone abort)"
engine="$TMP/fail-engine"
cat >"$engine" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in promote) exit 2 ;; esac
exit 0
EOF
chmod +x "$engine"
log="$TMP/abort-curl.log"; : >"$log"
set +e
out="$(run_driver "$repo" "$engine" "$log" 2>&1)"; rc=$?
set -e
check "ordinary promote error keeps its exit status" "$rc" "2"
check "ordinary promote error returns to main" "$(git -C "$repo" branch --show-current)" "main"
check "ordinary promote error creates no commit" "$(git -C "$repo" rev-list --count origin/main..main)" "0"
if git --git-dir="$TMP/abort.git" for-each-ref --format='%(refname)' refs/heads/validate/ | grep -q .; then bad "ordinary abort must not push a recovery branch"; else ok "ordinary abort pushes no recovery branch"; fi
if [ -s "$log" ]; then bad "ordinary abort must not open a PR"; else ok "ordinary abort opens no PR"; fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

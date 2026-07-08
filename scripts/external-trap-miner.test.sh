#!/usr/bin/env bash
# Hermetic contract tests for external-trap-miner.sh (#0133 Part B).
# Run from repo root: bash scripts/external-trap-miner.test.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

SCRATCH="$(mktemp -d)"
FIXTURE="$(mktemp -d)"
QUEUE="$SCRATCH/queue"
SEEN="$SCRATCH/seen"
CLONE_MARKER="$SCRATCH/clone-called"
LOG="$SCRATCH/log.txt"
MINER="$REPO_ROOT/scripts/external-trap-miner.sh"

cleanup() { rm -rf "$SCRATCH" "$FIXTURE"; }
trap cleanup EXIT

# Tiny local git repo with one fix-shaped commit (real git, no network).
(
	cd "$FIXTURE" || exit 1
	git init -q
	git config user.email test@example.com
	git config user.name test
	printf 'baseline content for diff size\n' >main.go
	git add main.go
	git commit -q -m 'initial import'
	printf 'baseline content for diff size\nfixed race in handler\n' >main.go
	git add main.go
	git commit -q -m 'fix race in handler'
) || { bad 'fixture git repo setup'; exit 1; }

FIX_SHA="$(git -C "$FIXTURE" rev-parse HEAD)"

export QUEUE EXTERNAL_MINER_SEEN="$SEEN"
export MAXDIFF=3500 MINDIFF=40 SCAN=500
unset GITHUB_TOKEN

license_for_url() {
	case "$1" in
	*example/fixture*) echo 'MIT' ;;
	*reject/gpl*) echo 'GPL-3.0' ;;
	*reject/agpl*) echo 'AGPL-3.0-only' ;;
	*reject/noassert*) echo 'NOASSERTION' ;;
	*reject/empty*) echo '' ;;
	*) echo 'UNKNOWN-LICENSE' ;;
	esac
}

export -f license_for_url
export LICENSE_RESOLVER=license_for_url

clone_fixture() {
	echo called >>"$CLONE_MARKER"
	mkdir -p "$2"
	git clone -q "$FIXTURE" "$2"
}

export FIXTURE CLONE_MARKER
export -f clone_fixture
export CLONE_CMD=clone_fixture

reset_run() {
	rm -rf "$QUEUE" "$SEEN" "$CLONE_MARKER" "$LOG"
	mkdir -p "$QUEUE"
	: >"$SEEN"
}

queue_count() { find "$QUEUE" -maxdepth 1 -name '*.json' 2>/dev/null | wc -l | tr -d ' '; }

run_miner() {
	local seed=$1
	rm -f "$CLONE_MARKER"
	bash "$MINER" "$seed" "${2:-5}" >>"$LOG" 2>&1
}

# ---- allowlisted MIT: fix commit queued with provenance -----------------------
reset_run
SEED_ALLOW="$SCRATCH/seed-allow.txt"
cat >"$SEED_ALLOW" <<EOF
# curated seed — permissive license only
https://github.com/example/fixture MIT

EOF
run_miner "$SEED_ALLOW"
check 'MIT repo queues one entry' "$(queue_count)" '1'
if [ -s "$CLONE_MARKER" ]; then ok 'MIT repo invokes clone hook'; else bad 'MIT repo must clone'; fi
QUEUE_JSON="$(find "$QUEUE" -maxdepth 1 -name '*.json' | head -n1)"
if [ -n "$QUEUE_JSON" ]; then
	lic="$(jq -r '.source_license' "$QUEUE_JSON")"
	url="$(jq -r '.source_url' "$QUEUE_JSON")"
	reason="$(jq -r '.reason' "$QUEUE_JSON")"
	author="$(jq -r '.author' "$QUEUE_JSON")"
	check 'queue JSON source_license' "$lic" 'MIT'
	if contains "$url" '/commit/'; then ok 'queue JSON source_url has /commit/'; else bad "source_url missing /commit/: $url"; fi
	check 'queue JSON author' "$author" 'git-history'
	check 'queue JSON reason' "$reason" 'ext-fix-commit:example/fixture'
	if contains "$url" "$FIX_SHA"; then ok 'source_url cites full commit sha'; else bad "source_url missing sha $FIX_SHA: $url"; fi
else
	bad 'missing queue JSON for MIT case'
fi

# ---- GPL-3.0 rejected: no clone, no queue ------------------------------------
reset_run
SEED_GPL="$SCRATCH/seed-gpl.txt"
printf '%s\n' 'https://github.com/reject/gpl-repo Go' >"$SEED_GPL"
run_miner "$SEED_GPL"
check 'GPL repo queues nothing' "$(queue_count)" '0'
if [ ! -s "$CLONE_MARKER" ]; then ok 'GPL repo does not invoke clone hook'; else bad 'GPL repo must not clone'; fi
if grep -qF 'skip https://github.com/reject/gpl-repo: license GPL-3.0 not in allowlist' "$LOG"; then
	ok 'GPL skip logged'
else
	bad "GPL skip not logged; log=$LOG"
fi

# ---- AGPL / NOASSERTION / empty all rejected ---------------------------------
for case_name in 'AGPL-3.0-only:reject/agpl-repo' 'NOASSERTION:reject/noassert-repo' 'empty:reject/empty-repo'; do
	lic_label="${case_name%%:*}"
	repo_path="${case_name#*:}"
	reset_run
	seed="$SCRATCH/seed-$lic_label.txt"
	printf '%s\n' "https://github.com/$repo_path TS" >"$seed"
	run_miner "$seed"
	check "$lic_label repo queues nothing" "$(queue_count)" '0'
	if [ ! -s "$CLONE_MARKER" ]; then ok "$lic_label repo does not clone"; else bad "$lic_label repo must not clone"; fi
done

# ---- idempotency: second run adds nothing ------------------------------------
reset_run
run_miner "$SEED_ALLOW"
first_count="$(queue_count)"
run_miner "$SEED_ALLOW"
second_count="$(queue_count)"
check 'first run count' "$first_count" '1'
check 'second run idempotent' "$second_count" "$first_count"

# ---- a non-github/malformed line does not crash the run or drop later seeds ---
# (reviewer finding: mapfile masks parse failure → unbound-var crash under set -u,
# silently dropping every subsequent seed line.)
reset_run
SEED_MIXED="$SCRATCH/seed-mixed.txt"
cat >"$SEED_MIXED" <<EOF
https://gitlab.com/not/github Go
https://github.com/example/fixture React
EOF
run_miner "$SEED_MIXED"
check 'bad line skipped, later github/permissive line still mined' "$(queue_count)" '1'
if grep -qF 'skip https://gitlab.com/not/github: unresolvable host' "$LOG"; then
	ok 'non-github line logged as unresolvable'
else
	bad "non-github line not logged as unresolvable; log=$LOG"
fi

# ---- seen-ledger uses whole-line match: a fork sharing a SHA is not skipped ----
# (reviewer finding: grep -F substring false-positive; owner/repo:sha of one repo
# can be a substring of another's key.)
reset_run
# Pre-seed the ledger with a SUPERSTRING of the fixture's real key.
printf 'x-example/fixture:%s\n' "$FIX_SHA" >"$SEEN"
run_miner "$SEED_ALLOW"
check 'substring ledger match does not falsely skip a distinct repo' "$(queue_count)" '1'

# ---- seedfile ignores comment and blank lines --------------------------------
reset_run
SEED_NOISE="$SCRATCH/seed-noise.txt"
cat >"$SEED_NOISE" <<EOF
# only comments and blanks below until the real row

https://github.com/example/fixture React

EOF
run_miner "$SEED_NOISE"
check 'comment/blank seedfile still queues' "$(queue_count)" '1'

echo '----'
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

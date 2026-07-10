#!/usr/bin/env bash
# Hermetic contracts for the generated docs/issues/INDEX.md (#0141).
# Run from repo root: bash scripts/issues-index.test.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GENERATOR="$REPO_ROOT/scripts/generate-issues-index.sh"
PASS=0
FAIL=0

ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
ISSUES="$TMP/issues"
INDEX="$ISSUES/INDEX.md"
mkdir -p "$ISSUES"

write_issue() {
	local filename=$1 id=$2 title=$3 status=$4 severity=$5 group=$6
	cat >"$ISSUES/$filename" <<EOF
---
id: $id
title: $title
status: $status
severity: $severity
group: $group
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
EOF
}

# Deliberately create out of order. Both YAML quote styles must be unwrapped.
write_issue "0010-later.md" "0010" "'Single-quoted title'" "in-progress" "high" "0009"
write_issue "0002-middle.md" "0002" '"Double-quoted title"' "closed" "low" ""
write_issue "0001-first.md" "0001" "Plain title" "open" "medium" ""

if "$GENERATOR" --issues-dir "$ISSUES" --output "$INDEX"; then
	ok "generator writes an index"
else
	bad "generator failed"
fi

row_1="$(grep -n '^| \[0001\](' "$INDEX" | cut -d: -f1)"
row_2="$(grep -n '^| \[0002\](' "$INDEX" | cut -d: -f1)"
row_10="$(grep -n '^| \[0010\](' "$INDEX" | cut -d: -f1)"
if [ "$row_1" -lt "$row_2" ] && [ "$row_2" -lt "$row_10" ]; then
	ok "rows are ordered by numeric id"
else
	bad "row order is not 0001, 0002, 0010"
fi

if grep -Fqx '| [0001](0001-first.md) | Plain title | open | medium | — | |' "$INDEX"; then
	ok "plain title and empty group are extracted"
else
	bad "plain-title row is wrong"
fi
if grep -Fqx '| [0002](0002-middle.md) | Double-quoted title | closed | low | — | |' "$INDEX"; then
	ok "double-quoted title and status are extracted"
else
	bad "double-quoted-title row is wrong"
fi
if grep -Fqx '| [0010](0010-later.md) | Single-quoted title | in-progress | high | 0009 | |' "$INDEX"; then
	ok "single-quoted title and group are extracted"
else
	bad "single-quoted-title row is wrong"
fi

before="$(sha256sum "$INDEX" | cut -d' ' -f1)"
"$GENERATOR" --issues-dir "$ISSUES" --output "$INDEX"
after="$(sha256sum "$INDEX" | cut -d' ' -f1)"
if [ "$before" = "$after" ]; then
	ok "generation is idempotent"
else
	bad "second generation changed the index"
fi

sed -i 's/status: open/status: closed/' "$ISSUES/0001-first.md"
set +e
check_out="$("$GENERATOR" --check --issues-dir "$ISSUES" --output "$INDEX" 2>&1)"
check_status=$?
set -e
if [ "$check_status" -ne 0 ] && contains "$check_out" "out of date"; then
	ok "check mode detects drift"
else
	bad "check mode did not report drift: $check_out"
fi
if [ "$(sha256sum "$INDEX" | cut -d' ' -f1)" = "$after" ]; then
	ok "check mode does not rewrite drifted output"
else
	bad "check mode rewrote the index"
fi

"$GENERATOR" --issues-dir "$ISSUES" --output "$INDEX"
if "$GENERATOR" --check --issues-dir "$ISSUES" --output "$INDEX"; then
	ok "check mode accepts generated output"
else
	bad "check mode rejected generated output"
fi

# The repository artifact itself must also be current.
if "$GENERATOR" --check; then
	ok "repository issue index is generated from frontmatter"
else
	bad "repository issue index is stale"
fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]

#!/usr/bin/env bash
# Tests for ci-data-only.sh — sources the script (its source-guard stops main
# from running) and drives the pure classifier + main() via the CHANGED_FILES
# bypass, so no real git is needed. Run: bash ci-data-only.test.sh
#
# The contract is FAIL-CLOSED: only a confidently data-only pull_request returns
# "true"; everything else (code touched, push event, empty/garbage, uncertainty)
# returns "false" so the full CI pipeline runs (= status quo). A regression here
# must never let a code change skip the gate, nor hang a required status context.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }

# shellcheck source=/dev/null
. ./ci-data-only.sh

# --- pure classifier: is_data_only(<newline-separated paths>) ---
is_data_only "experience/2026/a.md
experience/2026/b.md" && r=true || r=false
check "only-experience -> data-only"      "$r" true

is_data_only "experience/2026/a.md
internal/server/serve.go" && r=true || r=false
check "experience+code -> NOT data-only"  "$r" false

is_data_only "cmd/twiceshy/main.go" && r=true || r=false
check "code-only -> NOT data-only"        "$r" false

is_data_only "" && r=true || r=false
check "empty -> NOT data-only"            "$r" false

is_data_only "

" && r=true || r=false
check "whitespace-only -> NOT data-only"  "$r" false

# A path that merely CONTAINS experience/ but isn't under it must not pass.
is_data_only "docs/experience-notes.md" && r=true || r=false
check "lookalike path -> NOT data-only"   "$r" false

# --- main(): event gating + bypass ---
out="$(EVENT_NAME=push CHANGED_FILES="experience/2026/a.md" main)"
check "push event -> false (main stays validated)" "$out" false

out="$(EVENT_NAME=pull_request CHANGED_FILES="experience/2026/a.md
experience/2026/b.md" main)"
check "PR + only-experience -> true" "$out" true

out="$(EVENT_NAME=pull_request CHANGED_FILES="experience/2026/a.md
go.mod" main)"
check "PR + mixed -> false" "$out" false

out="$(EVENT_NAME=pull_request CHANGED_FILES="" main)"
check "PR + empty changeset -> false (fail-closed)" "$out" false

out="$(EVENT_NAME= CHANGED_FILES="experience/2026/a.md" main)"
check "no event -> false (fail-closed)" "$out" false

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

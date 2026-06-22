#!/usr/bin/env bash
# Tests for apply-branch-protection.sh — sources the script (its source-guard
# stops the apply logic from running) and unit-tests the pure URL-derivation
# helpers. No real git / network. Run: bash apply-branch-protection.test.sh
#
# Guards the .git-suffix regression: an origin URL ending in `.git` MUST derive
# `owner/name` WITHOUT the suffix, or the Forgejo/GitHub API path 404s. ERE has
# no lazy quantifier, so the old `([^/]+/[^/]+?)(\.git)?` left `.git` attached.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()    { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad()   { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }

# shellcheck source=/dev/null
. ./apply-branch-protection.sh

# repo_from_origin: strip a trailing .git, then take the last owner/name segment.
check "forgejo https + creds + .git" "$(repo_from_origin 'http://claude:tok@192.168.50.244:3030/claude/twiceshy.git')" "claude/twiceshy"
check "forgejo https no .git"        "$(repo_from_origin 'http://192.168.50.244:3030/claude/twiceshy')"               "claude/twiceshy"
check "github https + .git"          "$(repo_from_origin 'https://github.com/dotts-h/twiceshy.git')"                  "dotts-h/twiceshy"
check "github ssh scp-style + .git"  "$(repo_from_origin 'git@github.com:dotts-h/twiceshy.git')"                      "dotts-h/twiceshy"
check "ssh:// + port + .git"         "$(repo_from_origin 'ssh://git@host:22/owner/name.git')"                         "owner/name"

# server_from_origin: scheme://host for https forms, embedded creds dropped.
check "server forgejo + creds + .git" "$(server_from_origin 'http://claude:tok@192.168.50.244:3030/claude/twiceshy.git')" "http://192.168.50.244:3030"
check "server github https"           "$(server_from_origin 'https://github.com/dotts-h/twiceshy.git')"                   "https://github.com"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

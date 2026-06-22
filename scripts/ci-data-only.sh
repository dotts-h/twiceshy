#!/usr/bin/env bash
# ci-data-only.sh — print "true" iff a change touches ONLY corpus data
# (experience/), so the CI workflows can SKIP the heavy Go pipeline for a
# data-only PR while STILL running the job (and posting its required status
# context). The heavy *steps* are gated on this output; the jobs themselves
# always run — a skipped required check posts no status and hangs the PR
# unmergeable (see the "No paths-ignore" note in .forgejo/workflows/*.yml).
# Interim mitigation D of ADR-0021 (#0078): take corpus imports off the code CI.
#
# FAIL-CLOSED by construction: anything uncertain returns "false" (run the full
# pipeline = today's behaviour). Only a confidently data-only pull_request
# returns "true". The worst case is a missed skip — never a skipped gate on a
# code change, never a hung required context.
#
# Inputs (env): EVENT_NAME (github.event_name), BASE_SHA (PR base sha), HEAD_SHA
# (default HEAD). CHANGED_FILES (newline-separated) bypasses git for the unit
# test. Run the tests: bash ci-data-only.test.sh
set -uo pipefail

# Paths whose sole change makes a PR "data-only". Extend deliberately: anything
# the engine's behaviour depends on (code, schema, workflows, scripts, docs,
# go.mod) must NOT be listed, so a PR touching it runs the full gate.
DATA_RE='^experience/'

# Seam (stubbed in the test via CHANGED_FILES): list changed paths base..head,
# one per line. A shallow CI checkout may lack the base commit, so fetch it
# best-effort; any failure yields an empty diff -> fail-closed "false".
changed_files() {
  local base="${BASE_SHA:-}" head="${HEAD_SHA:-HEAD}"
  [ -n "$base" ] || return 0
  git fetch --no-tags --depth=1 origin "$base" >/dev/null 2>&1 || true
  git diff --name-only "$base" "$head" 2>/dev/null || true
}

# Pure: succeed (data-only) iff there is >=1 real path and EVERY real path
# matches DATA_RE. Blank lines are ignored; no real path -> not data-only.
is_data_only() {
  local files="$1" real
  real="$(printf '%s\n' "$files" | grep -vE '^[[:space:]]*$' || true)"
  [ -n "$real" ] || return 1
  printf '%s\n' "$real" | grep -qvE "$DATA_RE" && return 1
  return 0
}

main() {
  # Only PRs are eligible to skip; a push to main always runs the full gate so
  # main stays fully validated.
  [ "${EVENT_NAME:-}" = "pull_request" ] || { echo false; return 0; }
  local files
  if [ -n "${CHANGED_FILES:-}" ]; then files="$CHANGED_FILES"; else files="$(changed_files)"; fi
  if is_data_only "$files"; then echo true; else echo false; fi
}

# Source-guard: the test sources this file and calls the functions directly.
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then main "$@"; fi

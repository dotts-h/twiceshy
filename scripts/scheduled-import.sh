#!/usr/bin/env bash
# scheduled-import.sh — the twiceshy live-feed heartbeat (issue 0022, ADR-0011).
#
# Runs a live importer BOUNDED (so the corpus grows gradually, never a runaway
# dump of the whole feed), commits the new QUARANTINED records to a branch, opens
# a PR, auto-merges it on green, and notifies via ntfy. Records are born
# quarantined; promotion to `validated` is a separate human/harness step (the
# execution-validation harness) — this script never writes a validated record,
# so the PR-as-trust-boundary invariant (ADR-0001 §6) holds.
#
# Idempotent: each run dedups against the corpus (Known advisories are skipped),
# so re-running only adds genuinely-new advisories. Meant to be invoked by a
# systemd timer on the brain; safe to run by hand.
#
# Env knobs:
#   TWICESHY_REPO          DEDICATED import clone (default /home/ori/twiceshy-import).
#                          MUST NOT be a working checkout — this script does
#                          `git reset --hard`, which would discard uncommitted work.
#   TWICESHY_IMPORT_SOURCE importer selector (default osv-live)
#   TWICESHY_IMPORT_LIMIT  max new records per run (default 25; the "no runaway" bound)
#   TWICESHY_AUTOMERGE     1 = auto-merge the PR on green (default), 0 = leave it open
#   NTFY_URL               ntfy topic URL for notifications (optional; skipped if unset)
#   GO                     go toolchain (default /usr/local/go/bin/go)
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-import}"  # dedicated clone; never a working checkout
SOURCE="${TWICESHY_IMPORT_SOURCE:-osv-live}"
LIMIT="${TWICESHY_IMPORT_LIMIT:-25}"
# Stack-first ecosystems for osv-live: npm (React + React Native), PyPI (Python),
# Go — imported each run in this order, then fan out to others. Space-separated;
# each is an exact OSV ecosystem label. Only used when SOURCE=osv-live.
ECOSYSTEMS="${TWICESHY_IMPORT_ECOSYSTEMS:-npm PyPI Go}"
AUTOMERGE="${TWICESHY_AUTOMERGE:-1}"
GO="${GO:-/usr/local/go/bin/go}"
NTFY_URL="${NTFY_URL:-}"

notify() { [ -n "$NTFY_URL" ] && curl -fsS -d "$1" "$NTFY_URL" >/dev/null 2>&1 || true; }

cd "$REPO"
git fetch origin -q && git checkout main -q && git reset --hard origin/main -q && git clean -fdq -- experience/

bin="$(mktemp -d)/twiceshy"
"$GO" build -o "$bin" ./cmd/twiceshy

branch="import/${SOURCE}-$(date -u +%Y%m%d-%H%M%S)"
git checkout -b "$branch" -q

if [ "$SOURCE" = "osv-live" ]; then
  # Stack-first import: one bounded ingest per ecosystem (npm, PyPI, Go, …),
  # accumulating into this branch. A single ecosystem failing (e.g. a network
  # blip) is logged + alerted but does NOT abort the others — a bulk importer
  # makes partial progress rather than failing the whole batch.
  for eco in $ECOSYSTEMS; do
    if out="$("$bin" ingest osv-live -ecosystem "$eco" -limit "$LIMIT" -corpus "$REPO" 2>&1)"; then
      echo "[$eco] $out"
    else
      echo "[$eco] FAILED: $out"
      notify "twiceshy import FAILED (osv-live $eco): $out"
    fi
  done
else
  if ! out="$("$bin" ingest "$SOURCE" -limit "$LIMIT" -corpus "$REPO" 2>&1)"; then
    notify "twiceshy import FAILED ($SOURCE): $out"
    git checkout main -q
    git branch -D "$branch" -q
    exit 1
  fi
  echo "$out"
fi

# git diff misses UNTRACKED files, and freshly-written records are untracked —
# use status (porcelain) so new records are actually detected and committed.
status="$(git status --porcelain -- experience/)"
if [ -z "$status" ]; then
  echo "no new records"; git checkout main -q; git branch -D "$branch" -q; exit 0
fi
n="$(printf '%s\n' "$status" | wc -l | tr -d ' ')"
git add experience/
git commit -q \
  -m "corpus(${SOURCE}): ${n} new quarantined advisory record(s) [scheduled import]" \
  -m "Automated live ${SOURCE} import (issue 0022). Records are quarantined; promotion to validated is a separate human/harness step." \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
sha="$(git rev-parse HEAD)"
git push -q -u origin "$branch"

api="http://192.168.50.244:3030/api/v1/repos/claude/twiceshy"
tok="$(git config --get remote.origin.url | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')"
pr="$(jq -nc --arg t "corpus(${SOURCE}): ${n} new quarantined records [scheduled]" \
        --arg b "Automated live ${SOURCE} import (issue 0022). Quarantined records; the harness/human validates separately." \
        --arg h "$branch" '{title:$t,body:$b,head:$h,base:"main"}' \
      | curl -fsS -X POST "$api/pulls" -H "Authorization: token $tok" -H "Content-Type: application/json" -d @- \
      | jq -r '.number')"

if [ "$AUTOMERGE" = "1" ] && command -v forgejo-ci-merge >/dev/null; then
  forgejo-ci-merge claude/twiceshy "$pr" "$sha" "$REPO" || true
fi
git checkout main -q
notify "twiceshy: imported ${n} new ${SOURCE} records (PR #${pr})"
echo "done: ${n} records, PR #${pr}"

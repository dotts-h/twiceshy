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
# Fetch horizon (#0072 item 4): osv-live fetches the FULL OSV history per ecosystem
# (internal/ingest/osvlive.go pulls <ecosystem>/all.zip — no date window). The LIMIT
# below bounds how many NEW records are written per run, not what is fetched; with
# dedup, "get everything" = run repeatedly until a run adds zero new records (the
# corpus plateaus once caught up). No backfill mode is needed — the horizon is
# already the whole history; raise LIMIT to catch up faster.
#
# Env knobs:
#   TWICESHY_REPO          DEDICATED import clone (default /home/ori/twiceshy-import).
#                          MUST NOT be a working checkout — this script does
#                          `git reset --hard`, which would discard uncommitted work.
#   TWICESHY_IMPORT_SOURCE importer selector (default osv-live)
#   TWICESHY_IMPORT_LIMIT  max new records per run (default 25; the "no runaway" bound)
#   TWICESHY_AUTOMERGE     1 = auto-merge the PR on green (default), 0 = leave it open
#   NTFY_URL               ntfy topic URL for notifications (optional; skipped if unset)
#   TWICESHY_PREFLIGHT_CMD pre-flight gate run on the new records BEFORE the PR is
#                          opened (#0072 item 1); default = the fast corpus-guard
#                          subset. On red, no PR is opened and ntfy fires — never
#                          create an un-mergeable PR that piles up red (lesson exp-0746).
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
# Pre-flight gate: the corpus-guard subset (schema/dup-id via LoadCorpus, the D2
# staleness guard, the push-precision eval) — fast and Docker-free, so it runs on the
# brain. Override to `make test` for the full gate.
PREFLIGHT_CMD="${TWICESHY_PREFLIGHT_CMD:-$GO test ./internal/record/ ./internal/doctor/ ./internal/eval/ -count=1}"

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

# Pre-flight gate (#0072 item 1): run the corpus guard on the committed batch BEFORE
# opening a PR. The PR's CI runs the same gate, so a failure here is a failure there —
# catch it now and DON'T open an un-mergeable PR that sits red and freezes the queue
# (the exp-0746 freeze). Records stay committed on the local branch in the dedicated
# clone for inspection; the next run dedups, so a transient failure self-heals.
if ! gate_out="$(cd "$REPO" && eval "$PREFLIGHT_CMD" 2>&1)"; then
  notify "twiceshy import PRE-FLIGHT FAILED (${n} ${SOURCE} records held, NO PR opened) — inspect branch ${branch} in ${REPO}: $(printf '%s' "$gate_out" | tail -n 3)"
  echo "pre-flight gate failed — not opening a PR:"; printf '%s\n' "$gate_out" | tail -n 20
  git checkout main -q
  exit 1
fi

git push -q -u origin "$branch"

api="http://192.168.50.244:3030/api/v1/repos/claude/twiceshy"
tok="$(git config --get remote.origin.url | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')"
pr="$(jq -nc --arg t "corpus(${SOURCE}): ${n} new quarantined records [scheduled]" \
        --arg b "Automated live ${SOURCE} import (issue 0022). Quarantined records; the harness/human validates separately." \
        --arg h "$branch" '{title:$t,body:$b,head:$h,base:"main"}' \
      | curl -fsS -X POST "$api/pulls" -H "Authorization: token $tok" -H "Content-Type: application/json" -d @- \
      | jq -r '.number')"

git checkout main -q

# Don't swallow the merge result (#0072 item 2, lesson exp-0746): forgejo-ci-merge
# exits 0=merged, 1=CI red (left open), 3=timeout. A left-open PR is exactly the
# silent-stall seed — announce it NOW so it's visible at creation, not only when the
# periodic corpus-stall-alarm catches the pile-up hours later.
if [ "$AUTOMERGE" != "1" ]; then
  notify "twiceshy: imported ${n} new ${SOURCE} records (PR #${pr}) — auto-merge off, PR left open"
elif ! command -v forgejo-ci-merge >/dev/null; then
  notify "twiceshy: imported ${n} new ${SOURCE} records (PR #${pr}) — forgejo-ci-merge unavailable, PR left open"
elif forgejo-ci-merge claude/twiceshy "$pr" "$sha" "$REPO"; then
  notify "twiceshy: imported ${n} new ${SOURCE} records and merged PR #${pr}"
else
  notify "twiceshy: import PR #${pr} (${n} ${SOURCE} records) left OPEN — auto-merge refused (CI red or timeout); needs attention"
fi
echo "done: ${n} records, PR #${pr}"

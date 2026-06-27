#!/usr/bin/env bash
# scheduled-retro.sh — the twiceshy session-retro drain (#0065, ADR-0018).
#
# Sibling of scheduled-import.sh, same trust model: drains the session-retro QUEUE
# (transcripts the SessionEnd hook ships), runs the off-pool analyzer to extract
# candidate lessons, commits the new QUARANTINED drafts to a branch, opens a PR,
# auto-merges on green, notifies via ntfy. Records are born quarantined; promotion to
# `validated` is the separate judge/harness step — so the PR-as-trust-boundary
# invariant (ADR-0001 §6) holds. An analyzer mistake is contained: a bad draft never
# auto-validates.
#
# Where import RE-FETCHES an idempotent feed, retro CONSUMES the queue: retro-intake
# dequeues a transcript only after it is FULLY analyzed, and writes its candidates as
# drafts on the branch. A later PR failure therefore leaves the drafts committed on the
# local `retro/*` branch in the dedicated clone for inspection (the next run dedups by
# fingerprint), exactly as import does — no transcript's lessons are silently lost.
#
# Env knobs:
#   TWICESHY_REPO          DEDICATED retro corpus clone (default /home/ori/twiceshy-retro).
#                          MUST NOT be a working checkout — this script `git reset --hard`s.
#   TWICESHY_RETRO_QUEUE   the brain-local queue the SessionEnd hook writes to
#                          (default /home/ori/twiceshy-retro-queue). A missing dir = empty.
#   TWICESHY_RETRO_URL     analyzer endpoint (the :8729 retro-analyzer shim). Also reads
#   TWICESHY_RETRO_MODEL   the analyzer model (passed through to retro-intake).
#   TWICESHY_RETRO_LIMIT   max new drafts written per run (default 25; the no-runaway bound).
#   TWICESHY_AUTOMERGE     1 = auto-merge the PR on green (default), 0 = leave it open.
#   TWICESHY_RETRO_DRYRUN  1 = analyze + report only; write nothing, dequeue nothing, no PR
#                          (safe verification — pairs with retro-intake -dry-run).
#   NTFY_URL               ntfy topic for notifications (optional).
#   TWICESHY_BIN           prebuilt engine binary (default /home/ori/.local/bin/twiceshy).
#   TWICESHY_FORGEJO_REPO  forge repo PRs target (default claude/twiceshy-corpus).
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-retro}"          # dedicated clone; never a working checkout
QUEUE="${TWICESHY_RETRO_QUEUE:-/home/ori/twiceshy-retro-queue}"
LIMIT="${TWICESHY_RETRO_LIMIT:-25}"
AUTOMERGE="${TWICESHY_AUTOMERGE:-1}"
DRYRUN="${TWICESHY_RETRO_DRYRUN:-0}"
BIN="${TWICESHY_BIN:-/home/ori/.local/bin/twiceshy}"
FORGEJO_REPO="${TWICESHY_FORGEJO_REPO:-claude/twiceshy-corpus}"
NTFY_URL="${NTFY_URL:-}"
NTFY_TOKEN="${NTFY_TOKEN:-}"

notify() { [ -n "$NTFY_URL" ] && curl -fsS ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "$1" "$NTFY_URL" >/dev/null 2>&1 || true; }

[ -n "${TWICESHY_RETRO_URL:-}" ] || { echo "TWICESHY_RETRO_URL required (the :8729 analyzer shim)"; exit 2; }

cd "$REPO"
git fetch origin -q && git checkout main -q && git reset --hard origin/main -q && git clean -fdq -- experience/
git fetch origin main -q || true
BASE_ARGS=()
if git rev-parse --verify -q origin/main >/dev/null; then
  BASE_ARGS=(-base origin/main)
fi

dbtmp="$(mktemp -u).retro.db"
preflight="$BIN index -corpus $REPO -db $(mktemp -u).preflight.db"

# DRY-RUN: analyze + report only (writes nothing, dequeues nothing, no PR). The same
# code path as a real drain up to the write — safe end-to-end verification.
if [ "$DRYRUN" = "1" ]; then
  echo "=== DRY-RUN retro drain (no write, no dequeue, no PR) ==="
  "$BIN" retro-intake -queue "$QUEUE" -corpus "$REPO" -db "$dbtmp" -limit "$LIMIT" -dry-run "${BASE_ARGS[@]}"
  exit 0
fi

branch="retro/capture-$(date -u +%Y%m%d-%H%M%S)"
git checkout -b "$branch" -q

# Drain: analyze each queued transcript into quarantined drafts (dequeued on success).
if ! out="$("$BIN" retro-intake -queue "$QUEUE" -corpus "$REPO" -db "$dbtmp" -limit "$LIMIT" "${BASE_ARGS[@]}" 2>&1)"; then
  notify "twiceshy retro drain FAILED (queue $QUEUE held, nothing dequeued): $(printf '%s' "$out" | tail -n 3)"
  echo "retro-intake failed:"; printf '%s\n' "$out" | tail -n 20
  git checkout main -q; git branch -D "$branch" -q
  exit 1
fi
echo "$out"

# New drafts are untracked — use status (porcelain), not diff, to detect + commit them.
status="$(git status --porcelain -- experience/)"
if [ -z "$status" ]; then
  echo "no new drafts from retro"; git checkout main -q; git branch -D "$branch" -q; exit 0
fi
n="$(printf '%s\n' "$status" | wc -l | tr -d ' ')"
git add experience/
git commit -q \
  -m "corpus(retro): ${n} new quarantined session-derived record(s) [session-retro]" \
  -m "Automated session-retro capture (#0065, ADR-0018). Records are quarantined; promotion to validated is the separate judge/harness step." \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
sha="$(git rev-parse HEAD)"

# Pre-flight: the corpus guard on the committed batch BEFORE opening a PR — a failure
# here is a failure in the PR's CI, so don't open an un-mergeable PR that sits red and
# freezes the queue (exp-0746). Drafts stay on the local branch for inspection.
if ! gate_out="$(cd "$REPO" && eval "$preflight" 2>&1)"; then
  notify "twiceshy retro PRE-FLIGHT FAILED (${n} drafts held, NO PR) — inspect branch ${branch} in ${REPO}: $(printf '%s' "$gate_out" | tail -n 3)"
  echo "pre-flight gate failed — not opening a PR:"; printf '%s\n' "$gate_out" | tail -n 20
  git checkout main -q
  exit 1
fi

git push -q -u origin "$branch"
api="http://192.168.50.244:3030/api/v1/repos/${FORGEJO_REPO}"
tok="$(git config --get remote.origin.url | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')"
pr="$(jq -nc --arg t "corpus(retro): ${n} session-derived quarantined records [session-retro]" \
        --arg b "Automated session-retro capture (#0065, ADR-0018). Quarantined drafts; the judge/harness validates separately." \
        --arg h "$branch" '{title:$t,body:$b,head:$h,base:"main"}' \
      | curl -fsS -X POST "$api/pulls" -H "Authorization: token $tok" -H "Content-Type: application/json" -d @- \
      | jq -r '.number')"
git checkout main -q

# Never swallow the merge result (exp-0746): announce a left-open PR at creation.
if [ "$AUTOMERGE" != "1" ]; then
  notify "twiceshy retro: ${n} new drafts (PR #${pr}) — auto-merge off, PR left open"
elif ! command -v forgejo-ci-merge >/dev/null; then
  notify "twiceshy retro: ${n} new drafts (PR #${pr}) — forgejo-ci-merge unavailable, PR left open"
elif forgejo-ci-merge "$FORGEJO_REPO" "$pr" "$sha" "$REPO"; then
  notify "twiceshy retro: captured ${n} new drafts and merged PR #${pr}"
else
  notify "twiceshy retro: PR #${pr} (${n} drafts) left OPEN — auto-merge refused (CI red or timeout); needs attention"
fi
echo "done: ${n} drafts, PR #${pr}"

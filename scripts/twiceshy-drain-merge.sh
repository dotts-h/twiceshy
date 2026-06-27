#!/usr/bin/env bash
# twiceshy-drain-merge.sh — supervised bulk-drain accelerator (ADR-0021 catch-up).
#
# Merges every open import/* and validate/* PR on the corpus repo whose CI is
# GREEN, overriding the anomaly-hold + the soak veto-window (the batch-level
# HUMAN-oversight gates) for a one-off backlog drain. The per-record judge gate
# and the corpus CI still apply, so only judge-approved, schema-valid batches
# land. Stop the timer to end the drain (then the normal soak/anomaly gates rule
# again). Idempotent + side-effect-light: a tick with no green PR does nothing.
set -uo pipefail
REPO="${TWICESHY_CORPUS_REPO:-claude/twiceshy-corpus}"
CLONE="${TWICESHY_REPO:-/home/ori/twiceshy-corpus}"
API="${TWICESHY_FORGEJO_API:-http://192.168.50.244:3030/api/v1}"
tok="$(git -C "$CLONE" config --get remote.origin.url 2>/dev/null | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')"
[ -n "$tok" ] || { logger -t twiceshy-drain-merge "no token — skip" 2>/dev/null || true; exit 0; }

prs="$(curl -fsS -m 15 "$API/repos/$REPO/pulls?state=open&limit=50" -H "Authorization: token $tok" 2>/dev/null)" || exit 0
printf '%s' "$prs" | python3 -c 'import sys,json
for p in json.load(sys.stdin):
    print(p["number"], p["head"]["ref"], p["head"]["sha"])' 2>/dev/null | while read -r num ref sha; do
  case "$ref" in import/*|validate/*) ;; *) continue ;; esac
  st="$(curl -fsS -m 15 "$API/repos/$REPO/commits/$sha/status" -H "Authorization: token $tok" 2>/dev/null \
        | python3 -c 'import sys,json;print(json.load(sys.stdin).get("state",""))' 2>/dev/null)"
  if [ "$st" != "success" ]; then
    logger -t twiceshy-drain-merge "PR #$num ($ref): CI=$st — wait" 2>/dev/null || true
    continue
  fi
  code="$(curl -s -m 20 -o /dev/null -w '%{http_code}' -X POST "$API/repos/$REPO/pulls/$num/merge" \
            -H "Authorization: token $tok" -H "Content-Type: application/json" -d '{"Do":"merge"}')"
  logger -t twiceshy-drain-merge "PR #$num ($ref): merge http=$code" 2>/dev/null || true
  echo "PR #$num ($ref): CI=success merge=$code"
done

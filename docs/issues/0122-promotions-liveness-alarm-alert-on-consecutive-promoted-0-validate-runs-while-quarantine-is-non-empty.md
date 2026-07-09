---
id: 0122
title: Promotions-liveness alarm: alert on consecutive promoted=0 validate runs while quarantine is non-empty
status: closed
severity: high
group: 
depends_on: []
forgejo: 523
links:
  adr:
  prs: [569]
  issues: []
  regression:
assets: []
---

## Summary

The validate pipeline promoted **zero records for ~5 days (2026-07-01 → 2026-07-06)
while every run stayed green** (anomaly=0, PR opened, auto-merged) and nothing
alerted. Root cause of the freeze itself: the out-of-repo gpt-oss judge shim's
Ollama structured-output schema still enumerated the four pre-#0110 check names, so
constrained decoding could not emit `usefulness` — the model doubled `meaning`/`scope`
instead, the engine's strict parser fail-safed every verdict (`duplicate "meaning"
check in verdict`), each hold triggered the 168h cooldown, and the whole ~3.3k
quarantine backlog froze (recorded as exp-4454). Fixed 2026-07-06 (shim schema +
SYSTEM updated to five checks, holds file cleared).

This issue is the missing net: **nothing in the observability stack treats
"promoted=0 with a non-empty quarantine" as a failure.** The stall alarm watches
branch/CI stalls, the digest reports counts without judging them, and the anomaly
monitor only fires on promote/adapt exit 3. A green run is not a productive run.

## Repro
1. Break the judge so every verdict fail-safes (e.g. reintroduce a stale check enum
   in the shim schema).
2. Let two or more scheduled validate runs complete.
Expected: an ntfy alert like "N consecutive validate runs promoted 0 while X records
sit quarantined — judge pipeline likely broken".
Actual: runs finish green (anomaly=0), digest shows "0 anomaly-flagged", no alert;
the freeze is only found by a human reading run manifests.

## Evidence

- `runs/run-20260705T*.json` … `run-20260706T043413Z-promote.json` on the validate
  clone: `counts: {held: N, ineligible: 1048, promoted: 0}` for every run since
  2026-07-01; latest held reasons ~92% `duplicate "meaning"/"scope" check in verdict`.
- `runs/promote.holds.json` had accumulated 3,298 entries (the whole quarantine).
- Daily digest for the same window: "validate: 6 run(s) logged, 0 anomaly-flagged" —
  technically true, operationally blind.

## Notes

Suggested shape (smallest thing that works, per the metrics-digest/#0116 pattern):

- Extend `scripts/corpus-stall-alarm.sh` (or the digest) to read the last K promote
  manifests (`runs/run-*-promote.json`) and alert when all K have `promoted == 0`
  **and** the corpus has quarantined records eligible for judging (holds/cooldown
  excluded — otherwise an idle-but-healthy system false-positives).
- Consider K=3 (≈12h at the current 4h cadence) to tolerate a single quiet run.
- Optionally: a judge canary in the same script — one live POST to the judge shim
  asserting the verdict parses with exactly the canonical five checks
  (`meaning, scope, usefulness, license, poison`), catching schema/contract drift
  minutes after a deploy instead of days (the exp-4454 class).
- Related: #0116 (metrics digest), #0084 (hold cooldown — the compounding factor),
  exp-0746 ("never silent again" — same lesson, different organ).

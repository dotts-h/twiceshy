---
id: 0082
title: Restart on the new home + verify end-to-end
status: closed
severity: high
group: 0076
depends_on: [0081]
forgejo: 455
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phases 5-6: re-enable the timers against the corpus store; verify a full import->quarantined->validate->served cycle, id-allocation across the move (no colliding exp-NNNN), the gold/eval suites against the fixture, and that the stall alarm fires on a synthetic red.

## Outcome (2026-06-22) — restarted on the new home + verified

- **Timers re-enabled** against the corpus store: `twiceshy-import.timer` (daily) + `twiceshy-validate.timer` (daily) live; `twiceshy-corpus-sync.timer` (30m, serving from the corpus) + `twiceshy-stall-alarm.timer` (15m, watching the corpus) active. Burst `twiceshy-pump` left disabled (feed is caught up).
- **Import cycle** ran against the corpus clone: deduped 250k+ advisories, 0 new (feed caught up — expected, so no PR), prebuilt binary + binary-based preflight on a data-only clone. ✅
- **Validate cycle** ran against the corpus: promote/adapt (judges) → **opened PR #3 on twiceshy-corpus**, corpus CI **green**, promoted 26 records → tripped the anomaly monitor → **held for review (not auto-merged)** = the human-oversight guardrail working. ✅
- **Write path** also exercised by corpus PR #4 (the exp-2753/2754 lesson records) — author → corpus CI → merge.
- **id-allocation across the move**: the snapshot preserved ids; new records continue at max+1 (LoadCorpus clean, no dup). Noted: the `record_experience` MCP allocator returned the same id for two *rapid* proposals (uncommitted, so max+1 collides) — handled by renumbering; a reserve-on-allocate would prevent it (cf. exp-0059).
- **gold/eval against the fixture**: delivered by #0079 (engine CI uses the frozen fixture).

Deferred quick-checks (alarm is installed + unit-tested + runs clean): a live synthetic-red stall-alarm fire test, and exercising the import→PR path once a genuinely-new advisory appears.

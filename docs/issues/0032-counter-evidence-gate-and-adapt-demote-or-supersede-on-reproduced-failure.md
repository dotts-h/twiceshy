---
id: 0032
title: Counter-evidence gate + adapt — demote/supersede on reproduced failure, tighten scope otherwise
status: open
severity: high
group: 0027
depends_on: [0028, 0031]
forgejo:
links:
  adr: ADR-0013
  prs: []
  issues: [0020]
  regression:
assets: []
---

## Summary

The negative direction of ADR-0013, closing the loop: take an outcome report
(#0031), turn it into a repro, and re-run the record's **original repro + the
counter** through the broker (#0018/#0020). Then the diverse judge (#0028) decides:

- claim no longer holds, **or** the counter reproduces → approve a **demotion to
  `stale`** or a **superseding corrected record** (supersede-never-delete);
- counter does **not** reproduce → at most **tighten `applies_to`** (scope /
  near-miss) — **a misapplied lesson never demotes a correct card** (the
  attribution guard).

This is the drafter pipeline run in reverse, reusing the same gate + judge.

## Touches

`internal/repro` (re-run original + counter) + `internal/drafter` (synthesize the
counter-repro / a corrected record) + a `doctor` path + `cmd/twiceshy`. All changes
are per-record deltas via git (ADR-0008), never silent store rewrites.

## Acceptance

- [ ] Report → repro; broker re-runs original + counter; verdict drives the action.
- [ ] Reproduced failure + judge PASS → `stale` or a superseding corrected record,
  with the counter-attestation + verdict in provenance; original is **superseded,
  never deleted**.
- [ ] Non-reproducing report → no demotion; optional `applies_to` tightening only.
- [ ] Demotion rides the ADR-0012 self-merge PR flow (git-audited, CI-gated,
  reversible); non-provable records escalate to a human (ADR-0013 §5).
- [ ] Test-first (stubbed broker + judge); `make ci` green.

## Notes

The attribution guard (reproduce-before-demote) is what keeps the loop healthy vs.
a poisoning vector: a confused or hostile report can only *propose* work, and only
execution-backed counter-evidence can move a card. Pairs with #0029 (the positive
mirror) — same engine, opposite direction.

---
id: 0089
title: record_experience id allocation collides — two drafts in one session get the same exp-NNNN; route through ingest.NextID (exp-0743 stale-id trap)
status: open
severity: medium
group: 
depends_on: []
forgejo: 365
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Two distinct `record_experience` calls in one session both returned `record_id: exp-2758`
— the id allocator handed the same number twice. This is the documented **exp-0743
stale-id trap**: id allocation derived from the frontmatter-max (`maxRecordNum`) of
*committed* records doesn't account for an as-yet-uncommitted draft from earlier in the
same session, so the second novel draft collides. The second draft either overwrites the
first or points at a colliding id.

This is the same class the post-#0086 code-quality audit flagged as a defer item
(`Q-cmd-3 / A-purecore-3`): the record-id drainers reinvent allocation via `maxRecordNum`
instead of the canonical filename-aware `ingest.NextID`. Independent confirmation from a
field session that it bites the `record_experience` path too.

## Repro
1. In one session, call `record_experience` for two distinct novel traps.
Expected: each gets a distinct `exp-NNNN`.
Actual: both returned `exp-2758` (the corpus had 2,757 committed records).

## Evidence
Field report (2026-06-23 RN session): the after-await event-recycling trap and the
react-native-compass-heading RCT-Folly trap both allocated `exp-2758`.

## Notes
Fix: route all id allocation through the canonical `ingest.NextID` (filename-aware disk
scan, not frontmatter-max), and have it account for ids already proposed/written within
the run. Add `record.Num` / `record.FormatID` helpers and consolidate the divergent
`exp-` ↔ int parse/format reimplementations (4+ sites, inconsistent error handling) onto
them, per the audit's `Q-cmd-3 / A-purecore-3`. Guard with a test that two novel drafts
in one session get distinct ids. Promote from "defer" to "do" — it blocks dogfood
capture (#0088).

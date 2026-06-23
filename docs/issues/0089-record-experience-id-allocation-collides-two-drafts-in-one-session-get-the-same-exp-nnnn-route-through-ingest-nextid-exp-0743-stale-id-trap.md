---
id: 0089
title: record_experience id allocation collides — two drafts in one session get the same exp-NNNN; route through ingest.NextID (exp-0743 stale-id trap)
status: closed
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

## Resolution (done 2026-06-23)

Two parts, both now done:

- **The collision (the reported bug)** was fixed in the flywheel-kickoff PR (#362,
  `8f9b76b`): `record_experience`'s `allocateNextID` keeps a locked in-process
  high-water mark (`h.lastID`) so two calls in one server session can't both get the
  committed-max id, and the retro-intake / intake-reports drainers increment per
  allocation within a run. Guarded by `internal/server/server_test.go` ("two
  record_experience calls in one session must allocate distinct ids (#0089)").
- **The consolidation** (this PR — the audit's `Q-cmd-3 / A-purecore-3`): the divergent
  `exp-`↔int reinventions are routed onto the canonical `record.Num` / `record.FormatID`
  — `bumpID` and `maxRecordNum` (cmd/twiceshy), `ingest.NextID`, and the two drainer id
  formats. This removes the inconsistent error handling (e.g. `strconv.Atoi(_)` discarding
  the parse error, and `ingest.NextID`'s bespoke parse/format) so the `record` package owns
  the exp-NNNN grammar in one place. Behavior-preserving; the existing collision / nextid /
  drainer tests stay green.

Unblocks dogfood capture (#0088). The dogfood experience record for the original field-report
trap lives in the corpus data product (twiceshy-corpus, post-ADR-0021), not the engine repo.

---
id: 0141
title: INDEX.md is hand-maintained derived state — generate it from issue frontmatter (or teach next-issue.sh to read frontmatter) so drift is impossible, not just detected
status: open
severity: low
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The 2026-07-08 reconciliation (PR #551) found INDEX.md badly drifted from
issue-file frontmatter: 10 stale status cells, 3 wrong/missing group links,
2 missing rows. Root cause is structural: new-issue.sh writes both places at
creation, but every later lifecycle edit (close-out, reassign, severity bump)
touches only frontmatter, and next-issue.sh reads only INDEX - so the picker
was recommending closed items as BUILD. scripts/issues-index.test.sh now
DETECTS drift in CI; this issue is the deeper fix so drift cannot happen:
either (a) generate INDEX.md from frontmatter (generator + --check mode, the
test then just runs the check), or (b) make next-issue.sh read frontmatter
directly and demote INDEX.md to a cosmetic human view. Option (a) preserves
the human-readable table as an artifact; pick via a short ADR if needed.

## Repro
1. Close any issue by editing only its frontmatter status; run scripts/next-issue.sh.
Expected: the picker reflects the close immediately (no second hand-edit).
Actual: the picker keeps recommending the closed item until INDEX.md is also
hand-edited; only the CI guard (issues-index.test.sh) catches the miss.

## Evidence
- PR #551: 15 drifted/missing cells accumulated across ~5 weeks of close-outs.
- scripts/next-issue.sh parses INDEX tables only; frontmatter is the SSOT per
  docs/issues conventions.

## Notes

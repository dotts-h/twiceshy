---
id: 0073
title: Non-OSV web watchers — changelog/advisory/EOL/deprecation feeds emit quarantined drafts (split from #0072 item 3)
status: open
severity: medium
group: 0015
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0015, 0072, 0023]
  regression:
assets: []
---

## Summary
Split from #0072 item 3 (Horia: "watchers for new info on the web"). The corpus
should ingest beyond OSV: changelog / advisory / EOL / deprecation web feeds. Each
watcher emits **quarantined** drafts through the existing ingest ladder (born
quarantined; promotion is a separate gated step), adjacent to the live
deps.dev/endoflife importer (#0023). At least one non-OSV watcher is the bar.

## Why now
#0072 hardened the pipeline (pre-flight gate, stall alarm, osv full-history horizon)
but deliberately left the new-source breadth to this child so #0072 stays a focused
robustness PR. The plumbing (`ingest` ladder, scheduled-import timer, quarantine
invariant) is now solid enough to add sources safely.

## Acceptance
- [ ] At least one non-OSV web watcher feeds quarantined drafts through the ingest ladder.
- [ ] The watcher is bounded (per-run limit) and dedups against the corpus, like osv-live.
- [ ] Drafts are born `quarantined`; no watcher ever writes a `validated` record.

## Notes
Adjacent to #0023 (deps.dev/endoflife importer). Relates to #0022 (scheduled
importers), #0072 (pipeline hardening this builds on). Source-license/facts-only
provenance (ADR-0003) applies to every ingested draft.

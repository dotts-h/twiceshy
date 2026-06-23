---
id: 0073
title: Non-OSV web watchers — changelog/advisory/EOL/deprecation feeds emit quarantined drafts (split from #0072 item 3)
status: closed
severity: medium
group: 0015
depends_on: []
forgejo:
links:
  prs: [375]
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

## Resolution (done 2026-06-23)

Added the first non-OSV web watcher: an **npm-deprecation** importer (deps.dev was
never implemented; only endoflife shipped from #0023, so npm deprecation is new ground).

- **`internal/ingest/npmlive.go`** (`NpmLiveSource`): checks each seed package's npm
  `/<pkg>/latest` for a `deprecated` flag and emits a quarantined deprecation draft.
  Mirrors `EOLLiveSource` — a `fetch` seam (tests stub it), a curated bounded package
  seed set (unknown packages 404→skip), and the skip-junk rule for a malformed body.
  **Facts-only (ADR-0003 §4):** only the *fact* that the latest version is deprecated +
  the version are used; the maintainer's deprecation message is never reproduced (a test
  enforces this) — the record points at the npm page for the notice + replacement.
- **`twiceshy ingest npm-deprecation`** registered in `importSource`; runs the same
  ingest ladder (dedup, born quarantined, propose-only) as the other importers.
- Verified live: a dry run against the real registry produced 9 quarantined drafts for
  genuinely-deprecated packages (request, node-sass, tslint, …) and skipped the
  non-deprecated one — no message text reproduced.

Acceptance met: ≥1 non-OSV watcher feeds quarantined drafts through the ladder; bounded +
dedup'd like osv-live; never writes validated. Scheduling it on a timer (adding
`npm-deprecation` to the scheduled-import cadence) is an ops step, like the other live importers.

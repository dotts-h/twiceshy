---
id: 0072
title: Corpus-pipeline hardening — importer pre-flight gate + ntfy on failure, red-PR stall alarm (never silent again), web watchers, osv historical-vs-recent fetch
status: open
severity: high
group: 0015
depends_on: []
forgejo: 305
links:
  adr:
  prs: []
  issues: [0015, 0022]
  regression:
assets: []
---

## Summary
The live importer silently froze the corpus for ~12h (root cause #302): imports went red,
`forgejo-ci-merge` correctly refused, ~15 PRs piled up — and **nothing alerted**, because
the importer swallows the merge result (`scheduled-import.sh: forgejo-ci-merge … || true`)
and there is no alarm on a left-open red PR. Harden the pipeline so a failure is **loud,
contained, and self-correcting** — Horia's "harden these / regen with notifications."

## Work items
1. **Importer pre-flight gate.** Before opening an import PR, run the gate (`make test`, or
   at least the corpus-guard subset) on the new records in the import clone. On red, isolate
   the offending record(s), **skip them**, ntfy, and open the PR with the clean subset —
   never create an un-mergeable PR. (= "regen with notifications".)
2. **Red-PR / stall alarm.** ntfy (`NTFY_URL`/`TWICESHY_ALERT_URL`) when any `import/*` or
   `validate/*` PR is left **open-and-red** past a short threshold, and/or when the corpus
   record count has not advanced in N hours. The freeze must never be silent again.
3. **Web watchers** (Horia "watchers for new info on the web"). New corpus sources beyond
   OSV: changelog / advisory / EOL / deprecation web feeds (adjacent to the live
   deps.dev/endoflife importer #0023). Each watcher emits quarantined drafts through the
   existing ingest ladder.
4. **osv historical-vs-recent.** Determine whether `osv-live` fetches a recent window (so it
   plateaus once caught up) vs full history; add a backfill/full-sync mode so "get
   everything" actually pulls the historical set, not just recent.

## Evidence
2026-06-22: corpus frozen at 745 for ~12h, zero alerts; the importer is already broad
(now 14 ecosystems × limit 75 after the unfreeze) so the bottleneck is robustness +
visibility, not breadth.

## Acceptance
- [ ] An import batch that would fail CI is caught **before** the PR and alerted; the clean
      subset still lands.
- [ ] A left-open red PR (or a stalled record count) fires an alert.
- [ ] At least one non-OSV web watcher feeds quarantined drafts.
- [ ] osv-live's fetch horizon is documented; a backfill mode exists if it was recent-only.

## Notes
Relates to #0022 (scheduled importers), #0023 (live deps.dev/endoflife), #302 + #0071 (the
EOL fixes). The stall this hardens against is recorded in exp-0746.

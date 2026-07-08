---
id: 0134
title: wtfjs/wtfpython importer — WTFPL curated gotcha collections as quarantined experience records
status: closed
severity: low
group: 0088
depends_on: []
forgejo: 535
links:
  adr:
  prs: []
  issues: [0133]
  regression:
assets: []
---

## Summary

Small importer for the two big WTFPL-licensed curated gotcha collections —
`denysdovhan/wtfjs` and `satwikkansal/wtfpython` (~37k stars each, licenses
verified 2026-07-07). Each entry is already experience-shaped: a surprising
snippet (symptom), the actual behavior, and an explanation (root cause) —
exactly the JS and Python engineering-trap material the corpus lacks (#0088).
WTFPL imposes no attribution/share-alike obligations, so this is the rare
prose source that is ingest-OK as-is.

## Repro
1. Search the corpus for classic JS/Python traps (`[] + []`, mutable default
   argument, `NaN !== NaN`, late-binding closures).
Expected: records exist.
Actual: none — corpus JS/Python coverage is advisories only.

## Evidence

- License check 2026-07-07: both repos report SPDX `WTFPL` via the GitHub API.
- Counter-examples checked the same day and excluded: `teivah/100-go-mistakes`
  (NOASSERTION — no license, no-go), MIT link-lists (list licensed, linked
  blog content not).

## Close-out (2026-07-08, PR #563)

Shipped. The `wtf` source imports both WTFPL collections as quarantined trap
drafts — live run created **126 records (63 npm + 63 PyPI)**, the JS/Python
trap coverage #0088 targets, born quarantined behind the judge ladder. Hermetic
fixture tests cover both README formats + malformed-skip.

The live run also surfaced and fixed a shared-path defect: ImportBatch aborted
the ENTIRE import on one schema-invalid entry (wtfjs `baNaNa`, a 6-char title),
which is why an early capped run showed 0 Python. Fixed at altitude — a typed
`ingest.ErrInvalidDraft` sentinel that ImportBatch skips+counts+logs while infra
errors still abort (same rule as #0142; dogfooded as exp-4475). Every importer,
including the unattended scheduled ones that previously abort-stalled forever on
one bad feed entry, is now robust. Reviewed clean (Opus reviewer: no bugs on the
shared path, sort panic-safe). Record-quality polish deferred to #0147.

## Notes

- Parse the single-README structure (both repos): section heading → title,
  fenced snippet → symptom/repro, explanation → root_cause; runnable snippets
  become `guard.repro` where practical (node/python one-liners).
- Yield ~10² per repo; provenance-tag source repo + commit SHA; born
  quarantined, panel promotes (same as every importer).
- Follows the adapter pattern of `internal/ingest/nodebreaking.go` (#0115);
  add both to `docs/WEB_SOURCES.md` when built.
- Sibling of #0133 (experience-shaped sources); this one is the small,
  self-contained starter.

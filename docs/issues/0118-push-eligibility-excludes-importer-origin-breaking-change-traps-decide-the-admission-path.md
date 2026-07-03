---
id: 0118
title: Push eligibility excludes importer-origin breaking-change traps — decide the admission path
status: closed
severity: medium
group: 
depends_on: []
forgejo: 497
links:
  adr: docs/adr/ADR-0028-push-eligibility-and-corroborating-specificity.md
  prs: [504]
  issues: [0106, 0115, 0112]
  regression:
assets: []
---

## Summary
ADR-0028 decision 1 excludes `provenance.source.author = twiceshy-importer` from the
push channel — calibrated against the OSV/advisory class (~940/990 validated records,
self-audit material). The new `node-breaking` source (#0115) also stamps
`twiceshy-importer` (the shared `-author` default, `cmd/twiceshy/main.go` importSource
flow), but its records are exactly mid-prompt material: post-cutoff SEMVER-MAJOR
breaking changes a pre-cutoff model gets systematically wrong (verified in the
2026-07-01 smoke: `createCipher` EOL, `console.assert` behavior change). Once these
records validate, the origin filter silently keeps them out of push.

## Options (decide before the first node-breaking records are promoted)
1. **Distinct origin literal per source** — schedule the node-breaking import with
   `-author node-breaking` (flag exists; zero code change). The ADR-0028
   `importerOrigins` list keeps meaning "bulk advisory pipelines". Cheapest; slightly
   bends the origin semantics ("author" becomes a channel label).
2. **Refine eligibility to source-class** — an explicit push-eligible predicate on a
   record class (e.g. a frontmatter field or a per-source allowlist in the index),
   superseding the blanket origin cut. Cleaner; costs a schema/index decision.
3. **Prospector-gated admission (the ADR-0029 endgame)** — importer-origin records
   become push-eligible only when measured model-hard (#0112). The principled cut:
   push serves what is PROVEN to change model behavior. Needs prospector volume first.

## Recommendation
Option 1 now (config-level, reversible), option 3 as the destination once the
prospector produces model-hard tags at volume; option 2 only if a second source makes
the label approach confusing.

## Notes
Found during the #0115 live smoke (2026-07-01). No action needed until node-breaking
records reach `validated`; the promote cadence gives ~days of runway.

## Close-out (2026-07-03, PR #504)
Option 1 adopted (the issue's own recommendation): scheduled-import.sh gains
TWICESHY_IMPORT_AUTHOR (built TDD-first via helm-tdd run 12c602c5 — RED
failed-as-expected, GREEN 1 iteration, mutation 1.0; guarded by
TestScheduledImportAuthorFlag incl. the unset-leaves-argv-unchanged regression),
and scripts/twiceshy-import-node-breaking.{service,timer} schedule a daily
node-breaking import with `-author node-breaking`, keeping those records out of
ADR-0028's importerOrigins cut once validated. Side fix: the ops harness now
isolates TWICESHY_NTFY_ENV, curing the TestNtfyAuthorization failure on hosts
with a real /etc/twiceshy/ntfy.env. Option 3 (prospector-gated admission)
remains the ADR-0029 destination once model-hard tags exist at volume.

---
id: 0118
title: Push eligibility excludes importer-origin breaking-change traps — decide the admission path
status: open
severity: medium
group: 
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0028-push-eligibility-and-corroborating-specificity.md
  prs: []
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

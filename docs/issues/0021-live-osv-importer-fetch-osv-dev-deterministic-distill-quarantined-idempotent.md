---
id: 0021
title: Live OSV importer — fetch osv.dev, deterministic distill, quarantined, idempotent
status: closed
severity: high
group: 0015
depends_on: []
forgejo: 111
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The feed (ADR-0011 phase 2): `OSVLiveSource` implementing `ingest.Source` —
fetch **live** from osv.dev (covers GHSA), distill facts **deterministically**
(OSV metadata → `applies_to` near-1:1; **minimize model-generated prose**, which
carries the only verbatim-reproduction risk). Records born **quarantined** (never
auto-trusted). **Idempotent:** re-running dedups (fingerprint / source_url), not
duplicates. License: distilled facts only; `source_license` / `source_url` set.

## Touches

`internal/ingest` (new live `Source` beside embedded `osvadapter.go`); `Prepare`
pipeline unchanged.

## Acceptance

- [ ] Fetches osv.dev live; deterministic distillation (golden-tested w/ fixtures)
- [ ] Emits quarantined records; re-run is idempotent (no dupes)
- [ ] Output passes the ingestion screen; license-clean provenance recorded
- [ ] **Visible result: record count climbs** when run

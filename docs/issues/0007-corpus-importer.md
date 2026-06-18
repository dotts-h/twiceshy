---
id: 0007
title: Corpus importer — bootstrap quarantined records from license-clean version-knowledge
status: open
severity: high
group: 0008
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0003-corpus-bootstrap-source-scope.md
  prs: []
  issues: [0004]
  regression:
assets: []
---

## Summary
Bootstrap the corpus (extends Phase 0 / #1) from external **license-clean**
structured sources, emitting `quarantined` records via PR (same trust boundary
as the write path, #3). Precision-first. Design:
[docs/design/corpus-bootstrap.md](../design/corpus-bootstrap.md) (appendix has
the original issue draft); decision: ADR-0003.

## Scope (build in slices; first slice is the schema fields)
- [ ] **Schema fields (first slice):** additive `provenance.source_license` (SPDX,
      or `"none (facts only)"`) + `source_url`; extend
      `schema/experience-record.v1.schema.json` (`provenance` is
      `additionalProperties: false`) and `internal/record`. `make ci` asserts they
      are optional and SPDX-shaped. Stays `schema_version: 1`. (ADR-0001 §2, ADR-0003 §4)
- [ ] `twiceshy ingest <source>`: emit one quarantined record per
      `(package, breaking-change)`, deduped by `fingerprint` + `applies_to`. (ADR-0001 §6)
- [ ] **Codemod adapter** (highest yield): wrap a codemod before/after as a
      fail-to-pass `guard.repro`; `kind=fix`/`convention`; derive `applies_to` from
      source→target majors. Promotion still waits on D3 (#4).
- [ ] **GitChameleon adapter:** map visible/hidden tests → `guard`/`guarding_test`.
- [ ] **OSV/GHSA adapter:** `affected[].ranges` → `applies_to`; record per-source
      license; quarantined unless a PoC/FIX yields a guard.
- [ ] **endoflife.date** sidecar → `provenance.valid.until` (feeds D2).
- [ ] **Pack-builder exclusion:** drop copyleft/contract-encumbered `source_license`
      records from commercial packs; emit CC-BY attribution. (ADR-0002 §4)
- [ ] **Near-miss guard:** imported prose/string-match records stay pull-only and
      below the relevance floor — never trade precision for volume.

## Notes
Stack Overflow (option B) is excluded (ADR-0003 §3). Everything imported is born
`quarantined` → pull-only; promotion to `validated` is gated on D3 (#4). The
importer can land any time to grow the pull corpus; no invariant is bent.

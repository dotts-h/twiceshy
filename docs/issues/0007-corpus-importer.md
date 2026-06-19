---
id: 0007
title: Corpus importer — bootstrap quarantined records from license-clean version-knowledge
status: closed
severity: high
group: 0008
depends_on: []
forgejo: 97
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
- [x] **Schema fields (first slice):** additive `provenance.source_license` +
      `source_url`. (PR #27)
- [x] `twiceshy ingest <source>`: emit one quarantined record per
      `(package, breaking-change)`, deduped by `fingerprint` + `applies_to`. (PR #28)
- [x] **Codemod adapter** (data-driven; live tool execution deferred): Go stdlib
      deprecations via embedded curated facts, `staticcheck SA1019` as the
      fingerprintable signature, `kind=fix`. (PR #28)
- [x] **GitChameleon adapter** → shipped as a **Python version-breaking** source
      (`py`): curated, license-clean facts (numpy/pandas/setuptools) keyed on the
      runtime AttributeError/DeprecationWarning. Literal GitChameleon dataset
      ingestion (its tests → guards) is deferred pending its license review + D3. (PR #31)
- [x] **OSV/GHSA adapter:** `affected[].ranges` → `applies_to`; GHSA = CC-BY-4.0
      with attribution; seeded across Maven/npm/PyPI. (PR #29)
- [ ] **endoflife.date** sidecar → `provenance.valid.until` — **deferred to #4
      (doctors)**: it only feeds D2 staleness, so it has no consumer until D2
      exists. Building it now would be machinery with no reader (phase discipline).
- [x] **Pack-builder exclusion:** fail-closed `internal/pack.Classify`; drops
      copyleft/share-alike/NC/ND from commercial packs, emits CC-BY attribution. (PR #30)
- [x] **Near-miss guard:** imported records born quarantined (pull-only) + one
      record per primary signature; guarded by tests. (PR #30)

## Notes
Stack Overflow (option B) is excluded (ADR-0003 §3). Everything imported is born
`quarantined` → pull-only; promotion to `validated` is gated on D3 (#4). The
importer can land any time to grow the pull corpus; no invariant is bent.

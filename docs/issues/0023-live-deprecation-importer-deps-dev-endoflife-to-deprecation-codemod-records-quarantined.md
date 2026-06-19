---
id: 0023
title: Live deprecation importer — deps.dev/endoflife to deprecation+codemod records, quarantined
status: open
severity: medium
group: 0015
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

ADR-0011 phase 3 — the most testable, cleanest live source: deps.dev /
endoflife.date / changelogs → **deprecation + codemod** records, quarantined.
Extends the existing embedded `deprecation.go` adapters to live fetch.

**Partial coverage (2026-06-19):** the *facts* this issue would import already
exist as a curated, license-clean embedded set
(`internal/ingest/data/go-deprecations.yaml`, the `go` source). A one-off
`twiceshy ingest go` run landed them as quarantined records (exp-0043 io/ioutil,
exp-0044 strings.Title, exp-0045 math/rand) so the #0026 drafter pipeline had
real records to prove against. The remaining scope of *this* issue is the **live**
deps.dev / endoflife.date fetch (auto-refreshing the set rather than the one-off
embedded seed) — it stays **open** for that.

## Touches

`internal/ingest/deprecation.go` + new live sources.

## Acceptance

- [ ] Live deps.dev / endoflife fetch; deterministic distillation; quarantined
- [ ] Idempotent; license-clean; passes the screen; test-first

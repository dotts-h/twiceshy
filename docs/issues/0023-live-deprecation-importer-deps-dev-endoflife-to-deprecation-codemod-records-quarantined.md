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

## Touches

`internal/ingest/deprecation.go` + new live sources.

## Acceptance

- [ ] Live deps.dev / endoflife fetch; deterministic distillation; quarantined
- [ ] Idempotent; license-clean; passes the screen; test-first

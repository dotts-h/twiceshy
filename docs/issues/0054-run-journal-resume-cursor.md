---
id: 0054
title: "Run journal / resume cursor"
status: open
severity: medium
group: 0034
depends_on: [0036]
forgejo: 144
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

A mid-corpus abort leaves an unmarked partial working tree. Write a machine-readable 'promoted X,Y; stopped at Z because <err>' marker so the next run resumes rather than re-walking.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §D4.

## Touches

`cmd/twiceshy/main.go` promoteCorpus/adaptCorpus (journal per action + on abort); reuse the #0036 JSON.

## Acceptance

- [ ] After a forced mid-run abort, a journal records what was done and where it stopped; the next run resumes.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).

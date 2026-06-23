---
id: 0091
title: Authoring-scaffold CLI — twiceshy author pre-stages a record + repro skeleton
status: open
severity: low
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

Convenience tooling for the authoring path (#0024): a `twiceshy author` command
that pre-stages a record skeleton (SCHEMA.md frontmatter) plus a positive/negative
repro skeleton under `experience/repro/`, so authoring a §5-clean record is a
fill-in-the-blanks flow. The path works without it (see
[docs/AUTHORING.md](../AUTHORING.md)); this reduces friction for the
corpus-seeding campaign (#0088).

## Notes

- Pre-fills provenance for an authored record: `source.author`, `source_license:
  none (authored, internal-only)`, no `source_url`.
- Not blocking; pure ergonomics over the existing record + repro shape.

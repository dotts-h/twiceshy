---
id: 0091
title: Authoring-scaffold CLI — twiceshy author pre-stages a record + repro skeleton
status: closed
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

## Resolution (done 2026-06-23)

Built `twiceshy author` as a pure core + thin CLI:

- **`internal/author.Scaffold(params, now)`** (pure): validates id / slug / title / kind /
  author, then returns the files to write — a quarantined, §5-clean record skeleton
  (placeholder symptom / resolution / applies_to, authored-internal provenance pre-filled,
  no `source_url`) + a positive repro `.sh` skeleton (fail-to-pass discipline), plus a
  negative one with `-with-negative`. The record **parses as a valid quarantined draft**, so
  the author fills in the blanks and runs `doctor revalidate` immediately. 97% covered.
- **`twiceshy author -id exp-NNNN -slug <slug> -title <title> [-kind] [-author] [-corpus]
  [-with-negative]`**: writes the files under `-corpus`, **refuses to overwrite** (all-or-
  nothing), makes `.sh` files executable, and prints the next steps (revalidate + similarity).
- Docs: AUTHORING.md lists `twiceshy author` (+ `twiceshy similarity`, #0090) under Tooling;
  CODEBASE_MAP lists the new package + subcommand.

Pure ergonomics over the existing record + repro shape (the authoring path works without it);
reduces friction for the #0088 seeding campaign.

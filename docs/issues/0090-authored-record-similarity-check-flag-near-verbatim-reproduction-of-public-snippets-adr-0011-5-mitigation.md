---
id: 0090
title: Authored-record similarity check — flag near-verbatim reproduction of public snippets (ADR-0011 §5 mitigation)
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

Optional mitigation from ADR-0011 §5 (residual risk: near-verbatim reproduction).
A check that flags an authored draft whose text is suspiciously close to a known
public snippet/phrase **before** promotion — an extra net, never the primary
control (author-from-spec discipline is). Follow-up to the authoring path (#0024);
see the canon [docs/AUTHORING.md](../AUTHORING.md) → "Residual risk + mitigations".

## Notes

- Scope: a cheap textual-similarity flag (n-gram / shingling, stdlib), surfaced as
  a *lead* for human review, not an auto-reject.
- Local-model assist allowed as a flagger, never a judge (ADR-0011 §8).
- Not blocking: §5 authoring ships without it; this hardens it.

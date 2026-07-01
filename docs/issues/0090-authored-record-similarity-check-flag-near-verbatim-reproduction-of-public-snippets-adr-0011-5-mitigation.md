---
id: 0090
title: Authored-record similarity check — flag near-verbatim reproduction of public snippets (ADR-0011 §5 mitigation)
status: closed
severity: medium
group: 0015
depends_on: []
forgejo: 460
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

## Resolution (done 2026-06-23)

Built the optional §5 net as a pure core + a thin CLI:

- **`internal/similarity`** (pure, stdlib): word-shingle (n-gram) sets and
  `Assess(draft, ref, n)` → containment (fraction of the draft's distinct shingles
  found in the reference — the plagiarism direction, undiluted by reference size) plus
  the matching n-gram passages. `DefaultN` = 5. 91% covered.
- **`twiceshy similarity -record <rec> -against <ref>… [-n] [-threshold]`**: parses the
  record and compares its **authored prose** (title / symptom / root_cause / fix /
  dead_ends / body — *not* error signatures or the original repro code, which is ours by
  construction + execution, §5 mitigation 2) against each reference, prints the matching
  phrases, and gives an advisory verdict. Always exits 0 — a LEAD for human review, never
  an auto-reject (the issue's explicit scope). `-against` is repeatable; the record path
  is normalized to its corpus-relative `experience/…` tail.
- Docs: AUTHORING.md mitigation 4 now points at the built check; CODEBASE_MAP lists the
  new package + subcommand.

No corpus of "public snippets" exists to auto-compare against (and authored records carry
no `source_url`), so the reviewer supplies the suspected source — faithful to "an extra net
behind author-from-spec discipline," which stays the primary control.

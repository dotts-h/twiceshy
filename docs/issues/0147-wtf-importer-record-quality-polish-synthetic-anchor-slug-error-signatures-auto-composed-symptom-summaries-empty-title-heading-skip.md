---
id: 0147
title: wtf importer record-quality polish: synthetic anchor-slug error_signatures, auto-composed symptom summaries, empty-title heading skip
status: closed
severity: low
group: 0088
depends_on: []
forgejo: 590
links:
  adr:
  prs: [572]
  issues: [0088, 0134]
  regression:
assets: []
---

## Summary
The #0134 wtf importer ships working (126 records live), but the drafts it
produces have three low-severity quality quirks a reviewer must fix before
validation. These do not block ingestion (records are born quarantined and
gated by the judge/human), but each dilutes record quality:
1. **Synthetic error_signatures**: entries carry `error_signatures:
   [wtfjs:<anchor-slug>]` (e.g. `wtfjs:-is-equal-`) — used as the dedup
   BatchKey, but not a real error signature. Most of these behavioral gotchas
   produce NO error output; the field should be empty/null with dedup keyed on
   a title/URL fingerprint instead, so a validated record never pollutes the
   push channel's error-token matching.
2. **Auto-composed symptom summaries** read awkwardly (e.g. "`[]` is equal
   `![]`: Array is equal not array") — acceptable but worth a cleaner
   title/summary derivation.
3. **Empty-title headings** (`## \n`) are not filtered by wtfjsSkipTitle; they
   currently fall through to record.Validate and are counted as Invalid (the
   #0134 batch safety net catches them, no crash) — but pre-filtering them at
   the parser is cleaner than relying on the downstream guard (reviewer note on
   PR #563).

## Repro
1. `twiceshy ingest wtf`, inspect a produced record's `error_signatures`.
Expected: empty (no real error) or a genuine signature.
Actual: a synthetic anchor slug like `wtfjs:-is-equal-`.

## Evidence
- Live record exp-0002 (scratch import): `error_signatures: [wtfjs:-is-equal-]`,
  `symptom.summary: '\`[]\` is equal \`![]\`: Array is equal not array'`.
- PR #563 reviewer note: empty-title headings rely on the downstream
  record.Validate guard rather than wtfjsSkipTitle.

## Acceptance
- Dedup no longer requires a fake error_signature; behavioral-gotcha records
  carry empty error_signatures (or a real one when the entry has error output).
- Empty/whitespace-only heading titles are skipped at the parser (counted).
- symptom.summary derivation reviewed for readability.

## Notes

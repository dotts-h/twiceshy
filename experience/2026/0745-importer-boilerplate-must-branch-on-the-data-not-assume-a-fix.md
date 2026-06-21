---
schema_version: 1
id: exp-0745
kind: trap
status: quarantined
title: A transcriber's boilerplate must branch on the actual data — don't assert a fix that the source doesn't have
symptom:
    summary: 'A feed importer that renders the same boilerplate remediation for every record emits self-contradictory records when the boilerplate assumes a field the source can leave empty. twiceshy''s OSV live importer wrote resolution.fix = "Upgrade affected packages past the fixed version" for EVERY advisory — including the fixed:null ones (no fixed version published), where that text references a version that does not exist. It was the largest #0061 transcription-defect class (10 of 19 audited rejects).'
    error_signatures:
        - importer emits "upgrade past the fixed version" for a fixed:null advisory
        - distilled record asserts a fix the source does not publish
applies_to:
    - ecosystem: Go
      package: github.com/dotts-h/twiceshy
resolution:
    root_cause: 'The fix-text was an unconditional template (`fmt.Sprintf("Upgrade ... past the fixed version ...")`) rather than a function of the advisory''s actual data. OSV''s `fixed` is legitimately null for an unpatched vuln, so the template asserted a non-fact on every such record. The judge held them (ADR-0016), but they polluted the quarantine queue and would be poison if a lenient judge approved them.'
    fix: 'Branch the boilerplate on the data: render "upgrade past the fixed version" only when an affected range actually carries a fixed version; otherwise render "no fix is published yet". General rule for any importer/transcriber: every templated claim must be conditional on the field it references existing — a generator that asserts more than the source contains manufactures contradictions. The source-side complement of exp-0744 (which made the JUDGE see the contradiction); this stops the importer creating it.'
guard:
    repro: null
    guarding_test: 'internal/ingest/osvlive_test.go::TestOSVLiveSource_NoFixedVersionFixText — a fixed:null advisory must not claim a fixed version exists; a fixed advisory still advises upgrading past the fix.'
provenance:
    source:
        author: claude
        session: twiceshy-get-next-2026-06-21
        pr: null
    recorded_at: "2026-06-21"
    validated_at: null
    valid:
        from: "2026-06-21"
        until: null
    superseded_by: null
---

## What happened
`internal/ingest/osvlive.go` rendered `resolution.fix` from a fixed string —
`"Upgrade affected packages past the fixed version; see <url>."` — for every
distilled OSV advisory. For the `fixed:null` advisories (OSV's valid encoding of
"no patch yet") that text references a fixed version that does not exist, so the
record claims to be unfixed and fixed at once. A 2026-06-20 Sonnet audit found this
in 10 of 19 rejects (#0061 Defect 3, the largest class).

## The fix
`osvLiveFixText(applies, sourceURL)` branches: "upgrade past the fixed version" only
when an affected range carries a fixed version, else "no fix is published yet".

## The general lesson
A transcriber's templated claims must be **conditional on the data they reference**.
"Upgrade past the fixed version" is a fact only when a fixed version exists; asserting
it unconditionally manufactures a contradiction on every record where it doesn't.
Pairs with [[0744-llm-judge-cannot-catch-a-contradiction-it-cannot-see]] (the judge
side of the same defect).

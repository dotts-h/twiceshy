---
schema_version: 1
id: exp-0744
kind: trap
status: quarantined
title: An LLM judge can only catch a contradiction it can SEE — render null/absent fields explicitly in the prompt
symptom:
    summary: 'An LLM-judge prompt that omits a field when its value is null forces the judge to detect a defect from the ABSENCE of a line rather than its presence — and cheap models reliably miss that. twiceshy''s BuildAdvisoryPrompt rendered "  fixed: X" only when a fixed version existed, so a fixed:null advisory showed no fixed line at all. The off-pool advisory judge (gpt-oss/gemini) then could not catch the "fix text says upgrade past the fixed version, but none was ever published" contradiction — the single largest defect class a full-file Sonnet audit found (10 of 19 rejects), because the audit read the raw YAML and the production judge read only the rendered prompt.'
    error_signatures:
        - cheap advisory judge approves a fixed:null record with upgrade-past-fix boilerplate
        - judge prompt omits a line for a null field
applies_to:
    - ecosystem: Go
      package: github.com/dotts-h/twiceshy
resolution:
    root_cause: 'A judge sees only its prompt, not the underlying record. Rendering a field conditionally ("emit the line only when non-null") makes a null value INDISTINGUISHABLE from a field the renderer simply skipped — the judge must infer the defect from what is missing, which is exactly the inference cheap models are worst at. The full-file Sonnet audit caught these because it read the explicit `fixed: null` in the YAML; the production judge was structurally blinded.'
    fix: 'Make the field TOTAL in the prompt: always render it, substituting an explicit marker for the empty case (e.g. `fixed: (none published)` when no fixed version exists). The contradiction is then visible on the line, not inferable from its absence. General rule for LLM-judge prompts: never let "absent in the prompt" stand in for "null in the data" — a judge cannot reject what it cannot see.'
guard:
    repro: null
    guarding_test: 'internal/judge/judge_test.go::TestBuildAdvisoryPrompt_MarksMissingFixedVersion — asserts a fixed:null record renders "fixed: (none published)" and a real fixed version still renders verbatim without the marker.'
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
`internal/judge/model.go` `BuildAdvisoryPrompt` emitted `  fixed: %s` only inside
`if at.Versions.Fixed != nil`. A `fixed: null` advisory therefore rendered **no**
`fixed:` line. The cheap off-pool judge, seeing only the prompt, could not reliably
flag the common boilerplate contradiction ("upgrade past the fixed version" with no
fixed version in existence). A 2026-06-20 full-file Sonnet audit found this was the
largest single reject class (#0061 Defect 3, 10/19) — it caught them only because it
read the raw YAML.

## The fix
Render `fixed:` as a total field: the version when present, else
`fixed: (none published)`. The judge now sees an explicit "no fix" line and can match
it against the fix text.

## The general lesson
A judge is a function of its prompt, not of the record behind it. Any field rendered
"only when present" turns a null into silence, and silence is the thing weak judges
overlook. Render null explicitly. (Same family as exp-0098: give the gate/judge a
structural signal it can act on, not one it must infer.)

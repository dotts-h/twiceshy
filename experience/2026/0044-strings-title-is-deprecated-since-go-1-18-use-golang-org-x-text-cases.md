---
schema_version: 1
id: exp-0044
kind: fix
status: quarantined
title: strings.Title is deprecated since Go 1.18 — use golang.org/x/text/cases
symptom:
    summary: staticcheck flags strings.Title as deprecated; its word-boundary rule mishandles Unicode.
    error_signatures:
        - 'SA1019: strings.Title is deprecated: The rule Title uses for word boundaries does not handle Unicode punctuation properly.'
applies_to:
    - ecosystem: Go
      package: strings
      versions:
        introduced: "1.18"
        fixed: null
resolution:
    root_cause: strings.Title uses a naive ASCII word-boundary rule that is wrong for Unicode punctuation and is not locale-aware.
    fix: Use cases.Title from golang.org/x/text/cases with an explicit language tag for correct, locale-aware title casing.
provenance:
    source:
        author: twiceshy-importer
        session: null
        pr: null
    recorded_at: "2026-06-19"
    validated_at: null
    valid:
        from: "2026-06-19"
        until: null
    source_license: none (facts only)
    source_url: https://pkg.go.dev/strings#Title
    superseded_by: null
---

Go 1.18 deprecated strings.Title because its word-boundary handling is
incorrect for Unicode. staticcheck SA1019 flags it. Use
golang.org/x/text/cases.Title with an explicit language.Tag instead.

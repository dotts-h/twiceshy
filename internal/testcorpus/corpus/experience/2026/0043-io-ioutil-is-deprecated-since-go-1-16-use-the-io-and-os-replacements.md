---
schema_version: 1
id: exp-0043
kind: fix
status: validated
title: io/ioutil is deprecated since Go 1.16 — use the io and os replacements
symptom:
    summary: staticcheck flags io/ioutil functions as deprecated; the package was superseded in Go 1.16.
    error_signatures:
        - 'SA1019: ioutil.ReadAll is deprecated: As of Go 1.16, this function simply calls io.ReadAll.'
applies_to:
    - ecosystem: Go
      package: io/ioutil
      versions:
        introduced: "1.16"
        fixed: null
resolution:
    root_cause: Go 1.16 moved io/ioutil's functions into io and os; the package now exists only for backward compatibility.
    fix: Replace ioutil.ReadAll/ReadFile/WriteFile/Discard with io.ReadAll, os.ReadFile, os.WriteFile, io.Discard; replace ioutil.TempFile/TempDir with os.CreateTemp/MkdirTemp.
guard:
    repro: null
    repros:
        - path: experience/repro/exp-0043-io-ioutil
          kind: positive
          label: auto-generated go-deprecation-template repro
    guarding_test: null
provenance:
    source:
        author: twiceshy-importer
        session: null
        pr: null
    recorded_at: "2026-06-19"
    validated_at: "2026-06-19"
    valid:
        from: "2026-06-19"
        until: null
    source_license: none (facts only)
    source_url: https://go.dev/doc/go1.16#ioutil
    superseded_by: null
    promotion:
        attested_at: "2026-06-19T15:13:38Z"
        reproduced_under:
            - go1.25
        judge_model: gpt-oss:20b
        judge_decision: approve
---

Go 1.16 deprecated the entire io/ioutil package: each function gained a
direct replacement in io or os. Code still compiles, but staticcheck SA1019
flags every call. Migrate to the io/os equivalents — behavior is identical.

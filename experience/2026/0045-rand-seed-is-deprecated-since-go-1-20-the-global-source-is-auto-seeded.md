---
schema_version: 1
id: exp-0045
kind: fix
status: validated
title: rand.Seed is deprecated since Go 1.20 — the global source is auto-seeded
symptom:
    summary: staticcheck flags math/rand Seed as deprecated; programs no longer need to seed the global source.
    error_signatures:
        - 'SA1019: rand.Seed is deprecated: As of Go 1.20 there is no reason to call Seed with a random value.'
applies_to:
    - ecosystem: Go
      package: math/rand
      versions:
        introduced: "1.20"
        fixed: null
resolution:
    root_cause: Go 1.20 auto-seeds the global math/rand source randomly, so a manual Seed call is unnecessary and can reduce randomness.
    fix: Remove rand.Seed(time.Now().UnixNano()); when an explicitly seeded generator is required, use rand.New(rand.NewSource(seed)).
guard:
    repro: null
    repros:
        - path: experience/repro/exp-0045-math-rand
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
    source_url: https://go.dev/doc/go1.20#math/rand
    superseded_by: null
    promotion:
        attested_at: "2026-06-19T15:14:45Z"
        reproduced_under:
            - go1.25
        judge_model: gpt-oss:20b
        judge_decision: approve
---

Go 1.20 made the top-level math/rand functions use a randomly-seeded global
source, so calling rand.Seed is now deprecated (staticcheck SA1019) and only
makes output less random. Drop the Seed call, or use a local rand.New source
when determinism is required.

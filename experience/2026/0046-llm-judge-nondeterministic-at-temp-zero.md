---
schema_version: 1
id: exp-0046
kind: convention
status: quarantined
title: "An LLM judge is non-deterministic at temperature 0 — gate a prompt on a labelled eval, not one run"

applies_to:
  - ecosystem: "Ollama"
    package: "gpt-oss:20b"
  - ecosystem: "twiceshy"
    package: "internal/judge"

resolution:
  root_cause: >
    A "diverse-model judge" (twiceshy's gpt-oss:20b approver, ADR-0013) gives
    DIFFERENT verdicts on byte-identical input across runs even at
    temperature 0 — sampling is not the only entropy source (batching,
    KV-cache and kernel non-determinism remain). The disagreement
    concentrates on the hard boundary cases: in the judge-prompt A/B
    (internal/judgeeval), the AGPL-encumbered license record false-approved
    ~1 time in 7 samples under the shipped prompt while being rejected the
    other 6. A single run therefore cannot rank two prompts, and a single
    production verdict on a borderline record is a coin-flip on the very
    cases that matter most (the fail-unsafe direction: a bad record
    auto-promoting).
  fix: >
    Gate the judge prompt on a hand-labelled gold set with `twiceshy
    judge-eval`, scoring false-approve rate over repeat=N samples (majority
    vote), not a single pass — re-run it after any prompt or model change.
    For higher-stakes promotion, sample the production judge repeat+majority
    too, so one unlucky roll cannot promote an encumbered/poison record. Keep
    the system prompt in version control (judge.Config.System) so the
    evaluated artifact is exactly what ships.
  dead_ends:
    - tried: "picking the better judge prompt from a single eval run (repeat=1)"
      why_it_failed: >
        The same prompt scored 0 false-approves in one repeat=1 run and 1
        false-approve (the AGPL case) in another — pure run-to-run noise. The
        repeat=5 majority run was needed to separate signal from the model's
        boundary non-determinism.
    - tried: "turning on the model's reasoning pass (think=true) to sharpen the judge"
      why_it_failed: >
        It made the judge WORSE, not better — it introduced a false-approve
        on a CC-BY-SA license case in both prompt variants, and is markedly
        slower. Reasoning is not a free accuracy lever for a strict-rubric
        classification.

guard:
  repro: null
  guarding_test: null

provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-19
  validated_at: null
  valid: { from: 2026-06-19, until: null }
  source_license: "none (facts only)"
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

twiceshy's autonomous-promotion loop (ADR-0013) leans on a diverse-model judge
to decide what a green execution gate cannot: whether a proven record is
meaningfully, correctly-scoped, license-clean, and non-misleading. Building the
judge-prompt eval (`internal/judgeeval`, a 27-case gold set + a false-approve
scorer) surfaced that the local judge model (gpt-oss:20b) is **not deterministic
at temperature 0** on the boundary cases — the AGPL license record was approved
in 1 of 7 samples under the shipped prompt, rejected in the other 6.

The practical consequences: (1) **rank prompts on repeat-N majority, never one
run** — the same prompt's false-approve count flipped 0↔1 between two repeat=1
runs. (2) **`think=true` is not a free win** — it added false-approves and is
slower. (3) Over-rejection is the safe direction (records stay quarantined for a
human), so the residual fragility is bounded, but for higher-stakes promotion the
production judge should itself sample repeat+majority so one unlucky roll cannot
auto-promote an encumbered or poison record. Measure the gate; do not trust a
single LLM verdict.

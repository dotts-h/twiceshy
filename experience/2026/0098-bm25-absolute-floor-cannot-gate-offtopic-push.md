---
schema_version: 1
id: exp-0098
kind: trap
status: quarantined
title: A BM25 absolute relevance floor cannot gate off-topic injection — use a discriminative low-df term
symptom:
    summary: 'A deterministic injection/push channel that gates retrieval on an absolute BM25 score floor injects on near-any prompt: off-topic prose ("write a haiku about cats", "what time is it in Tokyo") scores as high as a weak genuine hit, so a fixed threshold either passes everything or drifts as the corpus grows. The result is noise injected on every prompt, which trains agents to ignore the channel.'
    error_signatures:
        - push injects cards on off-topic prompts
        - relevance floor passes every match
applies_to:
    - ecosystem: Go
      package: modernc.org/sqlite
resolution:
    root_cause: 'BM25 magnitude is corpus-relative, not an absolute scale: its IDF and length-normalization terms shift with document count and average length (the same query scored ~8e-6 on a 2-record index and 2-21 on a 72-record one), and off-topic stopword/prose OR-noise scores in the same band as weak on-topic hits. So no single magnitude literal separates off-topic from on-topic, and any tuned value is obsolete one corpus generation later.'
    fix: 'Gate on document frequency instead of magnitude. Inject only when the query carries a DISCRIMINATIVE token — a content token present in 1..maxDF VALIDATED records (stopwords and corpus ecosystem-names excluded), maxDF = max(2, ceil(0.25*nValidated)) so it scales with the corpus — then search that discriminative subset at a small positive floor. Every off-topic token is df=0 or generic (df>=3); every genuine error query carries a df<=2 token. Structural and scale-free. Keep a fingerprint-exact bypass (a deterministic stack signature is real by construction).'
guard:
    repro: null
    guarding_test: internal/index/retrievepush_test.go::TestRetrievePushPrecisionRecall — runs against the live corpus; asserts off-topic prompts inject 0 cards and on-topic error prompts inject the correct record id with the weak noise card absent. Fails loudly if corpus growth closes the df gap.
provenance:
    source:
        author: claude
        session: twiceshy-adoption-2026-06-20
        pr: null
    recorded_at: "2026-06-20"
    validated_at: null
    valid:
        from: "2026-06-20"
        until: null
    superseded_by: null
---

## What happened
twiceshy's push channel (`POST /push`, the deterministic injection path) used an
absolute BM25 relevance floor (`DefaultFloor = 2.0e-06`). Probed live, it returned
2–3 trap cards for *every* prompt — including *"write a haiku about cats"* and
*"what time is it in Tokyo"*. Injecting noise on every prompt is worse than nothing:
it trains the agent to ignore the channel and burns context tokens, so adoption was zero.

## Why a bigger floor doesn't fix it
Raising the floor was measured dead on the live corpus. BM25 magnitude does not
separate the classes: off-topic *"buy milk"* topped 9.62 and *"capital of France"*
7.49, while a genuine *"llm judge"* hit scored 1.57 — the bands overlap, so no flat
cut splits them. And BM25 is corpus-relative (IDF + length-norm shift with size), so
any tuned number drifts: the same query scored ~8e-6 on a 2-record test index and
2–21 on the live one. A magic threshold is obsolete one corpus generation later.

## The fix: discriminative-term gate
Document frequency separates cleanly. Every off-topic content token is absent from
the validated corpus (df=0) or generic (df≥3); every genuine error query carries a
rare token (`fts5`=2, `tmpdir`=1, `bm25`=1, `sse`=1). Gate on "does the query carry a
token in 1..maxDF validated records?" — a structural, scale-free question — then
floor the discriminative subset. Measured: off-topic 8/8 → 0, on-topic 8/8 → correct.

## Dead ends
- Absolute BM25 floor (any value): bands overlap; drifts with corpus size. Rejected.
- Relative top/second-hit ratio: a rare token isolated in one record mimics a real
  match, and near-duplicate clusters suppress true positives. Fragile.

See ADR-0015. The df gate is a calibrated stopgap until ADR-0006 score-banding lands.

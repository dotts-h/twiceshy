---
schema_version: 1
id: exp-0622
kind: trap
status: validated
title: "Low document-frequency in a small corpus is not token specificity — a df-gated push gate leaks common words"

symptom:
  summary: >
    A relevance gate that treats a query token as "discriminative" when it has
    low document frequency in a small curated corpus injects unrelated results
    for off-domain inputs. Common dev/web vocabulary ("http", "request",
    "method", "version", "cache", "permission") has low df only because the
    corpus is tiny, not because it is specific — so an off-domain prompt
    (Svelte + FastAPI) matched and surfaced Go/SQLite/MCP cards. The gate
    "worked" on in-domain queries; it had near-zero precision on real traffic.
  error_signatures: []

applies_to:
  - ecosystem: "sqlite"
    package: "fts5"
  - ecosystem: "Go"
    package: "modernc.org/sqlite"

resolution:
  root_cause: >
    Document frequency conflates "rare in THIS corpus" with "specific". In a
    9-record store, "http" appears in 2 records (df=2) — indistinguishable by
    df from a genuinely rare identifier like "fts5" (df=2). The gate keyed on
    df in [1, maxDF] and so admitted common words as if they were signals. Two
    aggravators: (1) maxDF was scaled as ceil(0.25·nValidated), so promoting 66
    homogeneous OSV/GHSA advisory records grew the corpus and loosened the
    ceiling 3→19, widening the leak band; (2) the bet that "http/method/
    permission are genuine signals, keep them" was exactly inverted — they are
    the leak.
  fix: >
    Require BOTH signals, and measure precision, not just recall. (a) Fix the
    df ceiling (a small constant, not a fraction of the corpus, so growth can
    never loosen the gate). (b) Add a common-word stoplist: a token must be
    rare AND not common vocabulary; the genuine signals are the rare
    identifiers a real error query always carries (fts5, bm25, servemux,
    tmpdir, rand.Seed, setup-go), and stoplisting a common word never silences
    a genuine query because it still carries those. (c) Gate it with a
    precision eval: a NEGATIVE set of realistic off-domain prompts that MUST
    return nothing, run against the live corpus, alongside the positive recall
    set. Precision (false-injection rate) is the metric a recall-only eval
    never sees.
  dead_ends:
    - tried: "a magnitude/BM25 floor to reject off-topic queries"
      why_it_failed: >
        BM25 is corpus-relative — off-topic prose scores as high as a weak
        genuine hit, so no floor value separates them. This is why the df gate
        was introduced in the first place.
    - tried: "scaling the df ceiling with the corpus (ceil(0.25·nValidated)) to 'generalize as it grows'"
      why_it_failed: >
        It generalized the wrong way: a larger ceiling admits MORE tokens as
        discriminative, so corpus growth loosened the gate. A flood of
        homogeneous records (advisories) made it worse, not better.
    - tried: "keeping common dev words (http/method/permission) as discriminative because they look domain-relevant"
      why_it_failed: >
        They are common vocabulary, present in unrelated prompts; the precision
        eval showed they were the dominant leak source.

guard:
  repro: null
  guarding_test: "TestPushPrecisionOnLiveCorpus"

provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-21
  validated_at: 2026-06-21
  valid: { from: 2026-06-21, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

You build a recommender / push channel over a small curated corpus and need to
decide, per query token, "is this a real signal or noise?" Document frequency
looks perfect: a rare token (low df) is specific, a generic token (high df) is
filler. It tests beautifully — every in-domain query carries a low-df token and
surfaces the right record. Then it runs on real traffic and injects unrelated
cards into every vaguely-technical prompt, because **low df in a tiny corpus is
not specificity**. "http" sits in 2 of 9 records; so does "fts5". df cannot tell
them apart, but one is universal dev vocabulary and the other is a genuine
signal.

## Why it happens

Specificity is a property of the token in *general usage*; df measures it only
in *your* corpus. When the corpus is small, common words have low df by
accident. Worse, if you scale the "too generic" ceiling with corpus size to
"generalize", growth loosens the gate (more tokens qualify as rare), and a
batch of homogeneous records (here: 66 OSV/GHSA advisories promoted at once)
both inflates the ceiling and dilutes df — the gate calibrated on 9 records was
silently 6× looser at 75.

## The escape

- Combine df-rarity with a **common-word stoplist**: discriminative = rare in
  the corpus AND not common vocabulary. The genuine signals are the rare
  identifiers real error queries always carry; stoplisting a common word can't
  silence a genuine query.
- Make the corpus-generic ceiling a **fixed constant**, never a fraction of a
  growing corpus.
- **Measure precision, not just recall.** Add a negative eval set — realistic
  off-domain inputs that must return nothing — and run it against the live
  corpus. Recall-only evals are blind to the exact failure (over-injection)
  that makes a push channel worthless.

## Scope

Any df/IDF-style gate over a small or fast-growing curated corpus: push
recommenders, "related items", auto-tagging, retrieval injection. The smaller
or more homogeneous the corpus, the less df alone means. twiceshy's push
channel (ADR-0001 §4) is gated this way; the guarding test asserts zero
off-domain injection on the live corpus, and `twiceshy eval -push` reports the
precision/recall the fix is held to.

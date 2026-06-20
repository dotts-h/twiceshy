# ADR-0015: The push channel gates on a discriminative term, not a magnitude floor

- **Status:** Accepted (deciders: claude, during the twiceshy-adoption pass).
  Changes a retrieval invariant for the push channel only; operationally unlocks
  turning push on.
- **Related:** ADR-0001 §4 (embedding-free hot path), ADR-0004 (DefaultFloor is a
  pinned relevance floor), ADR-0007 (the floor is policy, not a per-caller
  accident), ADR-0006 (deferred score-banding, which subsumes this), #0002 (the
  push channel), #0005 (the eval that justifies enabling push). Supersedes the
  push channel's reliance on `DefaultFloor` for relevance; ADR-0004/0007 stand for
  the pull/Assess paths.

## Context

The push channel (`POST /push` → `UserPromptSubmit`/`PreToolUse` injection) is the
deterministic path that makes twiceshy fire without an agent remembering to ask.
It was **deferred** because it injected on near-any prompt. Measured on the live
corpus, `/push` returned 2–3 trap cards for *"write a haiku about cats"*, *"what
time is it in Tokyo"*, and *"how do I center a div in CSS"* — pure noise that, once
injected on every prompt, trains agents to ignore the channel and burns context
tokens. This is the concrete reason adoption was zero (the twiceshy-adoption memory).

Root cause: push reached the corpus through `Retrieve`, whose relevance gate is the
shared magnitude floor `DefaultFloor`. **BM25 magnitude does not separate off-topic
from on-topic**, and the separation it does have is not scale-free:

- Off-topic prose scored *above* a flat cut while a weak genuine hit scored below it
  (measured: *"buy milk"* top 9.62, *"capital of France"* 7.49, vs on-topic
  *"llm judge"* 1.57). The bands fully overlap — no single literal splits them.
- BM25's IDF and length-normalization shift with corpus size, so any tuned absolute
  floor drifts: the same query scored ~8e-6 on a 2-record test corpus and 2–21 on
  the live corpus. A magic number is obsolete one corpus generation later.

What *does* separate cleanly is **document frequency**. Every off-topic content
token is absent from the validated corpus (df=0) or generic (df≥3); every genuine
error query carries at least one *discriminative* token — present in 1–2 validated
records (`fts5`=2, `tmpdir`=1, `bm25`=1, `sse`=1, `permission`=2). This is structural
and scale-free: it asks "is this token rare enough to be a real signal?", a question
whose answer tracks the corpus instead of drifting against it.

## Options considered

1. **Raise `DefaultFloor` to a calibrated absolute value (e.g. 7.0).** One-line,
   but measured *dead* on the live corpus: off-topic and on-topic bands overlap, so
   no flat cut separates them, and the value drifts with corpus growth (TECH_DEBT
   L6/L7). Rejected.
2. **Relative top/second-hit ratio.** Self-normalizing across query length, but the
   on-topic distribution dips into the off-topic band (a rare token isolated in one
   record mimics a real match) and near-duplicate clusters suppress true positives.
   Fragile. Rejected.
3. **Discriminative-term (validated-df) gate.** Inject only when the query carries a
   token in 1..maxDF validated records (stopwords and ecosystem names excluded),
   then search that subset at a small positive `pushFloor`. Measured: off-topic 8/8
   → 0 cards, on-topic 8/8 → correct card (and it drops a weak noise card today's
   blanket push leaks). **Chosen.**

## Decision

Option 3, **push-only** via a new `Index.RetrievePush`. Push (`push.go`) calls it;
pull (`RetrieveFused`) and `Assess` keep `DefaultFloor` and their existing tests
unchanged (ADR-0004/0007 intact). A card is injected only if the query carries a
discriminative token — `df ∈ [1, maxDF]` over **validated** records, where
`maxDF = max(2, ceil(0.25·nValidated))` so the ceiling scales with the corpus —
and the resulting card clears `pushFloor` on the discriminative-token subset.
Fingerprint-exact matches bypass the gate (a deterministic stack signature is real
context by construction). Quarantined records are never surfaced. Embedding-free
throughout (ADR-0001 §4).

## Consequences

- **Push becomes safe to turn on.** Off-topic prompts inject nothing at near-zero
  token cost; only genuine error contexts inject, k≤3. This is the precondition the
  #0005 eval needs before the per-prompt push hook is enabled.
- **Known false-negative class:** a genuinely novel on-topic query whose only token
  is common injects nothing — acceptable under the "empty is an answer" invariant,
  and `search_experience` (pull) still surfaces it.
- **The df boundary is one document wide at small N.** Guarded by the live-corpus
  precision/recall test, which fails loudly if corpus growth closes the gap. Re-run
  it and re-confirm `pushFloor`/`pushMaxDF` when the validated corpus crosses ~30.
- **Stopgap, not the endgame.** ADR-0006 score-banding (normalized/RRF) eventually
  subsumes the df gate; this is the calibrated bridge that fixes the live bug now.
- **Cost per push call:** one ecosystem-set query + one count + ≤24 single-token
  df MATCHes — sub-millisecond, embedding-free. Caching the ecosystem set and
  validated count at `Rebuild` is a tracked follow-up before push goes high-traffic.

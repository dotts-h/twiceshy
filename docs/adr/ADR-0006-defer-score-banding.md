# ADR-0006: Keep three-state novelty; defer score-banding to the dense phase

- **Status:** Accepted (2026-06-17)
- **Deciders:** horia
- **Related:** [ADR-0001 §3](ADR-0001-architecture.md) (retrieval precedence,
  relevance floor — **locked**); [ADR-0004](ADR-0004-relevance-floor-is-index-policy.md)
  (the floor is index policy); [CONTEXT.md](../CONTEXT.md) (near-miss, relevance
  floor).

## Context

`index.Assess` classifies an incoming symptom against the corpus into three
states ([CONTEXT.md] novelty):

- **known** — an exact fingerprint hit (deterministic; the symptom is present);
- **similar** — a lexical match that cleared the relevance floor (a *lead* to
  verify, returned with its evidence, never an auto-merge);
- **novel** — nothing cleared the floor.

A natural next question is whether `similar` should be subdivided by score —
"strong similar" vs. "weak similar" bands, or a calibrated confidence — to give
callers (dedup-at-ingest, future doctors) a finer signal than a single
boundary.

Doing that *now*, on Phase-3's lexical-only index, would mean banding raw BM25
scores. Those scores are unbounded, corpus-dependent, and not comparable across
queries — the exact reason [ADR-0004 §4](ADR-0004-relevance-floor-is-index-policy.md)
already pins `DefaultFloor` only as a coarse demote-the-weak-token threshold
rather than a principled cutoff. The honest place for score normalization is
when dense retrieval and RRF fusion land ([ADR-0001 §3](ADR-0001-architecture.md)):
RRF produces fused ranks that *are* comparable, and an embedding distance gives a
calibratable similarity. Banding lexical BM25 first would bake a throwaway
heuristic into a contract callers come to depend on.

## Decision

1. **Novelty stays exactly three states**, keyed on the single relevance floor:
   `known` / `similar` / `novel`. No score bands, no confidence sub-levels, no
   fourth state in this phase.

2. **The evidence carries the nuance, not the verdict.** `Assess` already
   returns `Candidates []Hit` with per-hit `Score`; a caller that wants to
   reason about "how similar" reads the candidate scores directly. The
   *classification* stays coarse and stable; the *evidence* stays rich. This
   keeps the contract honest about what a lexical score can and cannot promise.

3. **Score-banding / calibrated confidence is deferred to the dense/RRF
   retrieval phase.** It is revisited there, against fused ranks and embedding
   distances that are actually comparable — and only if a consuming feature
   (e.g. a doctor's ADD/UPDATE/SUPERSEDE/NOOP arbitration, [CONTEXT.md] D1)
   demonstrates it needs more than the floor plus raw candidate scores.

## Consequences

- The `Novelty` type and `Assess`'s contract are stable for Phase 3 and stay
  registered as a seam ([ADR-0005](ADR-0005-stable-seams.md)); they will not
  churn under a banding redesign mid-phase.
- Callers that want finer granularity have a documented path today: inspect
  `Candidates[i].Score`. They are warned (here and in [ADR-0004]) that lexical
  scores are not cross-query comparable, so any thresholding they do is at their
  own risk until banding lands.
- We accept a coarser ingest signal now in exchange for not shipping a
  score-band contract we would have to break when dense retrieval changes the
  meaning of "score". This is a deliberate, reversible deferral, not a gap to
  paper over.
</content>

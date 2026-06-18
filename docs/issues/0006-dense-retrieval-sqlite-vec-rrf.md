---
id: 0006
title: Dense retrieval — sqlite-vec + reciprocal-rank fusion (pull channel only)
status: closed
severity: medium
group: 0008
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0006-defer-score-banding.md
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Add dense/vector retrieval (sqlite-vec) fused with the existing BM25/fingerprint
results via reciprocal-rank fusion (RRF), behind an embedding cache. **Pull
channel only** — the hot path (push/hook) stays embedding-free by rule
(ADR-0001 §4). Then land score-banding (ADR-0006) to replace the coarse
corpus-coupled `DefaultFloor` (TECH_DEBT L6/L7) with a normalized/RRF band.

## Scope
- [ ] Embedding seam + cache (local fastembed/ONNX or Ollama nomic-embed; ADR-0001 §9);
      hot path must not depend on it.
- [ ] sqlite-vec index over record embeddings, rebuildable like the FTS5 index.
- [ ] RRF fusion of {fingerprint, BM25, dense} on the pull `Retrieve` seam (ADR-0007),
      preserving the k≤3 cap and relevance floor.
- [ ] Score-banding (ADR-0006): normalized Similar-vs-Novel thresholds replacing
      `DefaultFloor`; update the boundary tests; pay down TECH_DEBT L6/L7.

## Notes
Independent of #0007/#0004 — buildable in parallel. Unblocks ADR-0006, which is
deferred pending this. Recommended sequence (NEXT_FEATURES.md) places it after
the corpus exists, but there is no hard dependency.

## Closeout (PR #35)
Shipped as **pure-Go cosine + RRF**, not sqlite-vec — see **ADR-0009** (sqlite-vec
is CGO, incompatible with the locked CGO-free cross-compiled build). Pull-only,
graceful fallback to fingerprint+BM25 when no/failed embedder; hot path stays
embedding-free (`Assess` never embeds — guarded by `TestAssessNeverEmbeds`).
Embeddings via local Ollama `nomic-embed-text` (`-embed-url`), stored as BLOBs.
**Score-banding stays deferred** (ADR-0006) — no consumer yet.

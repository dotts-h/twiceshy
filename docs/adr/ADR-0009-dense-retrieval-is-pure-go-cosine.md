# ADR-0009: Dense retrieval is pure-Go cosine, not sqlite-vec — preserve the CGO-free build

- **Status:** Accepted (2026-06-18)
- **Deciders:** horia
- **Related:** [ADR-0001 §3–4](ADR-0001-architecture.md) (retrieval precedence,
  hot-path embedding-free rule — **locked**); [ADR-0006](ADR-0006-defer-score-banding.md)
  (score-banding deferred to the dense/RRF phase); issue #0006.

## Context

ADR-0001 §3 plans dense retrieval as "sqlite-vec + RRF". When building #0006 the
sqlite-vec assumption collided with a locked property of the build:

- twiceshy uses **`modernc.org/sqlite`** (a pure-Go SQLite), and the release
  workflow cross-compiles the binaries with **`CGO_ENABLED=0`**
  (`release.yml`: "twiceshy is pure-Go / CGO-free, so these all cross-compile").
- **`sqlite-vec` is a CGO C extension.** Adopting it forces a CGO SQLite driver
  and `CGO_ENABLED=1`, which **breaks the pure-Go cross-compiled release** for
  linux/darwin × amd64/arm64 and complicates CI (the runner would need the C
  toolchain + the extension per platform).

The corpus is also small (tens of records, growing slowly), so an approximate
vector index buys nothing yet — a brute-force scan is sub-millisecond at this
scale. The hot path is embedding-free by rule (ADR-0001 §4) regardless.

## Decision

1. **Dense retrieval is pure-Go in-memory cosine similarity** over record
   embeddings — no sqlite-vec, no CGO. Embeddings are computed from a local
   Ollama `nomic-embed-text` endpoint (ADR-0001 §9), behind a cache, and stored
   in the derived index as a BLOB column (pure-Go `modernc` handles BLOBs);
   dense search is a brute-force cosine scan over the corpus.
2. **RRF fusion stands.** Dense hits are fused with fingerprint-exact and
   BM25/FTS5 hits via reciprocal-rank fusion on the pull `Retrieve` seam,
   preserving the hard cap k≤3 and the relevance floor.
3. **Dense is pull-only and degrades gracefully.** It runs only when an embedder
   endpoint is configured; if unset or unreachable, retrieval falls back to
   fingerprint + BM25 (the deployed server never hard-fails because Ollama is
   down). The hot/push path never calls the embedder (ADR-0001 §4 — unchanged).
4. **sqlite-vec is not adopted.** It is revisited only if the corpus outgrows an
   in-memory cosine scan **and** we are willing to take on CGO (losing the
   CGO-free cross-compiled release) — a deliberate, reversible deferral.

This **supersedes the "sqlite-vec" wording** in ADR-0001 §3, ADR-0006, and issue
#0006; the RRF and (deferred) score-banding intent is unchanged.

## Consequences

- The CGO-free, `CGO_ENABLED=0` cross-compiled release is preserved — no new CGO
  dependency, no new module (the Ollama client is stdlib `net/http`/`json`).
- Dense adds a *pull-time* runtime dependency on the Ollama endpoint, scoped and
  optional: pull-only, cached, and fallback-safe. Tests use a stub embedder, so
  CI needs no Ollama.
- Score-banding stays deferred per ADR-0006 — its condition (a consuming feature
  that needs more than the floor + raw scores, e.g. a doctor) is not yet met.
- If the corpus grows large enough that a brute-force scan hurts pull latency,
  reopen the sqlite-vec-vs-CGO trade here.

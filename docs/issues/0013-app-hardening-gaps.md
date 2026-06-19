---
id: 0013
title: App-hardening gaps — body cap, query timeouts, rate limit, path/error hygiene
status: closed
severity: medium
group: 0009
depends_on: []
forgejo: 103
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
SECURITY_ANALYSIS.md Facet 3, the P1 gaps left after what's already done
(timing-safe token, FTS5 escaping, query-size cap, no-unauth).

## Scope
- [ ] **Request body cap** on `record_experience` via `http.MaxBytesReader`.
- [ ] **Query timeouts** (`context.WithTimeout`) on index/DB ops; bounded result sizes.
- [ ] **Rate limiting** middleware (`golang.org/x/time/rate` or a small custom
      limiter) on the MCP endpoints — confirm dep budget before adding x/time.
- [ ] **Path-under-root assertion** when persisting records (defensive — paths are
      already derived via `buildPath`/`slugify`, but assert the resolved path stays
      under the corpus root before write).
- [ ] **Error hygiene:** client-facing errors carry no internal paths/stack;
      detail stays in server logs.
- [ ] **Container hardening:** the deploy image runs non-root, read-only FS where
      possible (lands with the deploy artifact).

## Acceptance
- [ ] Oversized `record_experience` body is rejected, not buffered unbounded.
- [ ] A write whose resolved path escapes the corpus root is refused; guarding test.

# ADR-0010: Doctors — build the framework + D2 staleness now; defer D1/D3/D4/D5 until their substrate exists

- **Status:** Accepted (2026-06-18)
- **Deciders:** horia
- **Related:** [ADR-0001 §7](ADR-0001-architecture.md) (doctors: background jobs,
  **delta-only, never whole-store rewrites** — locked); [ADR-0008](ADR-0008-write-path-persistence-is-a-cli-concern.md)
  (git/PR is the trust boundary); [CONTEXT.md](../CONTEXT.md) (the D1–D5 roster,
  `stale`); issue #0004.

## Context

#0004 enumerates five doctors (D1 dedup/reconcile, D2 staleness, D3 revalidation,
D4 lifecycle, D5 abstraction). Building all five now would violate the project's
own "no machinery without a consumer/substrate" discipline (the same reason the
endoflife sidecar and score-banding were deferred): **four of the five have no
input to act on yet.**

- **D3 revalidation** re-executes each record's **repro script** in a sandbox.
  **No record currently has a runnable repro** — importer records are quarantined
  *facts*, and the codemod live-execution adapter (before/after → repro) was
  deferred. A Docker sandbox runner with ~0 scripts to run is pure speculation,
  and it carries the most infra risk of anything in the project.
- **D4 lifecycle** reinforces/decays by **usage counters** (`provenance.usage`),
  but retrieval does not yet increment them — there is no signal to decay on.
- **D1 dedup/reconcile** — the write path already dedups at ingest via
  `index.Assess` (known/similar/novel). A full LLM-arbitrated ADD/UPDATE/
  SUPERSEDE/NOOP doctor is an enhancement over a working baseline, not a gap.
- **D5 abstraction** induces convention cards from **clusters** of related
  episodes; the corpus is far too small to cluster meaningfully.
- **D2 staleness** is the one with real substrate **today**: importer records
  carry `applies_to` versions, and `provenance.valid.until` + the `stale` status
  exist for exactly this. D2 also absorbs the **endoflife.date sidecar** deferred
  from #0007.

## Decision

1. **Build now:** a minimal **doctor framework** (`twiceshy doctor <name>`, a
   `doctors` package, delta-only, report/propose — never mutate `validated`
   records silently; the trust boundary is git/PR per ADR-0008) **and D2
   staleness**: cross-check each record's `applies_to` against endoflife.date
   (and explicit version facts) and flag/propose `stale` + `valid.until` for
   records whose world has moved on. endoflife access is an injectable seam
   (stub in tests — no network in CI), mirroring the embedder.
2. **Defer D1, D3, D4, D5** until their substrate exists, each with a named
   trigger:
   - **D3** → when records carry runnable repros (the codemod live-execution
     adapter, or human-authored guards). It is the keystone for the push channel
     and will be built then; the sandbox runner is designed at that point.
   - **D4** → when retrieval increments `provenance.usage`.
   - **D1 (LLM-arbitrated)** → when ingest dedup demonstrably needs more than
     `Assess`'s known/similar/novel.
   - **D5** → when the corpus is large enough to cluster.
3. **No invariant bent.** All doctors stay delta-only; D2 proposes changes for
   review rather than rewriting trusted records.

## Consequences

- The doctor framework lands once and the deferred doctors slot into it later
  without rework. #0004 is **partially closed** (framework + D2); D1/D3/D4/D5
  remain tracked as open follow-ups under it with the triggers above.
- The push channel (#0002) remains blocked on D3 (validated records) — which is
  honest: there is nothing to validate until runnable repros exist. This is
  surfaced, not hidden.
- If a consumer for a deferred doctor appears sooner than expected, reopen the
  relevant trigger here.

# ADR-0007: The relevance floor applies to the read path too, via a single injection seam

- **Status:** Accepted (2026-06-17)
- **Deciders:** horia
- **Related:** [ADR-0001 §3](ADR-0001-architecture.md) (relevance floor — a
  **locked** invariant for *injection*, push and pull alike); extends
  [ADR-0004](ADR-0004-relevance-floor-is-index-policy.md) (the floor is index
  policy on the write/`Assess` path) to the read path **without superseding it**;
  the `index.Index` seam is registered in [CONTRACTS.md](../CONTRACTS.md) per
  [ADR-0005](ADR-0005-stable-seams.md).

## Context

[ADR-0004](ADR-0004-relevance-floor-is-index-policy.md) made the relevance floor
**policy** of the index layer: `Assess` applies `DefaultFloor` unless the caller
explicitly overrides it, so the dedup-at-ingest write path can no longer lose the
floor to a zero-value `Query.Floor`. `Search` was deliberately kept a *pure
mechanism* — `Floor > 0` applies a floor, `Floor <= 0` is off — because raw,
floor-free recall is a real need (tests, future diagnostic/pull tooling).

But the floor in [ADR-0001 §3](ADR-0001-architecture.md) is an *injection*
invariant — it governs **both** channels ([CONTEXT.md](../CONTEXT.md) push/pull),
not just write-path classification. And the read path violates it: the
`search_experience` handler ([ADR-0001 §5](ADR-0001-architecture.md), the **pull
channel**) calls `index.Search` directly with `Floor` unset, i.e. floor-off. So
the single measured #1 failure mode — **near-miss** injection ([CONTEXT.md]
"near-miss") — is exactly what the pull channel does today: any weak lexical
overlap is returned to the agent instead of the honest empty answer.

The naive fix — set `Floor: DefaultFloor` in the handler — is precisely the
per-call accident [ADR-0004 §Options C](ADR-0004-relevance-floor-is-index-policy.md)
**rejected**: it reintroduces the bug the moment a second injection caller forgets
the line. The floor must stay structural.

## Decision

1. **`index.Retrieve` is the injection-path search; `Search` stays the raw
   mechanism.** A new exported method `Retrieve(ctx, Query) ([]Hit, error)`
   applies the floor policy and then calls `Search`. Every caller that retrieves
   records **for injection** — the pull channel (`search_experience`) and, when
   it lands, the push channel — goes through `Retrieve`. `Search` is unchanged
   and remains the floor-free mechanism for raw recall (`Floor: FloorOff`) and
   internal use. This keeps ADR-0004's clean split intact: `Search` does what it
   is told; the policy lives one layer up.

2. **The floor substitution lives in exactly one place.** Both `Retrieve` and
   `Assess` apply the default through a single internal helper
   (`floorPolicy`): `Floor == 0` → `DefaultFloor`; any explicit `Floor`
   (positive threshold, or `FloorOff`) is the caller's deliberate override.
   `Assess` is refactored to retrieve through `Retrieve`, so there is one
   definition of "the floor is on by default," shared by the read and write
   paths. The bug class — *an injection caller forgot the floor* — becomes
   unrepresentable, the same property ADR-0004 won for the write path.

3. **`search_experience` calls `Retrieve`.** The one-line handler change routes
   the pull channel through the floored seam. The pull channel deliberately does
   **not** expose a floor-off knob to MCP clients: an agent cannot ask the
   service to inject near-misses. (Raw recall stays an internal/diagnostic
   capability via `Search` + `FloorOff`.)

4. **`Retrieve` is registered as part of the `index.Index` seam**
   ([CONTRACTS.md](../CONTRACTS.md), [ADR-0005](ADR-0005-stable-seams.md)): it is
   the stable entry point injection callers build against.

## Options

- **A — move the default into `Search` itself** (`Search` substitutes
  `DefaultFloor` on `Floor == 0`; `FloorOff` the only off-switch). Most
  structural — the floor is unbypassable everywhere through one seam. Rejected
  for now: it **supersedes** [ADR-0004 §3]'s just-locked "`Search` is the pure
  mechanism, `0` = off" and churns every `Search`-level test that relied on
  floor-off. The named opt-out (`FloorOff`) would carry it, but the cost
  outweighs the benefit while there is a single read caller. Kept on the table as
  a future supersede if direct floor-free `Search` use proves to have no
  legitimate callers.

- **B — a floored injection seam (`Retrieve`), `Search` stays pure (chosen).**
  Additive, supersedes nothing, no churn to ADR-0004 or the `Search` test suite,
  and the floor substitution is shared (not per-caller). The residual risk — a
  future caller wiring injection through raw `Search` by mistake — is small (one
  read caller today) and caught by review against the registered seam.

- **C — set `Floor: DefaultFloor` in each injection handler.** Rejected: this is
  ADR-0004's rejected option C verbatim, reintroducing the per-call accident.

## Consequences

- The pull channel honors **"empty is an answer"** ([CONTRACTS.md]): a query
  whose only matches fall below `DefaultFloor` returns no hits instead of a
  near-miss. The [ADR-0001 §3](ADR-0001-architecture.md) injection invariant now
  holds on the read path as well as the write path.
- `index` gains one public method (`Retrieve`); `Assess` is reimplemented in
  terms of it with no behavior change. `Search`'s contract is untouched, so
  existing direct callers and tests are unaffected.
- Push-channel work (issue #2) inherits the floor for free by retrieving through
  `Retrieve`, closing the same gap before it can open.
- `DefaultFloor` remains the single coarse, corpus-pinned threshold owned by its
  guarding tests, revisited at the dense/RRF phase ([ADR-0006](ADR-0006-defer-score-banding.md)).

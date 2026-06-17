# ADR-0005: Register the stable seams in CONTRACTS.md

- **Status:** Accepted (2026-06-17)
- **Deciders:** horia
- **Related:** [ADR-0001](ADR-0001-architecture.md) (the locked architecture
  these seams realize); [CONTRACTS.md](../CONTRACTS.md) (the registry this ADR
  populates); [ARCHITECTURE.md](../ARCHITECTURE.md) (where the seams sit in the
  module map); [CONVENTIONS.md](../CONVENTIONS.md) "Docs — one fact, one home".

## Context

The Phase-1 read path and the Phase-3 in-memory write path are merged: the
`index` package, the three MCP tools, and the record schema now have a settled
shape that other code — and, soon, other repos (push channel #2, importer #7,
doctors) and external MCP clients — build against. Nothing yet declares which of
those shapes are **promises** versus incidental internals.

That gap is a drift risk. `docs/CONTRACTS.md` exists for exactly this (the
registry of stable seams, [CONVENTIONS.md] "one fact, one home"), but it is an
empty scaffold. Without it, a "drive-by" rename of a tool argument or a `Query`
field reads as an ordinary refactor instead of a contract break, and the
boundary nobody is allowed to cross silently (the quarantined-only write
invariant, [ADR-0001 §6](ADR-0001-architecture.md)) lives only in code comments.

## Decision

Register the following as stable seams in [CONTRACTS.md](../CONTRACTS.md), the
single source of truth for the *shape* (this ADR records *that* they are
contracts and *why*; it does not duplicate the tables). Changing any of them is
deliberate — it takes an ADR and a coordinated rollout, never a drive-by edit.

1. **The three MCP tools (pull channel + write path, [ADR-0001 §5]).** Tool
   names, argument and result schemas, and behavioral promises:
   - `search_experience` — `SearchArgs` → `SearchResult`; at most `MaxK` (=3)
     hits; **"empty is an answer"** (no near-miss padding); quarantined records
     hidden unless `include_quarantined`.
   - `get_experience` — `GetArgs{id}` → `GetResult` (full record markdown), any
     status (an explicit pull).
   - `record_experience` — `RecordArgs` → `RecordResult`; returns a
     **quarantined** draft to PR or a `known`-duplicate verdict; **never writes
     and never returns a validated record** ([ADR-0001 §6]).
   - Transport/auth invariants for all three: streamable HTTP (never the
     deprecated HTTP+SSE), **bearer required — no unauthenticated mode**,
     constant-time token compare, token never logged ([CONVENTIONS.md] Security).

2. **The `index.Index` seam.** Its method set (`Open`, `Close`, `Rebuild`,
   `Search`, `Assess`, `Get`, `NextID`) and the value types that cross it
   (`Query`, `Hit`, `Stored`, `Novelty`, `Assessment`), with the retrieval
   invariants: precedence **fingerprint-exact → lexical**, the **`MaxK` hard
   cap**, the **relevance floor** as index policy
   ([ADR-0004](ADR-0004-relevance-floor-is-index-policy.md)), exported scores
   positive/higher-is-better, and the embedding-free hot path
   ([ADR-0001 §3–4]). Three-state `Novelty` is fixed for this phase
   ([ADR-0006](ADR-0006-defer-score-banding.md)).

3. **The record schema + identity.** The on-disk record format
   ([SCHEMA.md](../SCHEMA.md), `schema_version: 1`, the versioned JSON Schema in
   `schema/`) and the `exp-NNNN` id format that `NextID` allocates and the file
   path embeds. The schema is the contract between writer (`ingest`/`record`)
   and reader (`index`), and the durable format of the corpus itself.

4. **The quarantined-only write invariant (the trust boundary, [ADR-0001 §6]).**
   Agent-proposed records land `quarantined`; promotion to `validated` requires
   the guard's sandbox fail-to-pass **plus** human PR review. **The PR is the
   trust boundary** — no code path writes a `validated` record, and quarantined
   records never reach the push channel. This is a cross-cutting invariant, not
   a single method, so it is registered under CONTRACTS.md "Invariants" citing
   this ADR.

The "Provides / Consumes" fleet contracts stay empty: twiceshy exposes its
surface over MCP/HTTP at runtime, not as a fleet `constellation.yaml` build-time
contract, until a sibling repo consumes it.

## Consequences

- `CONTRACTS.md` becomes the checklist a reviewer consults before approving a
  change to a tool signature, a `Query`/`Hit` field, the schema, or the write
  invariant; such changes now visibly require an ADR.
- These seams are stable, not frozen. Additive evolution (a new optional tool
  arg, a new `applies_to` field per [ADR-0003]) stays within the contract;
  renames, removals, and semantic changes are breaks that supersede via ADR.
- The registry points at code and SCHEMA/ADRs rather than copying them
  ([CONVENTIONS.md] "one fact, one home"); when a seam moves, its one home
  updates and the registry re-points — it is an index, not a second copy.
- No code changes land with this ADR; it is documentation of existing promises.
</content>

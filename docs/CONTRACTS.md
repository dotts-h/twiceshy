# CONTRACTS.md — twiceshy stable promises

> The registry of seams: interfaces, routes, event vocabularies, schemas, and
> invariants other code (or other repos) relies on. Changing anything listed
> here is **deliberate** — it gets a decision record and a coordinated rollout,
> never a drive-by edit. This is an *index*, not a prose copy: each entry states
> the shape tersely and points at the source of truth.

## Internal seams

> Promises between modules inside this repo. Changing any of these is deliberate
> ([ADR-0005](adr/ADR-0005-stable-seams.md)) — an ADR and a coordinated rollout,
> never a drive-by edit.

| seam | shape (one line) | stability | owner / source |
|------|------------------|-----------|----------------|
| `search_experience` | `SearchArgs` → `SearchResult`; ≤ `MaxK` (=3) hits; **empty is an answer** (no near-miss padding); quarantined hidden unless `include_quarantined` | stable | `internal/server` ([ADR-0001 §5](adr/ADR-0001-architecture.md)) |
| `get_experience` | `GetArgs{id}` → `GetResult` (full record markdown), any status — an explicit pull | stable | `internal/server` ([ADR-0001 §5](adr/ADR-0001-architecture.md)) |
| `record_experience` | `RecordArgs` → `RecordResult`; returns a **quarantined** draft to PR or a `known`-duplicate verdict; never writes, never returns a validated record | stable | `internal/server` ([ADR-0001 §6](adr/ADR-0001-architecture.md)) |
| MCP transport/auth (all three) | streamable HTTP (never HTTP+SSE); **bearer required — no unauthenticated mode**; constant-time token compare; token never logged | stable | `internal/server` ([CONVENTIONS.md](CONVENTIONS.md) Security) |
| `index.Index` | methods `Open`/`Close`/`Rebuild`/`Search`/`Assess`/`Get`/`NextID`; values `Query`/`Hit`/`Stored`/`Novelty`/`Assessment`; precedence **fingerprint-exact → lexical**, **`MaxK` hard cap**, relevance floor as index policy ([ADR-0004](adr/ADR-0004-relevance-floor-is-index-policy.md)), scores positive/higher-is-better, embedding-free hot path | stable | `internal/index` ([ADR-0001 §3–4](adr/ADR-0001-architecture.md)) |
| `index.Novelty` | exactly three states `known`/`similar`/`novel`, keyed on the single relevance floor; no score bands this phase | stable | `internal/index` ([ADR-0006](adr/ADR-0006-defer-score-banding.md)) |
| record schema + identity | on-disk record format (`schema_version: 1`, versioned JSON Schema in `schema/`) and the `exp-NNNN` id `NextID` allocates and the path embeds — the contract between writer (`ingest`/`record`) and reader (`index`) | stable | [SCHEMA.md](SCHEMA.md), `schema/` |

## Provides (consumed by other repos)

> Machine-checked by `fleet-doctor.sh`. Format — one bullet per contract:
> ``- `contract-id` — description · shape/schema pointer``
> The id is fleet-unique, kebab/dot style (e.g. `acme.users.api-v1`).

- *(none yet)*

## Consumes (provided by other repos)

> Same format. Every id listed here must be **provided** by exactly one sibling
> repo in the fleet's `constellation.yaml` — the fleet doctor fails otherwise.

- *(none yet)*

## Invariants

> Cross-cutting promises that aren't a single seam (determinism, ordering,
> escaping, atomicity). Each cites its decision record.

- **Quarantined-only write (the trust boundary).** Agent-proposed records land
  `quarantined`; promotion to `validated` requires the guard's sandbox
  fail-to-pass **plus** human PR review — **the PR is the trust boundary**. No
  code path writes a `validated` record, and quarantined records never reach the
  push channel. ([ADR-0001 §6](adr/ADR-0001-architecture.md),
  [ADR-0005](adr/ADR-0005-stable-seams.md))
- **Relevance floor is index policy, not a per-call argument.** Every novelty
  classification applies `index.DefaultFloor` by default; floor-off is the
  explicit, greppable `index.FloorOff` sentinel, never an accidental zero.
  ([ADR-0004](adr/ADR-0004-relevance-floor-is-index-policy.md))

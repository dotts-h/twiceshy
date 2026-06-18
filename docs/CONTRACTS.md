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
| `search_experience` | `SearchArgs` → `SearchResult` + enveloped text `Content`; ≤ `MaxK` (=3) hits; hit `title`/`summary` transport-sanitized + capped; multi-card body injection-framed; **empty is an answer** (no near-miss padding); quarantined hidden unless `include_quarantined` | stable | `internal/server` ([ADR-0001 §5](adr/ADR-0001-architecture.md)) |
| `get_experience` | `GetArgs{id}` → `GetResult` + same enveloped text `Content`; `GetResult.markdown` is the injection-framed rendering (enveloped + transport-sanitized + capped), not raw store bytes — any status, explicit pull | stable | `internal/server` ([ADR-0001 §5](adr/ADR-0001-architecture.md)) |
| `record_experience` | `RecordArgs` → `RecordResult`; returns a **quarantined** draft to PR or a `known`-duplicate verdict; never writes, never returns a validated record | stable | `internal/server` ([ADR-0001 §6](adr/ADR-0001-architecture.md)) |
| MCP transport/auth (all three) | streamable HTTP (never HTTP+SSE); **bearer required — no unauthenticated mode**; constant-time token compare; token never logged | stable | `internal/server` ([CONVENTIONS.md](CONVENTIONS.md) Security) |
| `index.Index` | methods `Open`/`Close`/`Rebuild`/`Search`/`Retrieve`/`RetrieveFused`/`Assess`/`Get`/`NextID`/`EmbedCorpus`; values `Query`/`Hit`/`Stored`/`Novelty`/`Assessment`; precedence **fingerprint-exact → lexical → dense**, **`MaxK` hard cap**, relevance floor as index policy — `Retrieve`/`Assess` apply it, `Search` is the floor-free mechanism, `RetrieveFused` is the **pull entry point** (fingerprint precedence + RRF over lexical+dense; falls back to `Search` with no/erroring embedder) ([ADR-0004](adr/ADR-0004-relevance-floor-is-index-policy.md), [ADR-0007](adr/ADR-0007-floor-on-the-read-path.md), [ADR-0009](adr/ADR-0009-dense-retrieval-is-pure-go-cosine.md)); scores positive/higher-is-better, **embedding-free hot path — `Assess`/`Search`/`Retrieve` never embed; only `RetrieveFused` does** | stable | `internal/index` ([ADR-0001 §3–4](adr/ADR-0001-architecture.md)) |
| `index.Embedder` | `Embed(ctx, text) ([]float32, error)`; pull-only, optional, fallback-safe; `OllamaEmbedder` edge is the only impl, a stub drives tests | stable | `internal/index` ([ADR-0009](adr/ADR-0009-dense-retrieval-is-pure-go-cosine.md)) |
| `index.Novelty` | exactly three states `known`/`similar`/`novel`, keyed on the single relevance floor; no score bands this phase | stable | `internal/index` ([ADR-0006](adr/ADR-0006-defer-score-banding.md)) |
| `ingest.Source` | `Name() string` + `Drafts(ctx) ([]Draft, error)`; a license-clean importer adapter emitting quarantined-record drafts; impls `deprecationSource` (go/py), `osvSource` (embedded snapshot) + `OSVLiveSource` (live osv.dev fetch) | stable | `internal/ingest` ([ADR-0003](adr/ADR-0003-corpus-bootstrap-source-scope.md)) |
| `doctor.Doctor` / `EOLSource` | `Doctor.Run(ctx, recs) (Report, error)` — **report-only/delta, never mutates the corpus**; `EOLSource.Cycles(ctx, product)` feeds D2 staleness | stable | `internal/doctor` ([ADR-0010](adr/ADR-0010-doctors-build-d2-defer-the-rest.md)) |
| ingestion safety gate | `screen.Scan` over every record text field before write; a hit sets `provenance.security_flags` (a flagged record can never become `validated`); default quarantine-with-flag | stable | `internal/screen`, `internal/ingest` ([SECURITY_ANALYSIS.md](research/SECURITY_ANALYSIS.md) §2) |
| record schema + identity | on-disk record format (`schema_version: 1`, versioned JSON Schema in `schema/`; additive `provenance.source_license`/`source_url`/`security_flags` and the `guard.repros` test-set) and the `exp-NNNN` id `NextID` allocates and the path embeds — the contract between writer (`ingest`/`record`) and reader (`index`) | stable | [SCHEMA.md](SCHEMA.md), `schema/` |

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

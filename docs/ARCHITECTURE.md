# twiceshy — Architecture

> The engineering deep dive: how a Git-backed experience service that feeds
> engineering lessons to LLM agents via MCP is put together and *why*.
> Keep it honest — when the code and this doc disagree, the code wins, so update this
> when a boundary moves. Decisions that shaped it link to ADRs (docs/adr/).

## Design goals

The load-bearing principles every module choice is justified against. They come
from [ADR-0001](adr/ADR-0001-architecture.md) and the research behind it
([EXPERIENCE_SERVICE_RESEARCH.md](research/EXPERIENCE_SERVICE_RESEARCH.md)).

- **Pure core / thin edges.** Domain logic (parsing, fingerprinting, novelty
  classification, dedup-at-ingest, marshalling) is pure and synchronous; it
  takes values and returns values. I/O and transport (SQLite, HTTP/MCP, the
  filesystem corpus) live at the edges and are kept deliberately thin. The core
  never reaches out to the network or the clock.
- **Testable without a network.** Every behavior worth a test is reachable with
  in-process values and a temp-dir SQLite file — no live MCP client, no remote
  model, no fixtures that need credentials. The cheap-executor and embedding
  work that *does* touch the network stays out of the hot path by construction.
- **Rebuildable, derived index.** The `experience/` markdown corpus is the
  single source of truth; the SQLite/FTS5 index is a *cache* that
  `index.Rebuild` reconstructs from the corpus at will. Losing or deleting the
  index is never data loss — it is a rebuild. Nothing authoritative lives only
  in SQLite.
- **Embedding-free hot path.** Retrieval at decision time is fingerprint-exact
  then lexical (BM25), capped and floored — no embedding call on the path an
  agent waits on ([ADR-0001 §3–4](adr/ADR-0001-architecture.md)). Dense/RRF
  retrieval ([ADR-0009](adr/ADR-0009-dense-retrieval-is-pure-go-cosine.md),
  pure-Go cosine) runs only on the pull path behind an optional embedder and
  stays off the hot path; `Assess` (the hot/push classifier) never embeds.

## Module map

Single Go module `github.com/dotts-h/twiceshy`, one deployable binary. Entry
point first, pure domain core in the middle, IO/transport edges last.

- **`cmd/twiceshy/`** *(edge — entry)* — the binary. Thin `main` → `run(ctx,
  args, out, getenv)`; subcommands `index` (rebuild) and `serve` (MCP server).
  All logic delegates to `internal/`.
- **`internal/record/`** *(pure core)* — the experience-record domain: `Parse`,
  `ParseFile`, `LoadCorpus`, frontmatter validation, and `Marshal` (the inverse
  — a `Record` back to on-disk markdown). Owns the [SCHEMA.md](SCHEMA.md) format.
- **`internal/fingerprint/`** *(pure core)* — deterministic signature hashing
  (`Generic`/`App`/`Normalize`) and `Dedup`, which splits a record's
  fingerprints into new vs. already-present against a known-set.
- **`internal/index/`** *(edge — SQLite, derived)* — the only stateful package.
  `Open`/`Close`/`Rebuild` manage the derived FTS5 index; `Search` is the pure
  retrieval mechanism (fingerprint-exact → lexical, `MaxK` cap, relevance
  floor); `Assess` classifies an incoming symptom as `known`/`similar`/`novel`;
  `RetrieveFused` is the pull entry point that fuses fingerprint + lexical +
  optional dense (cosine) via RRF behind an `Embedder` seam (ADR-0009);
  `Get` and `NextID` round it out.
- **`internal/ingest/`** *(pure core over the index seam)* — `Prepare`, the
  dedup-at-ingest write-path core: takes a `Draft` + `Meta`, probes the corpus
  through `index.Assess`, screens it through `internal/screen`, and returns an
  `Outcome` (a quarantined draft or a duplicate verdict). The `Source` adapter
  seam (`deprecationSource` for go/py, `osvSource`) feeds the importer.
- **`internal/screen/`** *(pure core)* — the ingestion safety gate (#0011):
  `Scan` over record text for secrets / executable-harmful-code / PII; masked
  findings, never echoes a raw secret.
- **`internal/pack/`** *(pure core)* — the experience-pack builder: `Classify`
  (fail-closed commercial-license eligibility) + `BuildManifest` (ADR-0002 §4).
- **`internal/doctor/`** *(pure core + endoflife edge)* — store-hygiene jobs,
  report-only/delta (ADR-0010): the `Doctor` seam + D2 staleness over an
  `EOLSource` (endoflife.date).
- **`internal/server/`** *(edge — MCP/HTTP)* — the three MCP tools
  (`search_experience`, `get_experience`, `record_experience`) over streamable
  HTTP, bearer-gated, behind a middleware chain (auth → rate-limit → timeout →
  max-bytes). Translates tool args to core calls and back; holds no domain logic.

Dependency direction is acyclic and points inward: `cmd` →
`server`/`index`/`ingest`/`pack`/`doctor`; `server` → `index`/`record`/`ingest`;
`ingest` → `index`/`record`/`screen`; `pack`/`doctor` → `record`; `index` →
`record`/`fingerprint`; `record`, `fingerprint`, `screen` depend on nothing
internal.

## Seams / contracts

The stable seams — the boundaries other code (and, soon, sibling repos) build
against — are registered once in [CONTRACTS.md](CONTRACTS.md) and not duplicated
here ([CONVENTIONS.md](CONVENTIONS.md) "one fact, one home"). In short:

- **The three MCP tools** are the external surface: `SearchArgs`→`SearchResult`
  (≤`MaxK`=3 hits, *"empty is an answer"* — no near-miss padding),
  `GetArgs`→`GetResult`, `RecordArgs`→`RecordResult`; bearer required, no
  unauthenticated mode ([ADR-0001 §5–6](adr/ADR-0001-architecture.md)).
- **`index.Index`** is the seam between the pure core and SQLite — the method
  set and the `Query`/`Hit`/`Stored`/`Novelty`/`Assessment` value types are the
  contract `ingest` and `server` mock in tests. Retrieval invariants live here:
  precedence, the `MaxK` hard cap, and the relevance floor as *index policy*
  ([ADR-0004](adr/ADR-0004-relevance-floor-is-index-policy.md)), with three-state
  novelty fixed for this phase
  ([ADR-0006](adr/ADR-0006-defer-score-banding.md)).
- **The record schema + `exp-NNNN` identity** is the contract between writer
  (`ingest`/`record`) and reader (`index`), and the durable format of the corpus.
- **The quarantined-only write invariant** is the trust boundary: agent-proposed
  records land `quarantined`; the PR is where a human promotes them. No code path
  writes `validated`; quarantined records never reach the push channel
  ([ADR-0001 §6](adr/ADR-0001-architecture.md)).

## Failure & offline behavior

- **Missing index = rebuild, not error.** The index is derived; a first run with
  no SQLite file rebuilds from the `experience/` corpus rather than failing.
- **Empty / no-hit retrieval is a valid answer.** Below the relevance floor,
  `search_experience` returns nothing — it never pads with near-misses, which is
  the measured #1 failure mode ([CONTEXT.md](CONTEXT.md) "near-miss").
- **No network on the hot path.** Retrieval and ingest classification are
  embedding-free and run fully in-process; a down model endpoint cannot stall a
  decision-time query.
- **Writes are safe by construction.** The write path produces *quarantined*
  drafts for PR review; it never mutates the validated corpus in place, so a
  faulted ingest cannot corrupt authoritative state.
- **Bearer-gated edge.** The server refuses unauthenticated requests; tokens come
  from the environment, are compared in constant time, and are never logged
  ([CONVENTIONS.md](CONVENTIONS.md) Security).

## CI/CD

- **One gate per push:** `make ci` = `golangci-lint` + `go test -race` + the
  coverage floor (`make cover-check`, floor in the Makefile `COVER_FLOOR`).
  Lowering the floor needs an ADR-grade reason in the commit message.
- **Process conformance:** `make doctor` runs the cookbook recipe-doctor
  aggregate (`.recipes/lock.json`); process drift is caught by machinery.
- CI must be green before merge; no `//nolint` without a trailing reason.

## Testing philosophy

- **Test-first.** New behavior starts with a failing test; every regression
  ships with a guarding test *and* an experience record under `experience/` —
  twiceshy is its own first corpus ([CONVENTIONS.md](CONVENTIONS.md) TDD).
- **Test the core as values.** The pure packages (`record`, `fingerprint`,
  `ingest`) are tested in-process with literal inputs; `index` is tested against
  a temp-dir SQLite file; `server` is tested through the MCP tool handlers. No
  test needs a network or live model.
- **Invariants get explicit tests.** The `MaxK` cap, the relevance floor as
  policy, fingerprint-exact precedence, and "empty is an answer" are asserted
  directly so a refactor that erodes one fails loudly.
- Run race-enabled tests locally before pushing (`make test`).

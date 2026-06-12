# twiceshy — agent briefing

"Once bitten, twice shy": a self-hosted service that feeds hard-won
engineering experience (traps, dead-ends, root causes, validated fixes) to
LLM coding agents at decision time.

## Read these before changing anything

1. [docs/CONTEXT.md](docs/CONTEXT.md) — the ubiquitous language. Use these
   terms, exactly, everywhere.
2. [docs/CONVENTIONS.md](docs/CONVENTIONS.md) — how we build: TDD,
   dependency budget, commit style, security rules.
3. [docs/adr/ADR-0001-architecture.md](docs/adr/ADR-0001-architecture.md) —
   the ten **locked** architecture decisions. Do not relitigate them;
   they are grounded in
   [docs/research/EXPERIENCE_SERVICE_RESEARCH.md](docs/research/EXPERIENCE_SERVICE_RESEARCH.md).
4. [docs/SCHEMA.md](docs/SCHEMA.md) — the experience-record format
   (normative), with JSON Schema in `schema/` and worked examples in
   `experience/`.

## Hard rules

- **TDD.** Failing test first. Every regression fix ships with a guarding
  test *and* an experience record under `experience/` (we dogfood).
- **Dependency budget** (CONVENTIONS.md): SQLite/FTS5 driver, MCP/HTTP
  library, YAML parser. Anything else: ask the owner first.
- **Phase discipline.** Phase 1 = read path only (parser/validator, FTS5
  index, fingerprint + lexical search, MCP `search_experience` /
  `get_experience`). Dense search, hooks, write path, doctors are tracked
  issues — do not implement them early.
- **Retrieval invariants** (ADR-0001 §3–4): hot path embedding-free; hard
  cap k≤3; relevance floor below which nothing is injected; quarantined
  records never reach the push channel.
- Supersede, never delete — records and ADRs alike.

## Commands

- `make ci` — what CI runs: lint + race tests + coverage floor.
- `make test` / `make lint` / `make cover-check` — the pieces.

## License

AGPL-3.0-only; external contributions need a CLA before merge
([ADR-0002](docs/adr/ADR-0002-licensing-strategy.md)).

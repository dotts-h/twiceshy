# twiceshy — agent briefing

"Once bitten, twice shy": a self-hosted service that feeds hard-won
engineering experience (traps, dead-ends, root causes, validated fixes) to
LLM coding agents at decision time.

See AGENTS.md for onboarding (context, conventions, ADRs, schema).

## Hard rules

- **TDD.** Failing test first. Every regression fix ships with a guarding
  test *and* an experience record under `experience/` (we dogfood).
- **Dependency budget** (CONVENTIONS.md): SQLite/FTS5 driver, MCP/HTTP
  library, YAML parser. Anything else: ask the owner first.
- **Phase discipline.** Do not build ahead of an open tracked issue / ADR. The
  once-deferred surfaces now ship: dense pull retrieval (ADR-0009), push +
  discriminative-term gate (ADR-0015), telemetry (#0067), doctors/repro,
  autonomous promote/demote, and spools. Use `docs/CODEBASE_MAP.md` as the
  navigational source of truth for what exists before opening new phase work.
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

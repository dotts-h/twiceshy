# twiceshy

> Once bitten, twice shy.

A self-hosted service that feeds hard-won engineering experience — issues,
dead-ends, root causes, validated fixes — to LLM coding agents at decision
time, so they stop repeating known mistakes on autopilot. A private,
curated, validated StackOverflow that injects itself at the right moment.

## How it works (the locked architecture)

- **Source of truth:** git-backed markdown experience records (YAML frontmatter
  + narrative) — format in [docs/SCHEMA.md](docs/SCHEMA.md). The corpus is a
  separate versioned data product (`twiceshy-corpus`,
  [ADR-0021](docs/adr/ADR-0021-decouple-corpus-as-a-data-product.md)); the engine
  ships a small frozen fixture (`internal/testcorpus/`) for tests.
- **Index:** one derived, always-rebuildable SQLite file (FTS5; sqlite-vec
  later).
- **Retrieval:** fingerprint-exact → BM25 → dense (RRF), stack-fingerprint
  filtered, hard cap k≤3 with a relevance floor — below it, *nothing* is
  injected.
- **Channels:** push (Claude Code hooks → trap cards), pull (MCP tools over
  streamable HTTP), index (generated one-liners).
- **Trust:** agent-proposed records are quarantined; promotion = sandbox
  fail-to-pass validation + human PR review. A new record IS a pull request.
- **Doctors:** background jobs that dedup, staleness-check, *re-execute
  repros* (CI for memories), decay, and abstract.

Full rationale: [docs/research/EXPERIENCE_SERVICE_RESEARCH.md](docs/research/EXPERIENCE_SERVICE_RESEARCH.md)
and [docs/adr/ADR-0001-architecture.md](docs/adr/ADR-0001-architecture.md).

## Status

Bootstrapping. Phase 1 (read path: parser/validator, FTS5 index,
fingerprint + lexical search, MCP `search_experience`/`get_experience`) is
done, and the Phase 3 write path (`record_experience` — propose-only,
returns a quarantined draft to open as a PR) has landed. Remaining phases
(hooks push channel, dense retrieval, doctors) are tracked as issues.

## Development

```sh
make ci    # lint + race tests + coverage floor — what CI runs
```

See [docs/CONVENTIONS.md](docs/CONVENTIONS.md) and
[docs/CONTEXT.md](docs/CONTEXT.md) first.

## License

[AGPL-3.0-only](LICENSE). Contribution and corpus licensing:
[docs/adr/ADR-0002-licensing-strategy.md](docs/adr/ADR-0002-licensing-strategy.md).

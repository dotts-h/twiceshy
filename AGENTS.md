# AGENTS.md — twiceshy

Git-backed experience service that feeds engineering lessons to LLM agents via MCP

This file is a **thin pointer**; the canonical rules live in the docs below — read
them before acting. One fact, one home: never copy a rule here, link it.

- **Conventions (the constitution):** [docs/CONVENTIONS.md](docs/CONVENTIONS.md) —
  workflow, doctrine, quality gates, environment facts.
- **Context (the glossary):** [docs/CONTEXT.md](docs/CONTEXT.md) — the ubiquitous
  language, defined once. Read it before naming a new type; add a term there before
  writing its code.
- **Decisions (why):** [docs/adr/](docs/adr/README.md) — one record per decision.

Quick gates (exact commands in CONVENTIONS): `make lint` and `make test`
before push; CI must be green before merge. Branch from `main`; never
commit to it directly.

## Experience store (index channel)

This repo hosts a **twiceshy** experience store — validated engineering traps,
fixes, and dead-ends from past sessions. **Check known traps before debugging**
an unfamiliar error or retrying a failed approach; do not wait for the problem to
surface on its own.

- **Pull (on demand):** call the `search_experience` MCP tool with verbatim error
  text or a short symptom; follow up with `get_experience` on a hit id. Empty
  results are valid — do not force a near-miss.
- **Push (deferred):** a `UserPromptSubmit` hook ([docs/PUSH_HOOK.md](docs/PUSH_HOOK.md))
  can inject cards at prompt time, but it is off by design until its precision is
  tuned (#0005) — prefer pull.

Wiring twiceshy into a consuming agent (MCP registration + the affordance pointer):
[docs/CONSUMING.md](docs/CONSUMING.md).

## Off-pool offload (brain VM only)
When on the `claude-brain` VM, the off-pool routing table lives in `~/.claude/CLAUDE.md`.
**DATA RULE:** never send secrets or sensitive code off-LAN.

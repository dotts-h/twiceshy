# CONTEXT — the ubiquitous language of twiceshy

> "Once bitten, twice shy." twiceshy feeds hard-won engineering experience —
> traps, dead-ends, root causes, validated fixes — to LLM coding agents at
> decision time, so they stop repeating known mistakes on autopilot.

This file defines the project vocabulary. Use these words, with these meanings,
in code identifiers, docs, ADRs, commit messages, and issues. When a concept is
missing, add it here first.

The architecture behind these terms is locked in
[ADR-0001](adr/ADR-0001-architecture.md), grounded in
[the research deep-dive](research/EXPERIENCE_SERVICE_RESEARCH.md).

## The store

- **Experience record** (or just **record**) — the atomic unit of the system:
  one markdown file (YAML frontmatter + narrative body) capturing one unit of
  engineering experience: symptom → root cause → fix → dead-ends → guard.
  Records live in git under `experience/` and are the **source of truth**.
  Format: [SCHEMA.md](SCHEMA.md).
- **Kind** — what a record is: `trap` (a plausible-but-wrong move and its
  escape), `fix` (a validated resolution to a concrete failure), `dead-end`
  (an approach that was tried and must not be retried), `convention` (a
  distilled rule induced from episodes), `workflow` (a proven multi-step
  procedure).
- **Narrative body** — the markdown prose below the frontmatter. Learning
  lives in narrative; the frontmatter is the machine-readable surface.
- **Index** — the single SQLite file (FTS5 now, sqlite-vec later) derived
  from the records. Always rebuildable, never authoritative, never backed up
  as a requirement. If the index and the records disagree, the records win.
- **Experience pack** — a distributable corpus of validated records, licensed
  separately from the AGPL code (see
  [ADR-0002](adr/ADR-0002-licensing-strategy.md)). The corpus is the moat,
  not the server.

## Identity & matching

- **Organization** — an administrative identity that owns one or more private
  workspaces. It is metadata, not an authorization boundary by itself.
- **Workspace** — an organization-scoped identity associated with tenant
  tokens. Private corpus isolation is deliberately downstream; this foundation
  records identity without claiming isolation that does not yet exist.
- **Plan** — one of `community`, `pro`, `team`, or `enterprise`; a named bundle
  of entitlements, never payment state.
- **Entitlements** — the capabilities and quota policy attached to a plan.
  Entitlements are internal authorization inputs; billing providers and prices
  are outside this vocabulary.

- **Fingerprint** — a deterministic `sha256:` hash over a *normalized* error
  signature (Sentry-style). Every record carries up to two:
  - **app fingerprint** — repo-specific (in-app frames / project identifiers
    included). Highest-precision match.
  - **generic fingerprint** — stack-generic (project identifiers stripped),
    so a lesson recorded in repo A can fire in repo B.
- **Error signature** — a normalized error message string on a record; the
  exact-match retrieval surface that feeds fingerprinting.
- **Stack fingerprint** — the OSV-style `applies_to` set
  (`{ecosystem, package, version-range}` + runtime constraints) describing
  *where* a record applies. Used to filter and boost retrieval, never as the
  sole match signal.
- **Bi-temporal validity** — a record is true for a **version range**
  (`applies_to`) *and* a **time range** (`provenance.valid.from/until`),
  Graphiti-style. Both axes are first-class.

## Lifecycle & trust

- **Quarantine / quarantined** — the landing status for every agent-proposed
  record. Quarantined records **never** enter the push channel; at most they
  surface on pull queries, labeled as quarantined. Git is the trust boundary.
- **Validation / validated** — the trusted status. Promotion from quarantine
  requires the guard executing fail-to-pass in a sandbox **plus** human PR
  review. A new experience record IS a pull request.
- **Guard** — the executable proof attached to a record: a **repro script**
  that fails before the fix and passes after (**fail-to-pass / F2P
  discipline**, SWE-bench-style) plus a named **guarding test** that keeps
  the fix fixed.
- **Stale** — a record whose `applies_to` no longer matches the live world
  (version bumped past the fix, docs moved on) or whose repro stopped
  reproducing. A stale store is worse than none — it becomes the
  outdated-context prompt.
- **Supersede / superseded** — the only way a record "goes away": close its
  validity interval (`valid.until`) and link `superseded_by` to its
  replacement. **Never delete.** Archive, don't evict.
- **Provenance** — who/what/when on every record: source (session, PR,
  author), recorded/validated timestamps, validity interval, supersession
  link, usage counters. Retrieval may weight by trust tier.

## Retrieval & injection

- **Push channel** — deterministic injection: a Claude Code
  `UserPromptSubmit`/`PreToolUse` hook calls a plain HTTP endpoint and injects
  trap cards via `additionalContext` — only on high-confidence matches.
- **Pull channel** — on-demand: MCP tools (`search_experience`,
  `get_experience`, and `record_experience`) over **streamable HTTP**
  (never the deprecated HTTP+SSE transport).
- **Index channel** — cheap ambient awareness: generated skill /
  `AGENTS.md`-style one-liners so the model knows the store exists. Never
  rely on the model spontaneously calling a memory tool — it won't.
- **Trap card** — the short rendering of a record that gets injected: title,
  applicability conditions, the trap, the escape. 1–3 per injection, never
  more.
- **Hot path** — the hook-lookup path. It is **embedding-free** by rule:
  fingerprint + lexical (FTS5/BM25) only. Dense/vector search runs only on
  explicit pull-channel queries, behind an embedding cache.
- **Relevance floor** — the score threshold below which **nothing** is
  injected. Injecting nothing is a feature.
- **Near-miss** — a related-but-wrong record; the system's **#1 failure
  mode**. A single near-miss in context measurably hurts agents more than
  random noise. Defenses: relevance floor, hard cap k≤3, stack-fingerprint
  filters, applicability conditions printed in the trap card itself.

## Maintenance

- **Doctor** — a background job that keeps the store honest. Doctors operate
  by **incremental deltas only — never whole-store rewrites** (context
  collapse). The roster:
  - **D1 dedup/reconcile** — LLM arbitrates ADD / UPDATE / SUPERSEDE / NOOP
    for a candidate against its top-k similar records.
  - **D2 staleness** — cross-checks `applies_to` against live versions/docs
    (Context7-style).
  - **D3 revalidation** — *the novel one*: re-executes each record's repro in
    a sandbox on a schedule and on dependency bumps. CI for memories.
  - **D4 lifecycle** — reinforces helpful records, decays never-hit ones
    (archive, never delete; beware evicting rare-but-critical traps —
    salience beats recency).
  - **D5 abstraction** — induces convention cards from clusters of related
    episodes; the episodes remain as evidence links.

## Evaluation

- **Trap-avoidance eval** — the project's regression suite for the store
  itself: walk an agent toward each recorded trap, memory on/off, and score
  avoidance plus steps/tokens. No published suite measures this.

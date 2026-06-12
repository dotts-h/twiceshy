# ADR-0001: twiceshy architecture — git-backed experience records, derived SQLite index, hybrid injection

- **Status:** Accepted (2026-06-12)
- **Deciders:** horia
- **Grounding:** [docs/research/EXPERIENCE_SERVICE_RESEARCH.md](../research/EXPERIENCE_SERVICE_RESEARCH.md)
  — a verified 250-source research fan-out. Its conclusions are adopted here
  as decided architecture, not open questions.

## Context

LLM coding agents are stateless by default and repeat known mistakes on
autopilot. The measured failure shape (research §1): models mirror the
context they are given — stale context in, stale code out (70–90%
deprecated-API rate on outdated-context prompts) — and they do **not**
reliably call knowledge tools unprompted (§3). What moves coding benchmarks
is strategy-level memory distilled from successes *and failures*, delivered
deterministically at decision time with high retrieval precision (§2, §5).
Meanwhile a shared experience store is a studied attack surface: <0.1%
poisoned records yields ≥80% attack success, and query-only attackers can
talk agents into self-poisoning (§6).

twiceshy is the service that closes this loop for our own engineering
experience, self-hosted, at marginal cost ≈ NAS electricity.

## Decision

The following ten decisions are **locked**. Supersede with a new ADR or not
at all.

### 1. Storage: git-backed markdown, everything else derived

Experience records are markdown files (YAML frontmatter + narrative body) in
a git repo — the single source of truth. The only other state is **one
SQLite file** (FTS5 now, sqlite-vec later) derived from the records and
rebuildable at any time. No managed vector DB, no Postgres, no graph DB.
Whole system state = one git repo + one SQLite file; cloud-portable by
construction.

### 2. Record schema

Defined formally in [SCHEMA.md](../SCHEMA.md) + a versioned JSON Schema.
Shape: `kind` (trap|fix|dead-end|convention|workflow); `symptom` (summary,
error_signatures, dual Sentry-style fingerprints: app-specific + generic);
`applies_to` (OSV-style `{ecosystem, package, version-range}`); `resolution`
(root_cause, fix, dead_ends[tried, why_it_failed]); `guard` (fail-to-pass
repro script + guarding test); `provenance` (source, bi-temporal validity
from/until, `superseded_by` — supersede, never delete; usage counters).
Lifecycle: `quarantined → validated → stale | superseded`.

### 3. Retrieval: precedence, fusion, and a hard cap

Precedence: **fingerprint-exact → BM25/FTS5 lexical → dense**, fused with
RRF (k=60) where dense participates, filtered and boosted by stack
fingerprint. Hard cap **k ≤ 3** with a **relevance floor** below which
*nothing* is returned for injection — near-miss injection is the #1 failure
mode; related-but-wrong context measurably hurts agents.

### 4. Hot-path rule: the hook path is embedding-free

The push-channel lookup runs fingerprint + lexical only. Dense search runs
exclusively on explicit pull-channel queries, behind an embedding cache.
Capacity target: ~1000 agents querying continuously on the NAS in §9.

### 5. Injection: hybrid channels, never "hope the agent asks"

- **Push:** Claude Code `UserPromptSubmit`/`PreToolUse` hook → plain HTTP
  endpoint → 1–3 short trap cards via `additionalContext`, high-confidence
  matches only.
- **Pull:** MCP tools (`search_experience`, `get_experience`,
  `record_experience`) over **streamable HTTP** — not the deprecated
  HTTP+SSE transport.
- **Index:** generated skill / `AGENTS.md` one-liners for ambient awareness.

### 6. Security: git is the trust boundary

Agent-proposed records land in **quarantine**; promotion to `validated`
requires sandbox validation + human PR review. A new experience record IS a
pull request — diffable, signed, revertable, blame-able. Quarantined records
never enter the push channel. Provenance on every record.

### 7. Doctors: background jobs, delta-updates only

Never whole-store rewrites (context collapse). D1 dedup/reconcile (LLM
arbitrates ADD/UPDATE/SUPERSEDE/NOOP against top-k similar); D2 staleness
(cross-check `applies_to` against live versions/docs, Context7-style);
**D3 revalidation — the novel one** (re-execute each record's repro in a
sandbox on a schedule and on dependency bumps, SWE-bench fail-to-pass
discipline); D4 lifecycle (reinforce helpful, decay never-hit — archive,
never delete; beware evicting rare-but-critical traps); D5 abstraction
(induce convention cards from episode clusters).

### 8. Evals: trap-avoidance regression suite

Walk an agent toward each recorded trap, memory on/off; score avoidance plus
steps/tokens. Publishable novelty — no published suite measures it.

### 9. Deployment: one Go service in Docker on the NAS

UGREEN DXP4800 Plus (Pentium Gold 8505, AVX2, 40 GB RAM, 12 TB, gigabit
LAN). Personal access via Tailscale; future public access via Cloudflare
Tunnel + bearer tokens. Local embeddings (fastembed/ONNX or Ollama
nomic-embed). Doctor LLM calls: local Ollama or Haiku-class API,
batch/overnight.

### 10. Cost & license doctrine

Marginal cost ≈ NAS electricity; no managed services. Code is AGPL-3.0;
the full licensing strategy (dual-licensing option, CLA, separate-process
paid services, separately licensed experience packs) is
[ADR-0002](ADR-0002-licensing-strategy.md).

## Consequences

- Phase 1 ships only the read path: parser/validator, FTS5 index,
  fingerprint + lexical search, MCP server (`search_experience`,
  `get_experience`). Dense search, hooks, write path, and doctors are
  tracked issues, not code, until their phase.
- The index can always be deleted and rebuilt; migrations are "re-run the
  indexer," never SQL migrations of authoritative data.
- Every retrieval feature must answer: "does this increase near-miss risk?"
  The relevance floor and k≤3 cap are invariants, not tunables to disable.
- The hot path must stay fast and embedding-free even when dense search
  lands; that separation is structural (different endpoints), not a flag.
- Record deletion is not implemented anywhere; supersession is.

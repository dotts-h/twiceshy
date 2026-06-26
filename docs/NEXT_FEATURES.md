# twiceshy — Next-features roadmap

> Research deliverable, **not a commitment**. Each pass re-reads the current code and
> grounds the *next* candidates in it. An item names the seam/files it touches, a rough
> effort (S/M/L), and a "why now." Build-first picks get promoted to docs/issues/;
> the rest stay candidates here until promoted. Decisions that shape them become ADRs.

## Where the product is now

> A short, honest snapshot of what already ships — the differentiators and how deep they
> are — so the next pass reaches for *new leverage* instead of re-deepening a done axis.

- **Read path (Phase 1) — done.** Git-backed markdown records → derived SQLite/FTS5
  index. Fingerprint (app + generic) and lexical (BM25) retrieval, hard cap k≤3,
  relevance floor applied as index policy (ADR-0004) on the `Retrieve` seam (ADR-0007).
  Hot path is embedding-free by rule (ADR-0001 §4).
- **Pull channel (Phase 1/3) — done.** MCP tools `search_experience`, `get_experience`,
  `record_experience` over streamable HTTP (`twiceshy serve`, bearer-token only).
- **Write path (Phase 3) — done.** `record_experience` is propose-only: dedup-at-ingest
  (`index.Assess` / `ingest.Prepare`), born `quarantined`, git/PR is the trust boundary
  (ADR-0008). Quarantined records never reach the push channel.
- **Corpus — decoupled.** The live corpus is now the separate `twiceshy-corpus`
  data repo (ADR-0021); this engine repo keeps only a frozen fixture for tests
  (see `docs/CODEBASE_MAP.md`).
- **Now ships:** push channel, dense pull retrieval, telemetry, doctors/repro,
  corpus importer, evals, spools, and autonomous promote/demote. Remaining from
  that stale phase list: none; derive new work from current issues/ADRs and
  `docs/CODEBASE_MAP.md`.

## Tiers / sequencing

> Candidates grouped into value×fit tiers, then a recommended build order. Mark the
> lead pick **BUILD FIRST**; for each item give: what · why now · what it touches · ADR?

The committed goal is: **build the remaining program in order → seed the corpus →
deploy** (NAS Docker = always-on server; brain = engine: importer + doctors' sandbox
repro execution + evals). Hosting per ADR-0001 §9. Recommended build order:

1. **#7 corpus importer — BUILD FIRST** (L). *What:* `twiceshy ingest` emitting
   `quarantined` records from license-clean structured sources, precision-first
   (codemods + GitChameleon, then OSV/GHSA, CVE, endoflife, changelog facts). *Why now:*
   the corpus is the moat and is essentially empty; this is the leverage. *Touches:*
   schema (`provenance.source_license` + `source_url`, additive — its first slice),
   `internal/record`, `schema/`, a new `internal/ingest`-adjacent importer, pack-builder
   exclusion. *ADR:* ADR-0003 (+ design `corpus-bootstrap.md`, appendix issue draft).
2. **#4 doctors** (L, epic). *What:* D1 dedup/reconcile, D2 staleness, **D3 revalidation
   (the novel one — re-run repros in a sandbox; promotes quarantine→validated)**, D4
   lifecycle, D5 abstraction; delta-only, never whole-store rewrites. *Why now:* nothing
   imported can become *validated* (push-eligible) without D3's fail-to-pass guard.
   *Touches:* new doctor jobs, sandbox runner (brain engine, isolated containers).
   *ADR:* ADR-0001 §7.
3. **#6 dense retrieval** (M, parallelizable). *What:* sqlite-vec + reciprocal-rank
   fusion behind an embedding cache; pull-channel only (hot path stays embedding-free).
   Then score-banding (ADR-0006) to replace the coarse `DefaultFloor` (TECH_DEBT L6/L7).
   *Why now:* improves pull-search precision; unblocks ADR-0006. Independent of #4/#7 —
   buildable any time. *Touches:* `internal/index`, new embedding seam. *ADR:* ADR-0006.
4. **#2 push path** (L, epic). *What:* `UserPromptSubmit`/`PreToolUse` hook → plain HTTP
   endpoint → 1–3 trap cards via `additionalContext`, high-confidence + validated only.
   *Why now:* this is the auto-injection that makes twiceshy fire "in each implementation"
   instead of waiting to be asked. *Touches:* new HTTP endpoint, trap-card renderer, hook
   script, ambient index skill. *ADR:* ADR-0001 §5. *Dep:* needs validated records (#4).
5. **#5 evals** (M). *What:* trap-avoidance regression suite — walk an agent toward each
   recorded trap, memory on/off, score avoidance + steps/tokens. *Why now:* prove the
   store helps before relying on it; publishable novelty. *Touches:* eval harness.
   *ADR:* ADR-0001 §8. *Dep:* needs the corpus + push (#2).

## Numbering

> The reconciled high-water marks (highest issue / ADR on disk) and which numbers this
> pass claims — so promoted issues and ADRs never collide across passes.

- **Issue numbers are phase-aligned** (ADR cross-references already use them): #1 Phase 0
  seed (done), #2 push path, #3 write path (done), #4 doctors, #5 evals, #6 dense
  retrieval, #7 corpus importer.
- **High-water before this pass:** issues — none on disk (referenced only in ADRs);
  ADRs — 0008.
- **This pass claims:** issues **0002, 0004, 0005, 0006, 0007** (the open remaining
  phases), and files **0001 / 0003** as closed for index completeness. No new ADRs
  (dense banding = ADR-0006; importer = ADR-0003; doctors/push/evals = ADR-0001).

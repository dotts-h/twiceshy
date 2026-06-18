# Retro 0001 — building the remaining program (importer → dense → security → doctors → deploy-ready)

**Session:** 2026-06-18. **Scope:** carried twiceshy from "Phase 1+3 done, roadmap
empty" to a deploy-ready single-tenant service. 16 PRs (#25–#40), all self-reviewed,
verified on real signals, self-merged green.

> Process learnings only (product decisions are ADRs). Durable lessons here are
> folded into [CONVENTIONS.md](../CONVENTIONS.md); this file carries the delta + story.

## What shipped
Roadmap filed (epic #0008) → corpus importer #0007 (go/osv/py adapters + pack-builder
license exclusion + near-miss guard) → dense retrieval #0006 (pure-Go cosine + RRF,
ADR-0009) → security workstream (Gemini threat analysis → ingestion safety gate #0011 +
app-hardening #0013) → doctors framework + D2 #0004 (ADR-0010) → deploy artifact
(Dockerfile + DEPLOY.md) → pre-deploy review (security + architecture + code-quality)
with cleanups + docs sync.

## What went well
- **Fork-subagent + self-review-gate cadence.** Delegate each slice's implementation to
  a fresh-context fork (TDD, `make ci` green, no commit); I review the diff, **verify on
  a real signal**, gate, self-merge. Kept my context clean across 16 PRs and the gate
  caught real issues a green build wouldn't.
- **Verify-on-a-real-signal earned its keep, repeatedly.** WebFetching the live GHSA
  advisories caught a Log4Shell `applies_to` range oversimplification; reviewing the
  pack classifier caught a CC-BY-NC/ND hole that would have leaked NonCommercial records
  into a commercial pack; a live `ingest`/container smoke caught the uid-65532 volume
  perms. None were visible from "tests pass."
- **"No machinery without a consumer/substrate" held the line.** Deferred D1/D3/D4/D5,
  the endoflife sidecar, score-banding, and sqlite-vec — each with a documented trigger
  (ADR-0009/0010, TECH_DEBT) — instead of building speculative scaffolding.

## What went wrong / dead-ends (the expensive learnings)
1. **CI status polling burned ~15 min on a false signal.** The forge's *combined* commit
   status `.state` is unreliable (returns a stale/partial verdict), and my poll did
   `group_by(.context) | .[-1]` which read the **oldest** (pending) status per context —
   so a finished run looked "all pending," and a stale prior-commit failure looked like a
   fresh failure. **Fix:** poll `actions/tasks` by `head_sha` for terminal per-run
   success/failure. → CONVENTIONS.
2. **gitleaks flagged the secret-*detector*'s own test fixtures**, and — the second bite —
   `gitleaks detect` scans the **whole commit range**, so a *follow-up* fix commit didn't
   clear it (the secret persisted in branch history); I had to squash to one clean commit.
   **Fix:** secret-shaped test data is assembled at runtime (never a literal token in any
   commit). → REGRESSIONS dead-ends + CONVENTIONS.
3. **sqlite-vec (named in ADR-0001/0006/#0006) collided with the locked CGO-free build.**
   The plan assumed a vector store without checking it against `CGO_ENABLED=0`
   cross-compilation. Resolved to pure-Go cosine (ADR-0009). **Lesson:** a roadmap item
   naming a dependency/storage tech must be checked against locked build constraints
   before it's committed to. → CONVENTIONS.
4. **Tracking hygiene slip:** #0011 was merged but its issue status wasn't flipped to
   `closed`, so the picker kept recommending it. **Fix:** close-out (status + INDEX) folds
   into the same PR that ships the work — caught and corrected.
5. **The roadmap over-specified doctors.** #0004 filed five doctors, but four had no
   substrate yet (no runnable repros for D3, no usage tracking for D4, Assess covers D1,
   thin corpus for D5). Filing the *work* is fine; building it isn't, until the input
   exists. ADR-0010 records the deferral + triggers.

## Forward
Deploy is the only remaining step (single-tenant, LAN-only, awaiting greenlight). Deferred
with triggers: push #0002 (needs D3/runnable repros), evals #0005, ongoing-maintenance
#0014, D1/D3/D4/D5, and the Tier-B public release #0010 (multi-tenant + trial + anti-abuse).

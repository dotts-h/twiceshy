---
id: 0094
title: Higher-signal experience capture — git fail→fix miner + `twiceshy learned` command (structurally beats transcript mining)
status: in-progress
severity: medium
group:
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0065]
  regression:
assets: []
---

## Summary

The corpus is ~99% OSV/CVE advisories and near-zero dev-stack craft traps. The
intended fix — retro-mining session transcripts (#0065) — is both indirect
(reconstructing a lesson from ~80 KB of session noise) and blocked by the local
analyzer's 4096-token context overflowing on real transcripts. Codex duck
(2026-06-26) reframed it: capture lessons **closer to the correction event**.

## Approach (structurally better than transcript mining)

1. **Git fail→fix miner** — walk our repos' history for fix-shaped, non-merge
   commits (subject matches fix/bug/regression/revert/race/leak/panic/…), format
   each as a small payload (commit message + bounded diff) and enqueue to the SAME
   retro queue. The existing `retro-intake` drain analyzes it off-pool (LAN-private
   Ollama) into QUARANTINED drafts the judge gates. **Small inputs fit 4096 ctx —
   no chunking needed**, and a commit is the actual problem→fix→(test) evidence.
   PROVEN 2026-06-26: one fix commit → 3 candidates (the trap high-quality).
2. **`twiceshy learned` capture command** — low-friction, agent/human-invoked at
   resolution time (`--error --root-cause --fix --verified-by`) → quarantined
   draft. The durable going-forward engine; beats hoping a 20B model reconstructs
   the lesson from a transcript.

## Quality gate (Codex duck — must apply before promote)

A draft promotes only with: concrete symptom/error-signature + specific root cause
+ an **applied/verified** fix + repro/test/verification + scope (tool/version).
Reject "advice", "probably/maybe/consider", or unreproduced claims. **Dedup on
normalized lesson-identity** (ecosystem+tool+error-sig+root-cause+fix), NOT
transcript/commit hash — the same lesson recurs across commits/sessions. Tag
provenance (`git-history` / `authored` / `osv` / `retro-transcript`) so retrieval
prefers validated craft traps over mined drafts. Mine for *candidates*, judge for
*truth* — only promote lessons grounded in observed outcomes, not agent narration
(feedback-loop guard).

## Status / next

- [x] git fail→fix miner (proven; 12 dev-stack drafts → corpus PR #34/#35, quarantined)
- [x] **analyzer robustness** — MAXDIFF 8000→3500 (fit 4096 ctx; ~50%→~15% failure) + `retro-intake`
      now dead-letters an unprocessable entry to `<queue>/dead/` and continues instead of aborting the
      whole drain (engine PR #391, `retro.ErrUnprocessable`). Poison pill no longer blocks the pipeline.
- [x] scale miner + automate — `git-miner-seen` ledger + `twiceshy-git-miner.timer` (daily 01:10) →
      `twiceshy-retro.timer` drain → quarantine → `twiceshy-validate` judge. 47 fix-commits queued.
- [x] **tighten the promote gate** — `HasSubstantiveRootCause` deterministic pre-gate (engine PR #393):
      holds any record whose `root_cause` is empty / "None" / "N/A" (advice, not a trap) cheaply before
      attestation+panel, every path. Calibrated on the first batch: holds exp-2770/2771, passes the 8
      traps. FOLLOW-UP: extend with error_sig-required + hedge-word reject + provenance-aware bar.
- [x] `twiceshy learned` command (engine feature; separate PR + tests) — the going-forward capture path
      (PR #413: write-to-corpus + `-stdout`, repeatable `-error`/`-dead-end`, idempotent capture,
      permissive bar — symptom-only captures are recorded but HELD by the root-cause pre-gate via a
      `None …` placeholder. Implemented by Composer 2.5, reviewed+reconciled by Claude.)
- [ ] defer: transcript chunking + 317-session backfill (only for uncommitted dead-ends)

## Notes

Reuses the existing queue → `retro-intake` → quarantine → judge/promote pipeline
(#0065, ADR-0018) — no engine change for the miner (it's a queue producer). The
transcript-chunking fix is deferred because this path obsoletes it for the
high-value (committed-fix) subset.

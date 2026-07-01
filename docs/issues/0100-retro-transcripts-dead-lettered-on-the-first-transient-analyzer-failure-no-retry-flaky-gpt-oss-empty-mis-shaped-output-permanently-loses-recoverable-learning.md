---
id: 0100
title: Retro transcripts dead-lettered on the first transient analyzer failure (no retry) — flaky gpt-oss empty/mis-shaped output permanently loses recoverable learning
status: closed
severity: high
group: 0064
depends_on: []
forgejo: 467
links:
  adr: docs/adr/ADR-0018-session-retro-capture.md
  prs: [430]
  issues: [0099, 0065]
  regression: docs/REGRESSIONS.md#retro-deadletter-no-retry-0100
assets: []
---

## Summary

The retro drain **permanently dead-letters a transcript on the FIRST analyzer
`ErrUnprocessable`, with no retry** (`cmd/twiceshy/retro.go:165`, commented
"deterministic content failure"). But the failures are predominantly
**transient** model flakiness — `gpt-oss:20b` intermittently returns empty
content or mis-shaped JSON. Re-running recovers the transcript. Result: **40
dead-lettered transcripts = lost learning**, much of it recoverable.

## Repro
1. Copy dead-lettered transcripts into a temp queue and re-analyze through the
   same binary + live `:8729` shim:
   `twiceshy retro-intake -queue <tmp> -corpus <corpus> -db <tmp.db> -dry-run -analyzer-model gpt-oss:20b`
2. Run it twice.

Expected (if dead-letter were deterministic): same transcripts fail both passes.

Actual: **non-deterministic.** Pass 1 → 3/5 fail `502 empty model content`,
1/5 fails `decode candidates: cannot unmarshal string`, 1/5 succeeds. Pass 2 →
the SAME 5 produce **0 unprocessable** (the empty-content ones returned valid
`{"candidates":[]}`; `git-chat-ecc5ed` produced 4 candidates both passes). The
first-failure dead-letter discarded recoverable transcripts.

## Evidence
- Dead-letter logic — `cmd/twiceshy/retro.go:165`:
  ```go
  if errors.Is(err, retro.ErrUnprocessable) {
      // Deterministic content failure — dead-letter this entry ...
      os.Rename(f, filepath.Join(dead, base))
  }
  ```
- `ErrUnprocessable` is raised by `internal/retro/model.go` on non-200, empty
  response, OR `decode candidates` failure — none of which are guaranteed
  deterministic with a stochastic local model.
- Shim (`scripts/retro-analyzer-shim.py`, `call_upstream`) retries on HTTP
  429/5xx **but not** on empty content / unparseable JSON: `raise
  ValueError("empty model content")` propagates straight to a 502, no re-roll.
- Dead-letter census (40 files): by capture source — fix-commit:twiceshy 11,
  fix-commit:chat 11, session-end 9, fix-commit:wayfarer 7, fix-commit:cosul 2.
  Journal failure classes — truncation (`Expecting ',' delimiter` + `Unterminated
  string`) ≈18, `empty model content` 9, `analyzer returned no candidates array` 9.

## Notes
- **Fix (chosen):** make the analyzer **retry on unusable content** — empty
  content, `_extract_json` parse failure, or a result missing the expected array
  — inside the shim's existing bounded retry budget, before surfacing a 502. This
  restores the invariant the Go comment assumes (what reaches the dead-letter path
  is *actually* deterministic) and recovers the dominant transient classes by
  re-rolling the model. Minimal, contained to the shim already brought in-repo by
  #0099.
- **Then reprocess:** move the 40 `dead/*.json` back into the queue so the next
  drain re-analyzes them with the resilient shim; the transient ones are recovered.
- **Out of scope here (follow-up):** truncation that is genuinely deterministic
  (a large candidate set always exceeding the `MAX_TOKENS=4096` output cap) is a
  separate lever (raise the cap / chunk); track separately if re-rolls don't clear
  the truncation class. A deeper Go-side attempt-counter (persisted across runs)
  is the heavier alternative — deferred unless shim re-roll proves insufficient.
- Continues the #0099 / measurement-chain robustness work under epic 0064;
  ADR-0018 (session retro capture), #0065 (the drain).

## Resolution (closed 2026-06-29, PR #430)

`drainRetro` now retries the analyze step up to `analyzeAttempts` (3) on
`ErrUnprocessable` before dead-lettering; transport errors still stop the drain
unchanged, and a genuine poison pill is still dead-lettered after the bounded
retries. Guarded by the transient-recovery + poison-pill tests in
`cmd/twiceshy/retro_intake_test.go`.

**Verified on real data.** The 40 dead-letters were requeued and re-drained with
the fixed binary: **26 drafts recovered, only 2 re-dead-lettered** (genuine poison
pills after 3 retries); the dead-letter dir dropped 40 → 2. Dogfood record
exp-3249 (corpus PR #76); regression row in `docs/REGRESSIONS.md`.

**Follow-up surfaced:** the usage-judge join (`JudgeUsage`) has no retry of its own
— one transient `empty model content` 502 skipped one session's attribution this
run (non-fatal). Worth applying the same bounded re-roll to the join call; tracked
informally here for a future pass.

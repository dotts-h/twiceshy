# #0069 — wire the served-vs-used helpfulness join into retro-intake

**Status:** approved scope (2026-06-28). Implementation → Composer 2.5 (`ask-cursor`)
against the gate; Claude orchestrates + reviews. Smallest piece that makes the helpfulness
signal LIVE on real traffic (#0069 acceptance 2; feeds #0005 slice 2, ADR-0026).

## What exists (reuse, do not re-implement)

- `retro.UsageJudge` / `retro.ModelUsageJudge` / `retro.NewModelUsageJudge(ModelConfig)` —
  judges a transcript → `[]CardVerdict{ID,Used}` (internal/retro/model_usage.go). Tested.
- `retro.RecordHelpfulnessAttributed(ctx, rec ConfirmHelpfuler, verdicts, served map[string]bool)
  (int, error)` — confirms ONLY verdicts that are Used **and** in `served` (internal/retro/helpful.go).
  Tested.
- `telemetry.ServedInSession(path, sessionHash) (map[string]bool, error)` — the served set for a
  session, from the #0067 decision log (internal/telemetry/served.go). Tested.
- `*index.Index` satisfies `retro.ConfirmHelpfuler` via `ConfirmHelpful` (internal/index/usage.go).
- `telemetry.Recorder.Hash(s)` = `hex(sha256(salt + s)[:16])` (internal/telemetry/decision.go:115).
- `drainRetro(ctx, analyzer, ix, repo, corpus, queue, opts, out)` — the drain (cmd/twiceshy/retro.go:103).

## Changes

### 1. `internal/telemetry`: standalone `Hash` (so the join hashes WITHOUT a write-Recorder)

Add `func Hash(salt []byte, s string) string` returning `hex(sha256(salt+s)[:16])` — the exact bytes
`Recorder.Hash` computes. Refactor `Recorder.Hash` to `return Hash(r.salt, query)` (keep the nil-recorder
guard). **Behavior must be byte-identical** to today's `Recorder.Hash` — the join's session hash MUST
match the server's logged hash or `ServedInSession` returns empty and the join silently confirms nothing.
(No empty-string special-case in `Hash`; the caller skips empty session ids.)

### 2. `cmd/twiceshy/retro.go`: best-effort helpfulness join in `drainRetro`

Add a small bundle (nil = join disabled = today's behavior):
```go
type helpfulJoin struct {
    judge     retro.UsageJudge
    rec       retro.ConfirmHelpfuler
    servedFor func(sessionID string) (map[string]bool, error) // hash + ServedInSession
}
```
`drainRetro` gains a `join *helpfulJoin` param. Per transcript, AFTER the analyzer succeeds and
**before dequeue**, when `join != nil` and `tr.SessionID != ""`, run the join **best-effort**:
- `served, err := join.servedFor(tr.SessionID)` — on err: log a warning, skip the join (do NOT block).
- `verdicts, err := join.judge.JudgeUsage(ctx, tr.Transcript)` — on err: log a warning, skip.
- `n, err := retro.RecordHelpfulnessAttributed(ctx, join.rec, verdicts, served)` — on err: log a warning
  (with `n`).
- on success: `fmt.Fprintf(out, "  confirmed %d helpful (from %s)\n", n, base)`.

**Invariant: the join must NEVER return early from `drainRetro`, never affect the dequeue, and never
change trap-extraction behavior.** It is secondary measurement; a flaky usage judge or missing decision
log must not jeopardize the primary trap drain (cf. "alerting must never break the loop it watches").
Dry-run: skip the join entirely (it confirms into the live usage table).

### 3. `cmd/twiceshy/retro.go`: build the join in `runRetroIntake`

- New flag `-telemetry-log` (default `getenv("TWICESHY_TELEMETRY_LOG")`) — the #0067 decision log the
  serve process writes. **If empty, the join is DISABLED** (`join = nil`) — opt-in, exactly like the
  serve-side telemetry is opt-in.
- Salt: `getenv("TWICESHY_TELEMETRY_SALT")` (same salt the serve process uses).
- `usageJudge`: `retro.NewModelUsageJudge(retro.ModelConfig{Endpoint: <same url as the analyzer>, Model:
  <same model>})` — reuse the analyzerFromEnv endpoint/model resolution (TWICESHY_RETRO_URL/JUDGE_URL +
  model). Build it next to the analyzer so a misconfigured endpoint fails fast.
- `servedFor := func(sid string) (map[string]bool, error) { return telemetry.ServedInSession(*telemetryLog,
  telemetry.Hash([]byte(salt), sid)) }`
- `join := &helpfulJoin{judge: usageJudge, rec: ix, servedFor: servedFor}` (only when `-telemetry-log` set).

## Gate

**Orchestrator-authored (already in the branch):** `internal/telemetry/hash_test.go` — locks that the
standalone `Hash(salt, s)` is byte-identical to `Recorder{salt}.Hash(s)` (the parity the whole join
depends on). It will fail to compile until `telemetry.Hash` exists (TDD red).

**Composer must add** `cmd/twiceshy/retro_test.go` proving the best-effort join, with a real temp corpus
+ index + an enqueued transcript (SessionID "s1"), `StubAnalyzer{}` (no candidates), and an injected
`helpfulJoin` using `retro.StubUsageJudge` + a counting stub `ConfirmHelpfuler` + a `servedFor` stub:
- **served-filter**: verdicts `[{exp-0001,Used:true},{exp-0002,Used:true}]`, `servedFor("s1")` →
  `{exp-0001:true}` ⇒ the stub recorder confirms **exp-0001 only** (used+served), NOT exp-0002 (used,
  not served). Drain completes; transcript dequeued.
- **best-effort on judge error**: `StubUsageJudge{Err: ...}` ⇒ drain still completes, transcript still
  dequeued, nothing confirmed (the judge error is logged, not propagated).
- **disabled**: `join == nil` ⇒ behaves exactly as today (no confirmations, no panic).

Definition of done: `go test ./... -race` green, `make lint` clean, `make test` green. Reuse seams
(`RecordHelpfulnessAttributed`, `ServedInSession`, `NewModelUsageJudge`, `ConfirmHelpful`) — zero
re-implementation of the attribution/served logic.

## Out of scope (separate #0069/#0005 pieces)

- The precision/recall reporter on real traffic (#0069 acceptance 3).
- #0005 slice-2 agent-task eval harness.
- The scheduled retro-intake drain timer + corpus-PR wrapper (ADR-0026 follow-on #2).

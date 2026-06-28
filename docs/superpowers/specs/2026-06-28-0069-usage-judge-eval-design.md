# #0069 acceptance 3 — usage-judge precision/recall eval

**Status:** approved scope (2026-06-28). Implementation → Composer 2.5 (`ask-cursor`); spec +
gate + review by Claude. Closes #0069's last acceptance: report how accurately the
`ModelUsageJudge` calls served cards used-vs-ignored. Validating the judge is the precondition
for trusting its live output (feeds #0005 slice 2).

Mirrors the existing push eval (`internal/eval` `PushCase`/`RunPush`/`PushReport`): a gold set,
run the seam, micro-average precision/recall. CI-able with a stub judge; run live against the
off-pool shim over the same gold set for the real number.

## What exists (reuse)

- `retro.UsageJudge` (`JudgeUsage(ctx, transcript) ([]retro.CardVerdict, error)`),
  `retro.StubUsageJudge`, `retro.NewModelUsageJudge(ModelConfig)`, `retro.CardVerdict{ID,Used}`.
- `cmd/twiceshy/main.go` `runEval` dispatches `-push`; `modelConfigFromEnv` (in retro.go) resolves
  the off-pool endpoint/model.

## Changes

### 1. `internal/eval`: the usage-judge eval (new file `usage.go`)

```go
// UsageCase is one gold-labeled session: the transcript the judge reads, the cards that were
// SERVED in it, and the subset the agent ACTUALLY used (the gold label). Used must be a subset
// of Served.
type UsageCase struct {
    Name       string
    Transcript string
    Served     []string // ids served/pushed in the session
    Used       []string // gold: the served ids the agent actually applied
}

// UsageReport micro-averages judge accuracy over the gold cases (restricted to SERVED cards —
// a verdict for a never-served id is ignored, exactly as the live join ignores it).
type UsageReport struct {
    Cases int
    TP    int // judge said used AND gold-used
    FP    int // judge said used but gold-ignored
    FN    int // gold-used but judge said ignored (or no verdict)
    Mismatches []string // "case: id (judge=used gold=ignored)" etc., for inspection
}
func (r UsageReport) Precision() float64 // TP/(TP+FP); 1.0 when TP+FP==0
func (r UsageReport) Recall() float64    // TP/(TP+FN); 1.0 when TP+FN==0

// RunUsage runs the judge over each case and accumulates TP/FP/FN. Per case: judge the
// transcript, keep only verdicts whose id is in Served, treat Used==true as the judge's
// positive set; compare to the gold Used set. A judge error aborts (the caller surfaces it —
// the eval needs every case judged to be meaningful, unlike the best-effort production join).
func RunUsage(ctx context.Context, judge retro.UsageJudge, cases []UsageCase) (UsageReport, error)

// UsageGold is the hand-labeled gold set. SYNTHETIC for now (no real-traffic telemetry yet —
// see the activation note in #0069); each transcript is written so the use/ignore is
// unambiguous, to measure the judge on clear cases first.
func UsageGold() []UsageCase
```

**`UsageGold` — author 3 cases (Composer drafts the transcript prose; the Served/Used labels are
fixed here, and the prose MUST make each label unambiguous):**
1. *one-used-one-ignored* — Served `[exp-0001, exp-0017]`, Used `[exp-0001]`. Transcript: the agent
   hits an FTS5 `syntax error` on a dotted module path, recalls exp-0001 and quotes the tokens
   (clearly applies it); never touches TMPDIR/exec (exp-0017 ignored).
2. *both-used* — Served `[exp-0002, exp-0006]`, Used `[exp-0002, exp-0006]`. Transcript: the agent
   fixes a bm25 `ORDER BY rank` (exp-0002) AND a Go ServeMux `POST` fall-through 405 (exp-0006).
3. *none-used* — Served `[exp-0001]`, Used `[]`. Transcript: the agent works an unrelated CSS-grid
   mobile bug; exp-0001 is present but irrelevant and never applied.

### 2. `cmd/twiceshy/main.go`: `-usage` mode on `runEval`

Add `usage := fs.Bool("usage", false, "run the usage-judge precision/recall eval over the gold set")`.
When set, call a new `runEvalUsage(ctx, getenv, out, *asJSON)` (so `runEval` needs `getenv` — thread
it through the dispatch in `run()`): build the judge via `retro.NewModelUsageJudge(<modelConfigFromEnv>)`
and run `eval.RunUsage(ctx, judge, eval.UsageGold())`; print `precision`, `recall`, `TP/FP/FN`, and the
mismatches (JSON when `-json`). A judge-build/endpoint error returns it (fail loud — unlike the
production join, the eval is explicitly invoked and must reach the model).

## Gate

**Orchestrator-authored (already in the branch):** `internal/eval/usage_test.go` — drives `RunUsage`
with a `retro.StubUsageJudge` over a controlled inline case and asserts the TP/FP/FN counts and the
Precision/Recall math (served-restriction included). Locks the aggregation, which is the part that
can be subtly wrong; the live judge accuracy is measured by running the command, not in CI.

**Composer adds:** `UsageGold` (the 3 labeled cases with unambiguous transcripts) and the `-usage`
command wiring; a small test that `UsageGold` is well-formed (every `Used` ⊆ `Served`, non-empty
transcripts).

Definition of done: `go test ./... -race`, `make lint`, `make test` all green; reuse the existing
seams (no re-implementation of JudgeUsage/CardVerdict). Do NOT touch the production join.

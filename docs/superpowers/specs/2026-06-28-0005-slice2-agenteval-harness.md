# #0005 slice 2 — agent-task (memory on/off) eval harness

**Status:** approved scope (2026-06-28). Implementation → Composer 2.5 (`ask-cursor`); spec +
gate + review by Claude. This is the **harness foundation** for slice 2 (the GitChameleon-style
agent-task eval): drive an agent toward each trap with memory on vs off and score avoidance +
steps/tokens. It delivers the reusable infrastructure + executable task cases; plugging in a *real*
off-pool agent runner (and running it) is the explicit follow-up — this PR does NOT make LLM calls.

## Why a new package

`internal/eval` is the retrieval eval (slice 1, deterministic, no agent). The agent-task eval is a
different shape (runs an agent, executes its output) — put it in a new `internal/agenteval` so the
seam boundary stays clean (pure-core: the harness aggregates; the runner/verifier edges are injected).

## Contract (`internal/agenteval/agenteval.go`)

```go
// TaskCase is one trap-avoidance probe: a coding task where the trap would bite, the experience
// card injected in the memory-ON arm, and the verify key the Verifier uses to check the output.
type TaskCase struct {
    TrapID   string // the validated trap this probes (e.g. "exp-2868")
    Prompt   string // the task handed to the agent
    Card     string // experience-card text injected ONLY in the memory-on arm
    VerifyID string // opaque key the Verifier maps to an avoidance check (e.g. a scaffold name)
}

// Result is one agent run's output + cost. card=="" is the memory-OFF arm.
type Result struct { Output string; Steps int; Tokens int }

// AgentRunner runs an agent toward prompt. card=="" => memory off; non-empty => memory on (the
// card is made available as experience). Stub in tests; a real impl wraps an off-pool model edge.
type AgentRunner interface { Run(ctx context.Context, prompt, card string) (Result, error) }

// Verifier decides whether the output AVOIDED the trap (true) or hit it (false). For executable
// traps the real impl runs tsc/go on the output; stub in tests.
type Verifier interface { Avoided(ctx context.Context, c TaskCase, output string) (bool, error) }

// Outcome is one (case, arm) result.
type Outcome struct { TrapID string; MemoryOn, Avoided bool; Steps, Tokens int }

// Report aggregates the on-vs-off comparison (the slice-2 headline numbers).
type Report struct {
    Cases                  int
    AvoidedOff, AvoidedOn  int
    StepsOff, StepsOn      int
    TokensOff, TokensOn    int
    Outcomes               []Outcome
}
func (r Report) AvoidanceOff() float64 // AvoidedOff/Cases; 0 when Cases==0
func (r Report) AvoidanceOn() float64  // AvoidedOn/Cases; 0 when Cases==0

// Run drives every case through BOTH arms (off then on), verifies each, and aggregates. A runner
// or verifier error aborts (the eval needs every case to mean anything). The card is passed to the
// runner ONLY in the on-arm; the off-arm always gets "".
func Run(ctx context.Context, runner AgentRunner, verifier Verifier, cases []TaskCase) (Report, error)
```

## Task cases (`agenteval.GoldTasks()`)

Author **3** cases tied to validated, executably-verifiable traps. `Card` = a concise rendering of
that trap's lesson. `Prompt` = a task whose naive answer hits the trap. (Composer writes the prose;
the TrapID/VerifyID are fixed.)
1. `exp-2868` (React 19 `useRef`) — Prompt: "In a React 19 + TS component, create a mutable ref that
   will hold a number, set later." Naive: `useRef<number>()` (TS2554). VerifyID `react19-useref`.
2. `exp-2870` (RN `<View>` text style) — Prompt: "Style a React Native row so its label text is bold."
   Naive: `fontWeight` on `<View>` (TS2769). VerifyID `rn-viewstyle`.
3. `exp-0001` (FTS5 MATCH raw input) — Prompt: "Build a SQLite FTS5 search query from a user string
   that may contain dots/dashes." Naive: raw string into `MATCH` (fts5 syntax error). VerifyID `fts5-match`.

These are DATA only here; the executable verification (the `Verifier` that scaffolds + runs
tsc/go per `VerifyID`) is the follow-up integration, noted in the package doc.

## Gate (`internal/agenteval/agenteval_test.go`) — orchestrator-authored, already in the branch

Drives `Run` with a **stub runner + stub verifier** (no LLM, no exec) to lock the on-vs-off
aggregation — the part that's easy to get wrong:
- 2 cases; the stub verifier returns avoided=false for both OFF-arm runs and avoided=true for both
  ON-arm runs (the expected "card helps" shape) ⇒ `AvoidedOff==0`, `AvoidedOn==2`,
  `AvoidanceOff()==0.0`, `AvoidanceOn()==1.0`; step/token sums add per arm; the off-arm runner is
  called with card=="" and the on-arm with the case's Card (assert the stub saw the right card per arm).
- empty cases ⇒ avoidance 0.0, no panic.

Also: `GoldTasks()` is well-formed (3 cases, every field non-empty, distinct TrapIDs).

Definition of done: `go test ./... -race`, `make lint`, `make test` green. No LLM/network calls in
this PR; the real `AgentRunner` (off-pool model) + executable `Verifier` are the follow-up that turns
the harness into a live number (depends on activation work / a chosen runner).

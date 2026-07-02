// SPDX-License-Identifier: AGPL-3.0-only

// Package agenteval is twiceshy's agent-task eval harness (#0005 slice 2): drive an
// agent toward each trap with memory on vs off and score avoidance plus steps/tokens.
// The harness aggregates; runner and verifier are injected at the edges.
//
// This PR delivers the reusable infrastructure and gold task cases only — no LLM calls
// and no executable verification. A real AgentRunner (off-pool model) and an executable
// Verifier (scaffold + tsc/go per VerifyID) are the follow-up that turns the harness
// into live numbers.
package agenteval

import "context"

// TaskCase is one trap-avoidance probe: a coding task where the trap would bite, the
// experience card injected in the memory-ON arm, and the verify key the Verifier uses
// to check the output.
type TaskCase struct {
	TrapID   string // the validated trap this probes (e.g. "exp-2868")
	Prompt   string // the task handed to the agent
	Card     string // experience-card text injected ONLY in the memory-on arm
	VerifyID string // opaque key the Verifier maps to an avoidance check (e.g. a scaffold name)
	// Deps names the npm packages (with major pins, e.g. "typescript", "@types/react@19")
	// the generic "tsc" verify class installs before type-checking. The literal
	// VerifyIDs ("react19-useref", "rn-viewstyle") keep their own hardcoded deps and
	// ignore this field; it exists for the prospector's generic verify classes.
	Deps []string
	// Control is a correct, trap-avoiding answer to Prompt, used to sanity-check the
	// drafted task and its verify class before it is ever handed to an agent.
	Control string
}

// Result is one agent run's output + cost. card=="" is the memory-OFF arm.
type Result struct {
	Output string
	Steps  int
	Tokens int
}

// AgentRunner runs an agent toward prompt. card=="" => memory off; non-empty => memory on
// (the card is made available as experience). Stub in tests; a real impl wraps an
// off-pool model edge.
type AgentRunner interface {
	Run(ctx context.Context, prompt, card string) (Result, error)
}

// Verifier decides whether the output AVOIDED the trap (true) or hit it (false). For
// executable traps the real impl runs tsc/go on the output; stub in tests.
type Verifier interface {
	Avoided(ctx context.Context, c TaskCase, output string) (bool, error)
}

// Outcome is one (case, arm) result.
type Outcome struct {
	TrapID   string
	MemoryOn bool
	Avoided  bool
	Steps    int
	Tokens   int
}

// Report aggregates the on-vs-off comparison (the slice-2 headline numbers).
type Report struct {
	Cases                 int
	AvoidedOff, AvoidedOn int
	StepsOff, StepsOn     int
	TokensOff, TokensOn   int
	Outcomes              []Outcome
}

// AvoidanceOff returns AvoidedOff/Cases; 0 when Cases==0.
func (r Report) AvoidanceOff() float64 {
	if r.Cases == 0 {
		return 0
	}
	return float64(r.AvoidedOff) / float64(r.Cases)
}

// AvoidanceOn returns AvoidedOn/Cases; 0 when Cases==0.
func (r Report) AvoidanceOn() float64 {
	if r.Cases == 0 {
		return 0
	}
	return float64(r.AvoidedOn) / float64(r.Cases)
}

// Run drives every case through BOTH arms (off then on), verifies each, and aggregates.
// A runner or verifier error aborts. The card is passed to the runner ONLY in the on-arm;
// the off-arm always gets "".
func Run(ctx context.Context, runner AgentRunner, verifier Verifier, cases []TaskCase) (Report, error) {
	rep := Report{Cases: len(cases)}
	for _, c := range cases {
		off, err := runner.Run(ctx, c.Prompt, "")
		if err != nil {
			return Report{}, err
		}
		offAvoided, err := verifier.Avoided(ctx, c, off.Output)
		if err != nil {
			return Report{}, err
		}
		if offAvoided {
			rep.AvoidedOff++
		}
		rep.StepsOff += off.Steps
		rep.TokensOff += off.Tokens
		rep.Outcomes = append(rep.Outcomes, Outcome{
			TrapID:   c.TrapID,
			MemoryOn: false,
			Avoided:  offAvoided,
			Steps:    off.Steps,
			Tokens:   off.Tokens,
		})

		on, err := runner.Run(ctx, c.Prompt, c.Card)
		if err != nil {
			return Report{}, err
		}
		onAvoided, err := verifier.Avoided(ctx, c, on.Output)
		if err != nil {
			return Report{}, err
		}
		if onAvoided {
			rep.AvoidedOn++
		}
		rep.StepsOn += on.Steps
		rep.TokensOn += on.Tokens
		rep.Outcomes = append(rep.Outcomes, Outcome{
			TrapID:   c.TrapID,
			MemoryOn: true,
			Avoided:  onAvoided,
			Steps:    on.Steps,
			Tokens:   on.Tokens,
		})
	}
	return rep, nil
}

// GoldTasks returns the three validated trap-avoidance probes used for the slice-2 eval.
func GoldTasks() []TaskCase {
	return []TaskCase{
		{
			TrapID:   "exp-2868",
			Prompt:   "In a React 19 + TS component, create a mutable ref that will hold a number, set later.",
			Card:     "React 19 @types/react dropped the zero-argument useRef overload (TS2554). Pass an explicit initial value: useRef<number | null>(null) for a mutable ref you fill later.",
			VerifyID: "react19-useref",
		},
		{
			TrapID:   "exp-2870",
			Prompt:   "Style a React Native row so its label text is bold.",
			Card:     "React Native <View> accepts ViewStyle only — fontWeight, fontSize, and color are TextStyle props. Put text styles on a <Text> child, not on the <View>.",
			VerifyID: "rn-viewstyle",
		},
		{
			TrapID:   "exp-0001",
			Prompt:   "Build a SQLite FTS5 search query from a user string that may contain dots/dashes.",
			Card:     "FTS5 MATCH parses its right-hand side as query syntax, not a literal string — dots, dashes, and quotes break barewords. Tokenize input, escape embedded quotes, wrap each token in double quotes, join with spaces.",
			VerifyID: "fts5-match",
		},
	}
}

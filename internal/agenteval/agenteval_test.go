// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"testing"
)

// stubRunner models the "card helps" shape: the OFF arm (card=="") returns the naive output at
// higher cost; the ON arm returns the fixed output at lower cost. It records the card per arm so
// the test can assert the off arm gets no card and the on arm gets the case's Card.
type stubRunner struct {
	offCalls, onCalls int
	cardsOn           []string
}

func (s *stubRunner) Run(_ context.Context, _ string, card string) (Result, error) {
	if card == "" {
		s.offCalls++
		return Result{Output: "naive", Steps: 5, Tokens: 100}, nil
	}
	s.onCalls++
	s.cardsOn = append(s.cardsOn, card)
	return Result{Output: "fixed", Steps: 3, Tokens: 60}, nil
}

// stubVerifier: the "fixed" output avoided the trap; "naive" hit it.
type stubVerifier struct{}

func (stubVerifier) Avoided(_ context.Context, _ TaskCase, output string) (bool, error) {
	return output == "fixed", nil
}

// Run drives both arms per case and aggregates the on-vs-off headline numbers. This locks that
// aggregation (and the card-injection contract: off arm gets "", on arm gets the card) — the part
// that's easy to get subtly wrong; the real avoidance numbers come from a live runner + verifier.
func TestRun_OnVsOffAggregation(t *testing.T) {
	cases := []TaskCase{
		{TrapID: "exp-0001", Prompt: "p1", Card: "card-1", VerifyID: "v1"},
		{TrapID: "exp-0002", Prompt: "p2", Card: "card-2", VerifyID: "v2"},
	}
	runner := &stubRunner{}
	rep, err := Run(context.Background(), runner, stubVerifier{}, cases)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Cases != 2 || rep.AvoidedOff != 0 || rep.AvoidedOn != 2 {
		t.Fatalf("cases/off/on = %d/%d/%d, want 2/0/2", rep.Cases, rep.AvoidedOff, rep.AvoidedOn)
	}
	if rep.AvoidanceOff() != 0.0 || rep.AvoidanceOn() != 1.0 {
		t.Errorf("avoidance off/on = %v/%v, want 0.0/1.0", rep.AvoidanceOff(), rep.AvoidanceOn())
	}
	if rep.StepsOff != 10 || rep.StepsOn != 6 || rep.TokensOff != 200 || rep.TokensOn != 120 {
		t.Errorf("steps off/on = %d/%d, tokens off/on = %d/%d; want 10/6 and 200/120",
			rep.StepsOff, rep.StepsOn, rep.TokensOff, rep.TokensOn)
	}
	if runner.offCalls != 2 || runner.onCalls != 2 {
		t.Fatalf("off/on runner calls = %d/%d, want 2/2 (each case runs both arms)", runner.offCalls, runner.onCalls)
	}
	if len(runner.cardsOn) != 2 || runner.cardsOn[0] != "card-1" || runner.cardsOn[1] != "card-2" {
		t.Errorf("on-arm cards = %v, want [card-1 card-2] (the off arm must inject no card)", runner.cardsOn)
	}
}

func TestRun_EmptyIsZero(t *testing.T) {
	rep, err := Run(context.Background(), &stubRunner{}, stubVerifier{}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.AvoidanceOff() != 0 || rep.AvoidanceOn() != 0 {
		t.Errorf("empty avoidance off/on = %v/%v, want 0/0", rep.AvoidanceOff(), rep.AvoidanceOn())
	}
}

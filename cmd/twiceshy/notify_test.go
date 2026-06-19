// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// recordingAlerter captures the guardrail alerts a run fired, so a test can
// assert which guardrail tripped without a live ntfy server.
type recordingAlerter struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingAlerter) Alert(_ context.Context, event, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingAlerter) has(event string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e == event {
			return true
		}
	}
	return false
}

func TestPromoteCorpus_AlertsOnAnomaly(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	persist := func(string, *record.Record) error { return nil }
	al := &recordingAlerter{}

	_, _, _ = promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxActions: 1}, nil, al, &bytes.Buffer{}, "")
	if !al.has("anomaly") {
		t.Fatalf("an anomalous run must fire an anomaly alert; got %v", al.events)
	}
}

func TestPromoteCorpus_AlertsOnEmergencyStop(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	persist := func(string, *record.Record) error { return nil }
	al := &recordingAlerter{}

	_, _, _ = promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{Paused: true}, nil, al, &bytes.Buffer{}, "")
	if !al.has("emergency_stop") {
		t.Fatalf("an emergency stop must fire an alert; got %v", al.events)
	}
}

func TestPromoteCorpus_AlertsOnBudgetCap(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true}}
	persist := func(string, *record.Record) error { return nil }
	al := &recordingAlerter{}

	_, _, _ = promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxRuns: 1}, nil, al, &bytes.Buffer{}, "")
	if !al.has("budget_cap") {
		t.Fatalf("a budget cap must fire an alert; got %v", al.events)
	}
}

// A clean run fires no guardrail alert (the channel stays quiet on a quiet night).
func TestPromoteCorpus_NoAlertOnCleanRun(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	persist := func(string, *record.Record) error { return nil }
	al := &recordingAlerter{}

	_, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, al, &bytes.Buffer{}, "")
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if len(al.events) != 0 {
		t.Fatalf("a clean run must fire no alerts; got %v", al.events)
	}
}

func TestAdaptCorpus_AlertsOnAnomaly(t *testing.T) {
	recs, runner := adaptDemoting(t, 3)
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	persist := func(string, *record.Record) error { return nil }
	al := &recordingAlerter{}

	_, _, _ = adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{MaxActions: 1}, nil, al, &bytes.Buffer{}, "")
	if !al.has("anomaly") {
		t.Fatalf("an anomalous adapt run must fire an anomaly alert; got %v", al.events)
	}
}

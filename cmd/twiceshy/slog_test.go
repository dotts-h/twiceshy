// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// parseLogEvents decodes the slog JSON lines a run emitted into maps, skipping
// blank lines. A non-JSON line is a hard failure: the write path must emit one
// machine-parseable event per decision (#0035, ADR-0013).
func parseLogEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var events []map[string]any
	sc := bufio.NewScanner(bytes.NewBufferString(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		events = append(events, ev)
	}
	return events
}

// indexByOutcome groups decision events by their outcome field; the summary
// event is returned separately. It asserts the run scope (run_id + stage) is on
// every event so a multi-run log is greppable.
func indexByOutcome(t *testing.T, events []map[string]any, wantStage string) (map[string]map[string]any, map[string]any) {
	t.Helper()
	byOutcome := map[string]map[string]any{}
	var summary map[string]any
	for _, ev := range events {
		if ev["run_id"] == nil || ev["run_id"] == "" {
			t.Fatalf("event missing run_id: %v", ev)
		}
		if ev["stage"] != wantStage {
			t.Fatalf("event stage = %v, want %q: %v", ev["stage"], wantStage, ev)
		}
		outcome, _ := ev["outcome"].(string)
		if outcome == "summary" {
			summary = ev
			continue
		}
		byOutcome[outcome] = ev
	}
	return byOutcome, summary
}

func TestPromoteCorpus_EmitsStructuredLog(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"),               // promoted
		eligibleRec("exp-0101"),               // held
		{ID: "exp-0102", Status: "validated"}, // ineligible
	}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	persist := func(string, *record.Record) error { return nil }

	var logbuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logbuf, nil)).With("run_id", "run-test")

	_, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, logger, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}

	events := parseLogEvents(t, logbuf.String())
	byOutcome, summary := indexByOutcome(t, events, "promote")

	// Every outcome must log — including the silent-today branches.
	for _, want := range []string{"promoted", "held", "ineligible"} {
		ev, ok := byOutcome[want]
		if !ok {
			t.Fatalf("no %q decision event; got %v", want, events)
		}
		if id, _ := ev["record_id"].(string); id == "" {
			t.Fatalf("%q event missing record_id: %v", want, ev)
		}
	}
	if byOutcome["held"]["reason"] == nil {
		t.Fatalf("held event missing reason: %v", byOutcome["held"])
	}

	// The promoted event carries the judge + attestation provenance.
	p := byOutcome["promoted"]
	if p["judge_model"] != "gemini-2.5-pro" {
		t.Fatalf("promoted event missing judge_model: %v", p)
	}
	if p["judge_decision"] != string(judge.Approve) {
		t.Fatalf("promoted event judge_decision = %v, want %q: %v", p["judge_decision"], judge.Approve, p)
	}
	if _, ok := p["reproduced_under"]; !ok {
		t.Fatalf("promoted event missing reproduced_under: %v", p)
	}
	if _, ok := p["duration_ms"]; !ok {
		t.Fatalf("promoted event missing duration_ms: %v", p)
	}

	if summary == nil {
		t.Fatal("no run summary event")
	}
	if summary["promoted"].(float64) != 1 || summary["held"].(float64) != 1 || summary["ineligible"].(float64) != 1 {
		t.Fatalf("summary counts wrong: %v", summary)
	}
	if _, ok := summary["duration_ms"]; !ok {
		t.Fatalf("summary missing duration_ms: %v", summary)
	}
}

// The adapt `held` branch emits no prose today (#0035 acceptance) — it must
// still produce a structured event so a quiet hold is visible in the log.
func TestAdaptCorpus_LogsHeldOutcome(t *testing.T) {
	orig := validatedRec("exp-0043")
	rep := reportRec("exp-0200", "exp-0043")
	recs := []*record.Record{orig, rep}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		// Original still holds, counter could not run → inconclusive → held.
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Inconclusive: true}},
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	persist := func(string, *record.Record) error { return nil }

	var logbuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logbuf, nil)).With("run_id", "run-test")

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, logger, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.held != 1 {
		t.Fatalf("held = %d, want 1", st.held)
	}

	byOutcome, summary := indexByOutcome(t, parseLogEvents(t, logbuf.String()), "adapt")
	held, ok := byOutcome["held"]
	if !ok {
		t.Fatalf("adapt held outcome emitted no event; got %v", byOutcome)
	}
	if id, _ := held["record_id"].(string); id != "exp-0043" {
		t.Fatalf("held event record_id = %v, want exp-0043: %v", held["record_id"], held)
	}
	if summary == nil || summary["held"].(float64) != 1 {
		t.Fatalf("adapt summary wrong: %v", summary)
	}
}

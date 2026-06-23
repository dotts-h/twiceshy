// SPDX-License-Identifier: AGPL-3.0-only

package eval_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/eval"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

func sig(id, summary string, sigs ...string) *record.Record {
	return &record.Record{
		ID:      id,
		Kind:    "trap",
		Status:  "validated",
		Symptom: &record.Symptom{Summary: summary, ErrorSignatures: sigs},
	}
}

func TestCases_DerivesFromSignaturesAndSummary(t *testing.T) {
	recs := []*record.Record{
		sig("exp-0001", "pool dries up", "boom near A", "boom near B"),
		{ID: "exp-0099", Kind: "trap", Status: "validated"},                                              // no symptom → no cases
		{ID: "exp-0100", Kind: "reference", Status: "validated", Symptom: &record.Symptom{Summary: "x"}}, // wrong kind
	}
	cases := eval.Cases(recs)
	// 2 signatures + 1 summary for exp-0001; nothing for the others.
	if len(cases) != 3 {
		t.Fatalf("want 3 cases, got %d: %+v", len(cases), cases)
	}
	var sigN, sumN int
	for _, c := range cases {
		if c.RecordID != "exp-0001" {
			t.Errorf("unexpected case record %q", c.RecordID)
		}
		switch c.Source {
		case "error_signature":
			sigN++
		case "summary":
			sumN++
		}
	}
	if sigN != 2 || sumN != 1 {
		t.Errorf("want 2 signature + 1 summary cases, got %d/%d", sigN, sumN)
	}
}

// Cases must exclude non-validated records entirely (quarantined/disputed are
// unretrievable by design — letting them into the eval would inflate recall
// against records an agent can never pull, masking a real gap) and must skip
// empty/whitespace-only signatures. Exercises eval.go's status filter and
// blank-signature skip, both untested by TestCases_DerivesFromSignaturesAndSummary.
func TestCases_FiltersStatusAndBlankSignatures(t *testing.T) {
	recs := []*record.Record{
		// quarantined record with a real symptom+signatures: must yield ZERO cases
		{ID: "exp-q", Kind: "trap", Status: "quarantined",
			Symptom: &record.Symptom{Summary: "pool dries", ErrorSignatures: []string{"boom"}}},
		// disputed too, for good measure
		{ID: "exp-d", Kind: "trap", Status: "disputed",
			Symptom: &record.Symptom{Summary: "x", ErrorSignatures: []string{"y"}}},
		// validated record whose signatures include blank/whitespace-only entries: those drop
		{ID: "exp-v", Kind: "trap", Status: "validated",
			Symptom: &record.Symptom{Summary: "real summary",
				ErrorSignatures: []string{"", "  ", "real sig"}}},
	}
	cases := eval.Cases(recs)
	// only exp-v contributes: 1 non-blank signature + 1 summary = 2 cases.
	if len(cases) != 2 {
		t.Fatalf("want 2 cases, got %d: %+v", len(cases), cases)
	}
	for _, c := range cases {
		if c.RecordID != "exp-v" {
			t.Errorf("non-validated record leaked into eval: %q", c.RecordID)
		}
		if strings.TrimSpace(c.Query) == "" {
			t.Errorf("blank signature became a case: %q", c.Query)
		}
	}
}

// stubSearcher returns a programmed hit list per query.
type stubSearcher struct{ byQuery map[string][]string }

func (s stubSearcher) Search(_ context.Context, q index.Query) ([]index.Hit, error) {
	var hits []index.Hit
	for _, id := range s.byQuery[q.Text] {
		hits = append(hits, index.Hit{ID: id})
	}
	return hits, nil
}

func TestRun_ScoresRecallRankNearMiss(t *testing.T) {
	cases := []eval.Case{
		{RecordID: "A", Query: "qa", Source: "error_signature"}, // found rank 1
		{RecordID: "B", Query: "qb", Source: "error_signature"}, // found rank 2, near-miss (C on top)
		{RecordID: "D", Query: "qd", Source: "summary"},         // not found
	}
	s := stubSearcher{byQuery: map[string][]string{
		"qa": {"A", "X"},
		"qb": {"C", "B"},
		"qd": {"Y", "Z"},
	}}
	rep, err := eval.Run(context.Background(), s, cases, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Found != 2 {
		t.Errorf("found=%d, want 2", rep.Found)
	}
	if rep.RecallAtK != 2.0/3.0 {
		t.Errorf("recall=%v, want 2/3", rep.RecallAtK)
	}
	// MRR = (1/1 + 1/2 + 0) / 3
	if want := (1.0 + 0.5) / 3.0; rep.MRR != want {
		t.Errorf("mrr=%v, want %v", rep.MRR, want)
	}
	// near-miss: qb (C on top) and qd (Y on top) → 2/3.
	if want := 2.0 / 3.0; rep.NearMissRate != want {
		t.Errorf("nearMissRate=%v, want %v", rep.NearMissRate, want)
	}
	// Per-case rank check.
	for _, r := range rep.Results {
		switch r.RecordID {
		case "A":
			if r.Rank != 1 || !r.Found {
				t.Errorf("A: rank=%d found=%v", r.Rank, r.Found)
			}
		case "B":
			if r.Rank != 2 || !r.NearMiss() {
				t.Errorf("B: rank=%d nearMiss=%v", r.Rank, r.NearMiss())
			}
		case "D":
			if r.Found {
				t.Errorf("D must be not found")
			}
		}
	}
}

// stubPusher returns a programmed hit list per query (mirrors stubSearcher).
type stubPusher struct{ byQuery map[string][]string }

func (p stubPusher) RetrievePush(_ context.Context, q index.Query) ([]index.Hit, error) {
	var hits []index.Hit
	for _, id := range p.byQuery[q.Text] {
		hits = append(hits, index.Hit{ID: id})
	}
	return hits, nil
}

func TestRunPush_PrecisionRecallAndClassification(t *testing.T) {
	cases := []eval.PushCase{
		{Query: "neg-clean"},                   // negative, no injection → clean
		{Query: "neg-leak"},                    // negative, injects → false injection
		{Query: "pos-hit", ExpectID: "exp-1"},  // positive, expected surfaced → recalled
		{Query: "pos-miss", ExpectID: "exp-2"}, // positive, expected absent → miss
	}
	p := stubPusher{byQuery: map[string][]string{
		"neg-leak": {"exp-9"},
		"pos-hit":  {"exp-1", "exp-3"},
		"pos-miss": {"exp-7"},
	}}
	rep, err := eval.RunPush(context.Background(), p, cases)
	if err != nil {
		t.Fatalf("RunPush: %v", err)
	}
	if rep.Negatives != 2 || rep.FalseInjections != 1 {
		t.Errorf("negatives=%d falseInjections=%d, want 2/1", rep.Negatives, rep.FalseInjections)
	}
	if rep.Positives != 2 || rep.Recalled != 1 {
		t.Errorf("positives=%d recalled=%d, want 2/1", rep.Positives, rep.Recalled)
	}
	if rep.Precision() != 0.5 || rep.Recall() != 0.5 {
		t.Errorf("precision=%v recall=%v, want 0.5/0.5", rep.Precision(), rep.Recall())
	}
	if len(rep.Leaks) != 1 || !strings.Contains(rep.Leaks[0], "exp-9") {
		t.Errorf("leaks=%v, want one mentioning exp-9", rep.Leaks)
	}
	if len(rep.Misses) != 1 || !strings.Contains(rep.Misses[0], "exp-2") {
		t.Errorf("misses=%v, want one mentioning exp-2", rep.Misses)
	}

	// Empty report: precision/recall are 1.0, not NaN (no negatives/positives).
	empty := eval.PushReport{}
	if empty.Precision() != 1 || empty.Recall() != 1 {
		t.Errorf("empty precision/recall = %v/%v, want 1/1", empty.Precision(), empty.Recall())
	}
}

func TestRun_EmptyCasesIsZeroNotNaN(t *testing.T) {
	rep, err := eval.Run(context.Background(), stubSearcher{}, nil, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.RecallAtK != 0 || rep.MRR != 0 || rep.NearMissRate != 0 {
		t.Errorf("empty eval must be all-zero, got %+v", rep)
	}
}

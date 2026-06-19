// SPDX-License-Identifier: AGPL-3.0-only

package eval_test

import (
	"context"
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

func TestRun_EmptyCasesIsZeroNotNaN(t *testing.T) {
	rep, err := eval.Run(context.Background(), stubSearcher{}, nil, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.RecallAtK != 0 || rep.MRR != 0 || rep.NearMissRate != 0 {
		t.Errorf("empty eval must be all-zero, got %+v", rep)
	}
}

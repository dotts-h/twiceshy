// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func validatedRec(id string) *record.Record {
	rp := "experience/repro/" + id + ".sh"
	at := "2026-06-12"
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "trap", Status: "validated",
		Title:      "a validated record with a sufficiently long title here",
		Symptom:    &record.Symptom{Summary: "x"},
		Resolution: &record.Resolution{RootCause: "x", Fix: "x"},
		Guard:      &record.Guard{Repro: &rp},
		Provenance: record.Provenance{
			Source: record.Source{Author: "a"}, RecordedAt: "2026-06-12",
			ValidatedAt: &at, Valid: record.Validity{From: "2026-06-12"},
		},
		Body: "b", Path: "experience/2026/" + id[len("exp-"):] + "-x.md",
	}
}

func reportRec(id, disputes string) *record.Record {
	d := disputes
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "dead-end", Status: "quarantined",
		Title:      "Outcome report against " + disputes + " long enough title",
		Symptom:    &record.Symptom{Summary: "did not hold"},
		Resolution: &record.Resolution{DeadEnds: []record.DeadEnd{{Tried: "x", WhyItFailed: "y"}}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "a"}, RecordedAt: "2026-06-19",
			Valid: record.Validity{From: "2026-06-19"}, Disputes: &d,
		},
		Body: "b", Path: "experience/2026/" + id[len("exp-"):] + "-r.md",
	}
}

func TestRunAdapt_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", corpus}, &buf, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("the counter-evidence gate without a judge must fail safe; got %v", err)
	}
}

func TestRunAdapt_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", corpus, "-dry-run"}, &buf, func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}

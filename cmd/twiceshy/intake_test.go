// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// intake-reports turns queued outcome reports into quarantined counter-records on
// disk, so the next adapt processes them with no human paste step (#0042,
// ADR-0013 §E1). Ids are allocated sequentially against the corpus, so two
// reports queued before a drain never collide.
func TestIntakeReports_MaterializesQueueIntoCorpus(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "queue")
	for _, r := range []spool.Report{
		{RecordID: "exp-0043", Outcome: "failed", Evidence: "go build still errors", Author: "agent-x", ReportedAt: "2026-06-19T12:00:00Z"},
		{RecordID: "exp-0044", Outcome: "reproduced", Author: "agent-y", ReportedAt: "2026-06-19T12:00:01Z"},
	} {
		if _, err := spool.Enqueue(queue, r); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := runIntakeReports([]string{"-corpus", corpus, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeReports: %v", err)
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 materialized counter-records, got %d", len(recs))
	}
	disputed := map[string]bool{}
	for _, r := range recs {
		if r.Status != "quarantined" {
			t.Fatalf("counter-record %s must be quarantined, got %q", r.ID, r.Status)
		}
		// reportDisputes is the exact predicate adapt uses to pick up reports.
		disputed[reportDisputes(r)] = true
	}
	if !disputed["exp-0043"] || !disputed["exp-0044"] {
		t.Fatalf("materialized records must dispute their targets; got %v", disputed)
	}
	if recs[0].ID == recs[1].ID {
		t.Fatalf("id collision across reports: %s", recs[0].ID)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained after intake; %d left", len(paths))
	}
}

func TestIntakeReports_BaseAllocatesPastBaseMax(t *testing.T) {
	corpus, base := corpusWithLocal2758AndBase2768(t)
	queue := filepath.Join(t.TempDir(), "queue")
	if _, err := spool.Enqueue(queue, spool.Report{
		RecordID:   "exp-0043",
		Outcome:    "failed",
		Evidence:   "go build still errors",
		Author:     "agent-x",
		ReportedAt: "2026-06-19T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeReports([]string{"-corpus", corpus, "-queue", queue, "-base", base}, &buf); err != nil {
		t.Fatalf("runIntakeReports: %v", err)
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	for _, r := range recs {
		if r.ID == "exp-2769" && reportDisputes(r) == "exp-0043" {
			return
		}
	}
	t.Fatalf("intake-reports with -base must allocate exp-2769; records: %v", recordIDs(recs))
}

func recordIDs(recs []*record.Record) []string {
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestIntakeReports_RequiresQueueFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runIntakeReports([]string{"-corpus", "."}, &buf)
	if err == nil || !strings.Contains(err.Error(), "-queue") {
		t.Fatalf("intake-reports without -queue must fail clearly; got %v", err)
	}
}

// A malformed queue entry is logged + removed so it cannot wedge the nightly
// drain; a valid sibling still materializes.
func TestIntakeReports_SkipsMalformedEntry(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "queue")
	if err := os.MkdirAll(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	// A report against an invalid record id → BuildReport rejects it.
	if _, err := spool.Enqueue(queue, spool.Report{RecordID: "not-an-id", Outcome: "x", Author: "a", ReportedAt: "2026-06-19T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := spool.Enqueue(queue, spool.Report{RecordID: "exp-0043", Outcome: "failed", Author: "a", ReportedAt: "2026-06-19T12:00:02Z"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeReports([]string{"-corpus", corpus, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeReports: %v", err)
	}
	recs, _ := record.LoadCorpus(corpus)
	if len(recs) != 1 || reportDisputes(recs[0]) != "exp-0043" {
		t.Fatalf("only the valid report should materialize; got %d records", len(recs))
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("both entries (valid materialized, malformed removed) must drain; %d left", len(paths))
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Fatalf("the malformed entry should be reported as skipped; got %q", buf.String())
	}
}

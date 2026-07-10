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

// report queues a dispute into the intake spool without the server (#0044).
func TestReport_QueuesDispute(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "queue")
	var buf bytes.Buffer
	if err := runReport([]string{"-id", "exp-0043", "-queue", queue}, &buf); err != nil {
		t.Fatalf("runReport: %v", err)
	}
	paths, err := spool.List(queue)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued report, got %d", len(paths))
	}
	got, err := spool.Read(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.RecordID != "exp-0043" {
		t.Fatalf("RecordID = %q, want exp-0043", got.RecordID)
	}
	if got.Author != "daily-audit" {
		t.Fatalf("Author = %q, want daily-audit", got.Author)
	}
}

func TestReport_RequiresQueueFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runReport([]string{"-id", "exp-0043"}, &buf)
	if err == nil || !strings.Contains(err.Error(), "-queue") {
		t.Fatalf("report without -queue must fail clearly; got %v", err)
	}
}

func TestReport_RejectsInvalidID(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "queue")
	var buf bytes.Buffer
	err := runReport([]string{"-id", "nope", "-queue", queue}, &buf)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("invalid -id must fail mentioning the id; got %v", err)
	}
}

// End-to-end: a queued dispute materializes via intake-reports into a counter-record
// that adapt's reportDisputes predicate picks up.
func TestReport_IntakeMaterializesDispute(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "queue")

	var buf bytes.Buffer
	if err := runReport([]string{"-id", "exp-0043", "-queue", queue, "-evidence", "promotion was wrong"}, &buf); err != nil {
		t.Fatalf("runReport: %v", err)
	}
	if err := runIntakeReports([]string{"-corpus", corpus, "-queue", queue}, &buf, noEnv); err != nil {
		t.Fatalf("runIntakeReports: %v", err)
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 materialized counter-record, got %d", len(recs))
	}
	if reportDisputes(recs[0]) != "exp-0043" {
		t.Fatalf("counter-record must dispute exp-0043; got %q", reportDisputes(recs[0]))
	}
	if recs[0].Status != "quarantined" {
		t.Fatalf("counter-record must be quarantined, got %q", recs[0].Status)
	}
}

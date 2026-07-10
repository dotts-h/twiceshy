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

func TestIntakeRecords_MaterializesQueueIntoCorpus(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "queue")
	d1 := spool.RecordDraft{
		Kind:            "trap",
		Title:           "connection leak in db rows retry loop",
		Summary:         "connection stays open on retry",
		ErrorSignatures: []string{"connection-leak-err"},
		RootCause:       "missing defer rows.Close()",
		Fix:             "add defer rows.Close()",
		Body:            "body explanation",
		Author:          "agent-xyz",
		Session:         "sess-xyz",
		ReportedAt:      "2026-07-07T19:24:00Z",
	}
	d2 := spool.RecordDraft{
		Kind:            "trap",
		Title:           "snowflake another leak in rows retry",
		Summary:         "another connection leak issue",
		ErrorSignatures: []string{"another-leak-err"},
		RootCause:       "missing defer rows.Close()",
		Fix:             "add defer rows.Close()",
		Body:            "another body explanation",
		Author:          "agent-abc",
		Session:         "sess-abc",
		ReportedAt:      "2026-07-07T19:24:01Z",
	}

	for _, d := range []spool.RecordDraft{d1, d2} {
		if _, err := spool.EnqueueRecord(queue, d); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := runIntakeRecords([]string{"-corpus", corpus, "-queue", queue}, &buf, noEnv); err != nil {
		t.Fatalf("runIntakeRecords: %v", err)
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 materialized records, got %d", len(recs))
	}
	for _, r := range recs {
		if r.Status != "quarantined" {
			t.Fatalf("materialized record %s must be quarantined, got %q", r.ID, r.Status)
		}
	}
	if recs[0].ID == recs[1].ID {
		t.Fatalf("id collision across drafts: %s", recs[0].ID)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained after intake; %d left", len(paths))
	}
}

func TestIntakeRecords_DropsKnownDuplicateEntry(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}

	vDate := "2026-07-07"
	vTest := "TestConnectionLeak"
	rec := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0001",
		Kind:          "trap",
		Status:        "validated",
		Title:         "connection leak in db rows retry loop that is long enough",
		Symptom: &record.Symptom{
			Summary:         "connection stays open on retry",
			ErrorSignatures: []string{"connection-leak-err"},
		},
		Guard: &record.Guard{
			GuardingTest: &vTest,
		},
		Resolution: &record.Resolution{
			RootCause: "missing defer rows.Close()",
			Fix:       "add defer rows.Close()",
		},
		Body: "body explanation",
		Path: "experience/2026/0001-leak.md",
		Provenance: record.Provenance{
			Source:      record.Source{Author: "agent-xyz"},
			RecordedAt:  "2026-07-07",
			Valid:       record.Validity{From: "2026-07-07"},
			ValidatedAt: &vDate,
		},
	}
	writeFixture(t, corpus, rec)

	queue := filepath.Join(t.TempDir(), "queue")
	d1 := spool.RecordDraft{
		Kind:            "trap",
		Title:           "connection leak in db rows retry loop that is long enough",
		Summary:         "connection stays open on retry",
		ErrorSignatures: []string{"connection-leak-err"},
		RootCause:       "missing defer rows.Close()",
		Fix:             "add defer rows.Close()",
		Body:            "body explanation",
		Author:          "agent-xyz",
		Session:         "sess-xyz",
		ReportedAt:      "2026-07-07T19:24:00Z",
	}
	if _, err := spool.EnqueueRecord(queue, d1); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeRecords([]string{"-corpus", corpus, "-queue", queue}, &buf, noEnv); err != nil {
		t.Fatalf("runIntakeRecords: %v", err)
	}

	// Corpus should still only have the seeded record.
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record in corpus, got %d", len(recs))
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained after duplicate detection; %d left", len(paths))
	}
}

func TestIntakeRecords_RequiresQueueFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runIntakeRecords([]string{"-corpus", "."}, &buf, noEnv)
	if err == nil || !strings.Contains(err.Error(), "-queue") {
		t.Fatalf("intake-records without -queue must fail clearly; got %v", err)
	}
}

func TestIntakeRecords_SkipsMalformedEntry(t *testing.T) {
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "queue")
	if err := os.MkdirAll(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a malformed JSON file into the queue
	if err := os.WriteFile(filepath.Join(queue, "2026-07-07T19-24-00Z-bad.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeRecords([]string{"-corpus", corpus, "-queue", queue}, &buf, noEnv); err != nil {
		t.Fatalf("runIntakeRecords: %v", err)
	}

	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained after skipping malformed entries; %d left", len(paths))
	}
}

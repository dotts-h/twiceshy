// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/retro"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// retroTestCorpus builds a writable empty corpus and an index over it, so drainRetro
// can dedup and writeRecord can persist.
func retroTestCorpus(t *testing.T) (corpus string, ix *index.Index) {
	t.Helper()
	corpus = t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	ix, err = index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, ""); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return corpus, ix
}

func aTrapCandidate() retro.Candidate {
	return retro.Candidate{
		Kind:            "trap",
		Title:           "fts5 MATCH treats raw input as query syntax",
		Summary:         "a dotted token throws fts5: syntax error",
		ErrorSignatures: []string{"fts5: syntax error"},
		Ecosystem:       "Go",
		Package:         "modernc.org/sqlite",
		RootCause:       "raw query is parsed as FTS5 query syntax",
		Fix:             "quote the user query before MATCH",
		Body:            "Quote user input before passing it to a MATCH clause.",
	}
}

func spoolOne(t *testing.T, queue, transcript string) {
	t.Helper()
	if _, err := spool.EnqueueTranscript(queue, spool.Transcript{
		SessionID:  "sess-1",
		Author:     "claude",
		Transcript: transcript,
		CapturedAt: "2026-06-22T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
}

// The headline acceptance for #0065: a spooled transcript becomes a quarantined
// draft on disk without any agent calling record_experience, and the queue drains.
func TestDrainRetro_MaterializesQuarantinedDraftAndDrains(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit fts5: syntax error and recovered")
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{aTrapCandidate()}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if analyzer.Calls != 1 {
		t.Errorf("analyzer Calls = %d, want 1", analyzer.Calls)
	}
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 quarantined draft, got %d", len(recs))
	}
	if recs[0].Status != "quarantined" {
		t.Errorf("draft status = %q, want quarantined (the analyzer drafts, never promotes)", recs[0].Status)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("queue must drain after a successful analysis; %d left", len(paths))
	}
}

// Fail-safe: an analyzer outage leaves the transcript queued for retry and writes
// nothing — a down endpoint must never read as "no traps".
func TestDrainRetro_AnalyzerErrorLeavesQueuedAndWritesNothing(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit something")
	analyzer := &retro.StubAnalyzer{Err: errors.New("endpoint down")}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, &buf); err == nil {
		t.Fatal("want an error when the analyzer is down (fail-safe), got nil")
	}
	if paths, _ := spool.List(queue); len(paths) != 1 {
		t.Errorf("a transcript must stay queued on analyzer error; %d left", len(paths))
	}
	if recs, _ := record.LoadCorpus(corpus); len(recs) != 0 {
		t.Errorf("nothing must be written on analyzer error; got %d", len(recs))
	}
}

func TestDrainRetro_DedupsRepeatedCandidateWithinRun(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit fts5 twice")
	c := aTrapCandidate()
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{c, c}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if recs, _ := record.LoadCorpus(corpus); len(recs) != 1 {
		t.Errorf("a repeated candidate must dedup to 1 draft; got %d", len(recs))
	}
}

func TestDrainRetro_DryRunWritesNothingAndKeepsQueue(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit fts5")
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{aTrapCandidate()}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22", dryRun: true}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if recs, _ := record.LoadCorpus(corpus); len(recs) != 0 {
		t.Errorf("dry-run must write nothing; got %d records", len(recs))
	}
	if paths, _ := spool.List(queue); len(paths) != 1 {
		t.Errorf("dry-run must dequeue nothing; %d left", len(paths))
	}
	if !strings.Contains(buf.String(), "would create") {
		t.Errorf("dry-run should report what it would create; got %q", buf.String())
	}
}

func TestRunRetroIntake_RequiresQueue(t *testing.T) {
	var buf bytes.Buffer
	err := runRetroIntake(context.Background(), []string{"-corpus", "."}, &buf, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "-queue") {
		t.Fatalf("retro-intake without -queue must fail clearly; got %v", err)
	}
}

func TestRunRetroIntake_RequiresAnalyzerEndpoint(t *testing.T) {
	var buf bytes.Buffer
	err := runRetroIntake(context.Background(), []string{"-corpus", t.TempDir(), "-queue", t.TempDir()}, &buf,
		func(string) string { return "" }) // no TWICESHY_RETRO_URL / TWICESHY_JUDGE_URL
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_RETRO_URL") {
		t.Fatalf("retro-intake without an analyzer endpoint must fail clearly; got %v", err)
	}
}

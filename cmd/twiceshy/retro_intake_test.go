// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/notify"
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
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, nil, notify.NopAlerter{}, &buf); err != nil {
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

func TestRunRetroIntake_BaseAllocatesPastBaseMax(t *testing.T) {
	corpus, base := corpusWithLocal2758AndBase2768(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit fts5: syntax error and recovered")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"kind":"trap","title":"fts5 MATCH treats raw input as query syntax","summary":"a dotted token throws fts5: syntax error","error_signatures":["fts5: syntax error"],"ecosystem":"Go","package":"modernc.org/sqlite","root_cause":"raw query is parsed as FTS5 query syntax","fix":"quote the user query before MATCH","body":"Quote user input before passing it to a MATCH clause."}]}`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	err := runRetroIntake(context.Background(), []string{"-queue", queue, "-corpus", corpus,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-base", base}, &buf, func(k string) string {
		switch k {
		case "TWICESHY_RETRO_URL":
			return srv.URL
		case "TWICESHY_RETRO_MODEL":
			return "test-model"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("retro-intake: %v", err)
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	for _, r := range recs {
		if r.ID == "exp-2769" && strings.Contains(r.Title, "fts5 MATCH") {
			return
		}
	}
	t.Fatalf("retro-intake with -base must allocate exp-2769; records: %v", recordIDs(recs))
}

// Fail-safe: an analyzer outage leaves the transcript queued for retry and writes
// nothing — a down endpoint must never read as "no traps".
func TestDrainRetro_AnalyzerErrorLeavesQueuedAndWritesNothing(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "agent hit something")
	analyzer := &retro.StubAnalyzer{Err: errors.New("endpoint down")}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, nil, notify.NopAlerter{}, &buf); err == nil {
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
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22"}, nil, notify.NopAlerter{}, &buf); err != nil {
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
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22", dryRun: true}, nil, notify.NopAlerter{}, &buf); err != nil {
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

// Regression: a -limit hit must still dequeue the fully-processed transcript.
// Signature-less candidates (conventions, or traps with no error_signatures) never
// fingerprint, so they come back Similar — not Known — on a re-analysis. If a
// limit-break left the transcript queued, the next drain would re-extract and
// re-write them as duplicate drafts. The fix enforces the limit at the transcript
// boundary: the transcript is processed whole and removed.
func TestDrainRetro_LimitHitStillDequeuesProcessedTranscript(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "a session that hit two distinct conventions")
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{
		{Kind: "convention", Title: "always quote fts5 MATCH input tokens", Body: "Quote every token before MATCH."},
		{Kind: "convention", Title: "register mux routes unqualified for clean 405s", Body: "Check the method in-handler."},
	}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-22", limit: 1}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("a limit-hit must still dequeue the fully-processed transcript; %d left (a re-run would re-write signature-less drafts as duplicates)", len(paths))
	}
}

// unprocessableAfterFirst is a local stub that returns ErrUnprocessable on the
// first call and real candidates on every subsequent call.
type unprocessableAfterFirst struct {
	candidates []retro.Candidate
	calls      int
}

func (u *unprocessableAfterFirst) Analyze(_ context.Context, _ string) ([]retro.Candidate, error) {
	u.calls++
	if u.calls == 1 {
		return nil, fmt.Errorf("model returned garbage: %w", retro.ErrUnprocessable)
	}
	return u.candidates, nil
}

// A transient ErrUnprocessable (model flakiness) must be retried and recovered,
// not dead-lettered on the first failure.
func TestDrainRetro_TransientUnprocessableIsRetriedNotDeadLettered(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")

	// Enqueue two transcripts. The first fails once then succeeds; the second succeeds.
	spoolOne(t, queue, "transcript that confuses the model")
	spoolOne(t, queue, "agent hit fts5: syntax error and recovered")

	stub := &unprocessableAfterFirst{candidates: []retro.Candidate{aTrapCandidate()}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), stub, ix, "", corpus, queue, retroOpts{now: "2026-06-26"}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro returned error (must continue past transient unprocessable): %v", err)
	}

	remaining, _ := spool.List(queue)
	if len(remaining) != 0 {
		t.Errorf("queue must be empty after processing both entries; %d left", len(remaining))
	}

	dead := filepath.Join(queue, "dead")
	deadFiles, _ := spool.List(dead)
	if len(deadFiles) != 0 {
		t.Errorf("transient failure must not dead-letter; dead/ has %d file(s)", len(deadFiles))
	}

	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 quarantined draft (both transcripts recovered; same candidate deduped), got %d", len(recs))
	}
	if !strings.Contains(buf.String(), "1 known/duplicate") {
		t.Errorf("second transcript must dedup against first; got: %q", buf.String())
	}

	if stub.calls < 2 {
		t.Errorf("transient failure must retry Analyze; calls = %d, want >= 2", stub.calls)
	}
}

type alwaysUnprocessable struct{ calls int }

func (a *alwaysUnprocessable) Analyze(_ context.Context, _ string) ([]retro.Candidate, error) {
	a.calls++
	return nil, fmt.Errorf("model keeps returning garbage: %w", retro.ErrUnprocessable)
}

// drainRetro must dead-letter a persistently unprocessable entry after bounded
// retries (poison pill), not retry forever.
func TestDrainRetro_PoisonPillDeadLetteredAfterBoundedRetries(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	spoolOne(t, queue, "transcript that always confuses the model")

	stub := &alwaysUnprocessable{}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), stub, ix, "", corpus, queue, retroOpts{now: "2026-06-26"}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro returned error (must dead-letter poison pill): %v", err)
	}

	remaining, _ := spool.List(queue)
	if len(remaining) != 0 {
		t.Errorf("queue must be empty after dead-lettering poison pill; %d left", len(remaining))
	}

	dead := filepath.Join(queue, "dead")
	deadFiles, _ := spool.List(dead)
	if len(deadFiles) != 1 {
		t.Errorf("dead-letter dir must hold exactly 1 file; got %d", len(deadFiles))
	}

	if stub.calls != analyzeAttempts {
		t.Errorf("poison pill must retry exactly analyzeAttempts times; calls = %d, want %d", stub.calls, analyzeAttempts)
	}

	if !strings.Contains(buf.String(), "skip") || !strings.Contains(buf.String(), "unprocessable") {
		t.Errorf("output must report the skip; got: %q", buf.String())
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

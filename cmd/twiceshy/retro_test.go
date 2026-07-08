// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/notify"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/retro"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// recordingAlerter captures alert events for test assertions.
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

// countingConfirmHelpfuler records ids passed to ConfirmHelpful for test assertions.
type countingConfirmHelpfuler struct {
	ids []string
}

func (c *countingConfirmHelpfuler) ConfirmHelpful(_ context.Context, id string) error {
	c.ids = append(c.ids, id)
	return nil
}

func enqueueRetroTranscript(t *testing.T, queue, sessionID, transcript string) {
	t.Helper()
	if _, err := spool.EnqueueTranscript(queue, spool.Transcript{
		SessionID:  sessionID,
		Author:     "claude",
		Transcript: transcript,
		CapturedAt: "2026-06-28T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
}

func enqueueRetroTranscriptWithSource(t *testing.T, queue, sessionID, transcript, sourceURL, sourceLicense string) {
	t.Helper()
	if _, err := spool.EnqueueTranscript(queue, spool.Transcript{
		SessionID:     sessionID,
		Author:        "trap-miner",
		Transcript:    transcript,
		CapturedAt:    "2026-06-28T10:00:00Z",
		SourceURL:     sourceURL,
		SourceLicense: sourceLicense,
	}); err != nil {
		t.Fatal(err)
	}
}

// External trap miners (#0133) stamp upstream commit URL + SPDX license on every
// draft mined from a retro-queue entry carrying source_url/source_license.
func TestDrainRetro_PropagatesSourceProvenance(t *testing.T) {
	const (
		wantURL     = "https://github.com/example/repo/commit/abc123"
		wantLicense = "MIT"
	)
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscriptWithSource(t, queue, "s1", "upstream issue thread", wantURL, wantLicense)
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{aTrapCandidate()}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 quarantined draft, got %d", len(recs))
	}
	if recs[0].Provenance.SourceURL != wantURL {
		t.Errorf("Provenance.SourceURL = %q, want %q", recs[0].Provenance.SourceURL, wantURL)
	}
	if recs[0].Provenance.SourceLicense != wantLicense {
		t.Errorf("Provenance.SourceLicense = %q, want %q", recs[0].Provenance.SourceLicense, wantLicense)
	}
}

// Interactive session captures omit source_url/source_license; provenance stays empty.
func TestDrainRetro_BackwardCompat_NoSourceProvenance(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent session")
	analyzer := &retro.StubAnalyzer{Candidates: []retro.Candidate{aTrapCandidate()}}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), analyzer, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 quarantined draft, got %d", len(recs))
	}
	if recs[0].Provenance.SourceURL != "" {
		t.Errorf("Provenance.SourceURL = %q, want empty", recs[0].Provenance.SourceURL)
	}
	if recs[0].Provenance.SourceLicense != "" {
		t.Errorf("Provenance.SourceLicense = %q, want empty", recs[0].Provenance.SourceLicense)
	}
}

// The served-vs-used join confirms ONLY cards that were both Used (judge) and served
// (telemetry decision log) — exp-0002 is used but not served, so it must not confirm.
func TestDrainRetro_HelpfulnessJoin_ServedFilter(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent applied exp-0001 lesson")

	judge := &retro.StubUsageJudge{Verdicts: []retro.CardVerdict{
		{ID: "exp-0001", Used: true},
		{ID: "exp-0002", Used: true},
	}}
	rec := &countingConfirmHelpfuler{}
	join := &helpfulJoin{
		judge: judge,
		rec:   rec,
		servedFor: func(sid string) (map[string]bool, error) {
			if sid != "s1" {
				t.Errorf("servedFor session = %q, want s1", sid)
			}
			return map[string]bool{"exp-0001": true}, nil
		},
	}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, join, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if len(rec.ids) != 1 || rec.ids[0] != "exp-0001" {
		t.Fatalf("confirmed ids = %v, want [exp-0001] (exp-0002 used but not served)", rec.ids)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("transcript must dequeue after successful drain; %d left", len(paths))
	}
}

// A flaky usage judge must not block the primary trap drain — errors are logged, not propagated.
func TestDrainRetro_HelpfulnessJoin_BestEffortOnJudgeError(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent session")

	rec := &countingConfirmHelpfuler{}
	join := &helpfulJoin{
		judge: &retro.StubUsageJudge{Err: errors.New("judge down")},
		rec:   rec,
		servedFor: func(string) (map[string]bool, error) {
			return map[string]bool{"exp-0001": true}, nil
		},
	}

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, join, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro must complete despite judge error: %v", err)
	}
	if len(rec.ids) != 0 {
		t.Fatalf("judge error must confirm nothing; got %v", rec.ids)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("transcript must still dequeue; %d left", len(paths))
	}
}

// join == nil disables the helpfulness path — today's behavior, no panic.
func TestDrainRetro_HelpfulnessJoin_Disabled(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent session")

	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, notify.NopAlerter{}, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("transcript must dequeue; %d left", len(paths))
	}
}

func TestDrainRetro_ChronicFailureRate_Alerts(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	for i := 0; i < 6; i++ {
		enqueueRetroTranscript(t, queue, fmt.Sprintf("s%d", i), "agent session")
	}
	al := &recordingAlerter{}
	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{Err: retro.ErrUnprocessable}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, al, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if !al.has("retro-analyzer-unreliable") {
		t.Fatal("expected retro-analyzer-unreliable alert on high failure rate")
	}
}

func TestDrainRetro_ChronicFailureRate_NoAlertOnSuccess(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent session")
	al := &recordingAlerter{}
	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, al, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if al.has("retro-analyzer-unreliable") {
		t.Fatal("must not alert when analyzer succeeds")
	}
}

func TestDrainRetro_ChronicFailureRate_NoAlertBelowMinSample(t *testing.T) {
	corpus, ix := retroTestCorpus(t)
	queue := filepath.Join(t.TempDir(), "retro")
	enqueueRetroTranscript(t, queue, "s1", "agent session")
	al := &recordingAlerter{}
	var buf bytes.Buffer
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{Err: retro.ErrUnprocessable}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, al, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if al.has("retro-analyzer-unreliable") {
		t.Fatal("must not alert below unprocessableMinSample even at 100% failure rate")
	}
}

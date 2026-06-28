// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/retro"
	"github.com/dotts-h/twiceshy/internal/spool"
)

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
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, join, &buf); err != nil {
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
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, join, &buf); err != nil {
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
	if err := drainRetro(context.Background(), &retro.StubAnalyzer{}, ix, "", corpus, queue, retroOpts{now: "2026-06-28"}, nil, &buf); err != nil {
		t.Fatalf("drainRetro: %v", err)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("transcript must dequeue; %d left", len(paths))
	}
}

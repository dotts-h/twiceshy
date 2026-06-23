// SPDX-License-Identifier: AGPL-3.0-only

package retro_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/retro"
)

// fakeRecorder captures the ids confirmed-helpful, and can be primed to fail on one.
type fakeRecorder struct {
	ids    []string
	failOn string
}

func (f *fakeRecorder) ConfirmHelpful(_ context.Context, id string) error {
	if id == f.failOn {
		return errors.New("boom")
	}
	f.ids = append(f.ids, id)
	return nil
}

// RecordHelpfulness must confirm ONLY the cards the judge marked Used — an ignored
// served card is an absent positive, never counter-evidence (#0069).
func TestRecordHelpfulness_RecordsOnlyUsed(t *testing.T) {
	rec := &fakeRecorder{}
	verdicts := []retro.CardVerdict{
		{ID: "exp-0001", Used: true},
		{ID: "exp-0002", Used: false},
		{ID: "exp-0003", Used: true},
	}
	n, err := retro.RecordHelpfulness(context.Background(), rec, verdicts)
	if err != nil {
		t.Fatalf("RecordHelpfulness: %v", err)
	}
	if n != 2 {
		t.Fatalf("recorded = %d, want 2 (only the Used cards)", n)
	}
	got := rec.ids
	if len(got) != 2 || got[0] != "exp-0001" || got[1] != "exp-0003" {
		t.Fatalf("confirmed ids = %v, want [exp-0001 exp-0003]", got)
	}
}

// Trust boundary: verdict ids come from a model over an UNTRUSTED transcript, so a
// malformed/garbage/injection-shaped id (empty, whitespace, non-exp, path-shaped) must
// be dropped before it touches the usage table — the same record.ValidID firewall the
// human confirm_helpful path applies. Only the well-formed exp-NNNN id is confirmed.
func TestRecordHelpfulness_SkipsInvalidID(t *testing.T) {
	rec := &fakeRecorder{}
	n, err := retro.RecordHelpfulness(context.Background(), rec, []retro.CardVerdict{
		{ID: "", Used: true},
		{ID: "   ", Used: true},
		{ID: "exp-", Used: true},
		{ID: "not-an-id", Used: true},
		{ID: "../exp-0001", Used: true},
		{ID: "exp-0009", Used: true}, // the only valid id
	})
	if err != nil {
		t.Fatalf("RecordHelpfulness: %v", err)
	}
	if n != 1 || len(rec.ids) != 1 || rec.ids[0] != "exp-0009" {
		t.Fatalf("only the valid id confirms; recorded=%d ids=%v, want 1 [exp-0009]", n, rec.ids)
	}
}

// A served card the judge marks Used more than once in a single session confirms
// exactly once — within-session dedup keeps one session to one reinforcement per card.
func TestRecordHelpfulness_DedupsWithinSession(t *testing.T) {
	rec := &fakeRecorder{}
	n, err := retro.RecordHelpfulness(context.Background(), rec, []retro.CardVerdict{
		{ID: "exp-0007", Used: true},
		{ID: "exp-0007", Used: true},
		{ID: "exp-0007", Used: true},
	})
	if err != nil {
		t.Fatalf("RecordHelpfulness: %v", err)
	}
	if n != 1 || len(rec.ids) != 1 || rec.ids[0] != "exp-0007" {
		t.Fatalf("a repeated Used card confirms once; recorded=%d ids=%v, want 1 [exp-0007]", n, rec.ids)
	}
}

// On the first recorder error RecordHelpfulness stops and returns the error with the
// count recorded so far, so the caller can leave the transcript for retry.
func TestRecordHelpfulness_StopsOnError(t *testing.T) {
	rec := &fakeRecorder{failOn: "exp-0002"}
	n, err := retro.RecordHelpfulness(context.Background(), rec, []retro.CardVerdict{
		{ID: "exp-0001", Used: true},
		{ID: "exp-0002", Used: true}, // fails here
		{ID: "exp-0003", Used: true}, // never reached
	})
	if err == nil {
		t.Fatal("a recorder error must propagate")
	}
	if n != 1 {
		t.Fatalf("recorded = %d, want 1 (the one before the error)", n)
	}
	if len(rec.ids) != 1 || rec.ids[0] != "exp-0001" {
		t.Fatalf("confirmed ids = %v, want [exp-0001] (stopped at the error)", rec.ids)
	}
}

// An empty verdict list (a session that applied nothing) records nothing, no error.
func TestRecordHelpfulness_Empty(t *testing.T) {
	rec := &fakeRecorder{}
	n, err := retro.RecordHelpfulness(context.Background(), rec, nil)
	if err != nil || n != 0 || len(rec.ids) != 0 {
		t.Fatalf("empty verdicts must be a no-op; n=%d err=%v ids=%v", n, err, rec.ids)
	}
}

// StubUsageJudge returns its primed verdicts (or error) and records the call.
func TestStubUsageJudge(t *testing.T) {
	want := []retro.CardVerdict{{ID: "exp-0005", Used: true}}
	s := &retro.StubUsageJudge{Verdicts: want}
	got, err := s.JudgeUsage(context.Background(), "a transcript")
	if err != nil {
		t.Fatalf("JudgeUsage: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
	if s.Calls != 1 || s.Last != "a transcript" {
		t.Fatalf("stub did not record the call: calls=%d last=%q", s.Calls, s.Last)
	}

	s.Err = errors.New("endpoint down")
	if _, err := s.JudgeUsage(context.Background(), "x"); err == nil {
		t.Fatal("a primed error must propagate")
	}
}

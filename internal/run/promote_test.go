// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// fakePromoter is a stub recordPromoter: it promotes the ids in `promote`,
// errors for ids in `err`, and otherwise holds. On a promotion it mutates the
// record like the real promoter so persist sees the flip.
type fakePromoter struct {
	promote map[string]bool
	err     map[string]error
	calls   []string
}

func (f *fakePromoter) Promote(_ context.Context, rec *record.Record) (promote.Outcome, error) {
	f.calls = append(f.calls, rec.ID)
	if e, ok := f.err[rec.ID]; ok {
		return promote.Outcome{}, e
	}
	if f.promote[rec.ID] {
		rec.Status = "validated"
		return promote.Outcome{
			Promoted:    true,
			Verdict:     judge.Verdict{Model: "gemini-2.5-pro", Decision: judge.Approve},
			Attestation: repro.Attestation{ReproducedUnder: []string{"go1.25"}},
		}, nil
	}
	return promote.Outcome{Reason: "judge did not approve"}, nil
}

func eligibleRec(id string) *record.Record {
	rp := "experience/repro/" + id + ".sh"
	return &record.Record{
		ID: id, Status: "quarantined",
		Guard: &record.Guard{Repro: &rp},
		// Resolution with a substantive root_cause is required by the
		// promote root-cause pre-gate (#0094): records without one are held.
		Resolution: &record.Resolution{RootCause: "stub root cause for corpus-level orchestration test"},
		Path:       "experience/2026/" + id[len("exp-"):] + "-x.md",
	}
}

func TestPromoteCorpus_PromotesEligiblePersistsFlip(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"),               // promoted (repro-eligible)
		eligibleRec("exp-0101"),               // held (repro-eligible, not promoted)
		{ID: "exp-0102", Status: "validated"}, // ineligible (not quarantined)
		// prose-class (no repro, no vuln id) → held, ADR-0020; root cause required by #0094.
		{ID: "exp-0103", Status: "quarantined", Resolution: &record.Resolution{RootCause: "stub root cause"}},
	}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	var persisted []string
	persist := func(_ string, rec *record.Record) error {
		persisted = append(persisted, rec.ID)
		return nil
	}
	var buf bytes.Buffer

	st, _, err := PromoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("PromoteCorpus: %v", err)
	}
	if st.Promoted != 1 || st.Held != 2 || st.Ineligible != 1 {
		t.Fatalf("stats = %+v, want promoted 1 / held 2 / ineligible 1 (exp-0103 is now prose-class, ADR-0020)", st)
	}
	if len(persisted) != 1 || persisted[0] != "exp-0100" {
		t.Fatalf("persisted = %v, want [exp-0100] (only the promoted record is written)", persisted)
	}
	// Ineligible records (exp-0102, not quarantined) never reach the promoter (cost
	// guard); the two repro-eligible records and the prose-class exp-0103 do.
	if strings.Join(fp.calls, ",") != "exp-0100,exp-0101,exp-0103" {
		t.Fatalf("promoter calls = %v, want the repro-eligible records + the prose-class record", fp.calls)
	}
}

func TestPromoteCorpus_AbortsOnPromoterError(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{
		promote: map[string]bool{"exp-0100": true},
		err:     map[string]error{"exp-0101": errors.New("broker exploded")},
	}
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }

	_, _, err := PromoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err == nil {
		t.Fatal("a promoter error must abort the run")
	}
	// The record promoted before the error stays written (independently valid).
	if len(persisted) != 1 || persisted[0] != "exp-0100" {
		t.Fatalf("persisted = %v, want [exp-0100]", persisted)
	}
}

func TestPromoteCorpus_JournalAbortsThenResumes(t *testing.T) {
	d := t.TempDir()
	jp := promote.JournalPath(d, "promote")
	recs := []*record.Record{
		eligibleRec("exp-0100"),
		eligibleRec("exp-0101"),
		eligibleRec("exp-0102"),
	}
	var persisted []string
	persist := func(_ string, rec *record.Record) error {
		persisted = append(persisted, rec.ID)
		return nil
	}
	var buf bytes.Buffer

	fp1 := &fakePromoter{
		promote: map[string]bool{"exp-0100": true, "exp-0101": true},
		err:     map[string]error{"exp-0102": errors.New("broker exploded")},
	}
	_, _, err := PromoteCorpus(context.Background(), d, recs, fp1, persist, guard.Guardrails{}, nil, nil, &buf, jp)
	if err == nil {
		t.Fatal("first run must abort on promoter error")
	}
	j, lerr := promote.LoadJournal(jp)
	if lerr != nil {
		t.Fatalf("LoadJournal: %v", lerr)
	}
	if j == nil {
		t.Fatal("journal file must exist after abort")
	}
	if j.Complete {
		t.Fatal("aborted journal must not be complete")
	}
	if j.StoppedAt == nil || j.StoppedAt.RecordID != "exp-0102" {
		t.Fatalf("StoppedAt = %+v, want record exp-0102", j.StoppedAt)
	}
	done := j.DoneIDs()
	if !done["exp-0100"] || !done["exp-0101"] || done["exp-0102"] {
		t.Fatalf("DoneIDs after abort = %v, want A and B only", done)
	}

	fp2 := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	persisted = nil
	buf.Reset()
	_, _, err = PromoteCorpus(context.Background(), d, recs, fp2, persist, guard.Guardrails{}, nil, nil, &buf, jp)
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if strings.Join(fp2.calls, ",") != "exp-0102" {
		t.Fatalf("resume must skip decided records; promoter calls = %v", fp2.calls)
	}
	if len(persisted) != 1 || persisted[0] != "exp-0102" {
		t.Fatalf("persisted on resume = %v, want [exp-0102]", persisted)
	}
	j, lerr = promote.LoadJournal(jp)
	if lerr != nil {
		t.Fatalf("LoadJournal after resume: %v", lerr)
	}
	if !j.Complete {
		t.Fatal("completed resume run must mark journal complete")
	}
}

func TestPromoteCorpus_NoJournalWhenPathEmpty(t *testing.T) {
	d := t.TempDir()
	jp := promote.JournalPath(d, "promote")
	recs := []*record.Record{eligibleRec("exp-0100")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	persist := func(_ string, _ *record.Record) error { return nil }

	_, _, err := PromoteCorpus(context.Background(), d, recs, fp, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err != nil {
		t.Fatalf("PromoteCorpus: %v", err)
	}
	if _, statErr := os.Stat(jp); !os.IsNotExist(statErr) {
		t.Fatalf("empty journalPath must write no file; stat err = %v", statErr)
	}
}

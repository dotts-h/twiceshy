// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// A COMPLETE journal must be ignored — the next run does a fresh walk, not a
// resume. Without this, a finished run's journal could suppress all future work
// (every record skipped as "already done"). #0054.
func TestPromoteCorpus_CompleteJournalIgnored(t *testing.T) {
	dir := t.TempDir()
	jp := promote.JournalPath(dir, "promote")
	// Pre-seed a COMPLETE journal that "already decided" exp-0100.
	seed := &promote.Journal{
		Stage: "promote", Complete: true,
		Actions: []promote.RecordAction{{ID: "exp-0100", Outcome: "promoted"}},
	}
	if err := seed.Save(jp); err != nil {
		t.Fatal(err)
	}

	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true}}
	persist := func(string, *record.Record) error { return nil }

	_, _, err := PromoteCorpus(context.Background(), dir, recs, fp, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, jp)
	if err != nil {
		t.Fatalf("PromoteCorpus: %v", err)
	}
	// Both records must be (re)processed — a complete journal does not skip them.
	if len(fp.calls) != 2 {
		t.Fatalf("a complete journal must be ignored; promoter calls = %v, want both records", fp.calls)
	}
}

// On resume, the final journal must carry the prior (pre-abort) actions PLUS the
// new ones — the journal is the full record of the resumed run, not just its tail.
func TestPromoteCorpus_ResumePreservesPriorActions(t *testing.T) {
	dir := t.TempDir()
	jp := promote.JournalPath(dir, "promote")
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102")}
	persist := func(string, *record.Record) error { return nil }

	// Run 1: promote 0100/0101, abort on 0102.
	fp1 := &fakePromoter{
		promote: map[string]bool{"exp-0100": true, "exp-0101": true},
		err:     map[string]error{"exp-0102": errBoom()},
	}
	if _, _, err := PromoteCorpus(context.Background(), dir, recs, fp1, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, jp); err == nil {
		t.Fatal("run 1 must abort")
	}

	// Run 2: resume — 0102 now succeeds.
	fp2 := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	if _, _, err := PromoteCorpus(context.Background(), dir, recs, fp2, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, jp); err != nil {
		t.Fatalf("resume: %v", err)
	}

	j, err := promote.LoadJournal(jp)
	if err != nil || j == nil {
		t.Fatalf("LoadJournal after resume: %v", err)
	}
	done := j.DoneIDs()
	for _, id := range []string{"exp-0100", "exp-0101", "exp-0102"} {
		if !done[id] {
			t.Errorf("final journal missing prior/new action for %s; DoneIDs=%v", id, done)
		}
	}
}

func errBoom() error { return &boomErr{} }

type boomErr struct{}

func (*boomErr) Error() string { return "broker exploded" }

func TestJournalPathForRun(t *testing.T) {
	if got := JournalPathForRun("/corpus", "promote", true /*effect*/); got != "" {
		t.Errorf("an -effect dry-run must disable journaling; got %q", got)
	}
	want := filepath.Join("/corpus", "runs", "promote.journal.json")
	if got := JournalPathForRun("/corpus", "promote", false); got != want {
		t.Errorf("a real run must journal to %q; got %q", want, got)
	}
}

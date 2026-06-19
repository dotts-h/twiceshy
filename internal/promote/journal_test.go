// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/promote"
)

func TestJournal_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs", "promote.journal.json")

	j := &promote.Journal{
		RunID:    "run-1",
		Stage:    "promote",
		Complete: false,
		StoppedAt: &promote.JournalStop{
			RecordID: "exp-0102",
			Error:    "broker exploded",
		},
		Actions: []promote.RecordAction{
			{ID: "exp-0100", Outcome: "promoted", FromStatus: "quarantined", ToStatus: "validated"},
			{ID: "exp-0101", Outcome: "held", FromStatus: "quarantined", ToStatus: "quarantined"},
		},
	}
	if err := j.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := promote.LoadJournal(path)
	if err != nil {
		t.Fatalf("LoadJournal: %v", err)
	}
	if got == nil {
		t.Fatal("LoadJournal returned nil journal")
	}
	if got.RunID != j.RunID || got.Stage != j.Stage || got.Complete != j.Complete {
		t.Fatalf("metadata mismatch: got %+v want %+v", got, j)
	}
	if got.StoppedAt == nil || got.StoppedAt.RecordID != "exp-0102" || got.StoppedAt.Error != "broker exploded" {
		t.Fatalf("StoppedAt mismatch: %+v", got.StoppedAt)
	}
	if len(got.Actions) != 2 || got.Actions[0].ID != "exp-0100" || got.Actions[1].Outcome != "held" {
		t.Fatalf("Actions mismatch: %+v", got.Actions)
	}
}

func TestLoadJournal_MissingReturnsNil(t *testing.T) {
	got, err := promote.LoadJournal(filepath.Join(t.TempDir(), "nope.journal.json"))
	if err != nil {
		t.Fatalf("LoadJournal: %v", err)
	}
	if got != nil {
		t.Fatalf("missing journal must be (nil, nil); got %+v", got)
	}
}

func TestJournal_DoneIDs(t *testing.T) {
	j := &promote.Journal{
		Actions: []promote.RecordAction{
			{ID: "exp-0100"},
			{ID: "exp-0101"},
		},
	}
	done := j.DoneIDs()
	if !done["exp-0100"] || !done["exp-0101"] || done["exp-0102"] {
		t.Fatalf("DoneIDs = %v, want exp-0100 and exp-0101 only", done)
	}
}

func TestJournalPath(t *testing.T) {
	got := promote.JournalPath("/corpus", "promote")
	want := filepath.Join("/corpus", "runs", "promote.journal.json")
	if got != want {
		t.Fatalf("JournalPath = %q, want %q", got, want)
	}
}

func TestJournal_SaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs", "adapt.journal.json")
	j := &promote.Journal{Stage: "adapt", Actions: []promote.RecordAction{}}
	if err := j.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("journal file not created: %v", err)
	}
}

// A corrupt/unparseable journal must surface as an error, not a silent nil —
// otherwise a half-written or hand-mangled journal would be treated as "no
// journal" and a mid-run abort would re-walk from scratch instead of resuming.
func TestLoadJournal_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "promote.journal.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := promote.LoadJournal(path)
	if err == nil {
		t.Fatal("a corrupt journal must error, not be silently treated as absent")
	}
	if j != nil {
		t.Fatalf("a corrupt journal must not return a partial journal; got %+v", j)
	}
}

// A genuinely-absent journal is the one case that is NOT an error (fresh run).
func TestLoadJournal_AbsentIsNotAnError(t *testing.T) {
	j, err := promote.LoadJournal(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("an absent journal must be (nil,nil), not an error: %v", err)
	}
	if j != nil {
		t.Fatalf("an absent journal must return nil; got %+v", j)
	}
}

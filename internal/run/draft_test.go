// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/record"
)

// fakeRunner is a stub pipelineRunner: it records the ids it was asked to run
// and returns a canned outcome per record id. An entry in err means "return that
// error" so the abort path is exercisable.
type fakeRunner struct {
	out   map[string]drafter.Outcome
	err   map[string]error
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, rec *record.Record) (drafter.Outcome, error) {
	f.calls = append(f.calls, rec.ID)
	if e, ok := f.err[rec.ID]; ok {
		return drafter.Outcome{}, e
	}
	return f.out[rec.ID], nil
}

// qrec builds a minimal record with optional repros attached to its guard.
func qrec(id, status string, repros ...record.Repro) *record.Record {
	r := &record.Record{ID: id, Status: status}
	if len(repros) > 0 {
		r.Guard = &record.Guard{Repros: repros}
	}
	return r
}

func TestDraftCorpus_PersistsOnlyAttached(t *testing.T) {
	recs := []*record.Record{
		qrec("exp-0043", "quarantined"), // drafts + holds → persisted
		qrec("exp-0044", "quarantined"), // drafts + rejected → not persisted
		qrec("exp-0045", "quarantined"), // unsupported → not gated, not persisted
		qrec("exp-0046", "validated"),   // not a candidate → never run
		qrec("exp-0047", "quarantined", record.Repro{Path: "experience/repro/x", Kind: "positive"}), // already proven → skipped
	}
	runner := &fakeRunner{out: map[string]drafter.Outcome{
		"exp-0043": {Drafted: true, Attached: true, ReproPath: "experience/repro/exp-0043-io-ioutil"},
		"exp-0044": {Drafted: true, Attached: false, Reason: "gate rejected"},
		"exp-0045": {Drafted: false, Reason: "unsupported"},
	}}

	persisted := map[string]bool{}
	persist := func(_ string, rec *record.Record) error { persisted[rec.ID] = true; return nil }

	st, err := DraftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("DraftCorpus: %v", err)
	}

	// Only the candidates were gated; the validated and already-proven records were not.
	if got := strings.Join(runner.calls, ","); got != "exp-0043,exp-0044,exp-0045" {
		t.Errorf("gated %q; want only the three quarantined repro-less records", got)
	}
	if len(persisted) != 1 || !persisted["exp-0043"] {
		t.Errorf("persisted = %v; want only the held record exp-0043", persisted)
	}
	want := DraftStats{Attached: 1, Rejected: 1, Unsupported: 1, Skipped: 1}
	if st != want {
		t.Errorf("stats = %+v; want %+v", st, want)
	}
}

// A quarantined record whose only repro is a NEGATIVE (dead-end) proof still
// lacks a positive fail-to-pass proof, so it remains a candidate — it must be
// gated, not skipped. (Guards the hasRepro→positive-only fix.)
func TestDraftCorpus_NegativeReproIsStillACandidate(t *testing.T) {
	recs := []*record.Record{
		qrec("exp-0043", "quarantined", record.Repro{Path: "experience/repro/dead", Kind: "negative"}),
	}
	runner := &fakeRunner{out: map[string]drafter.Outcome{
		"exp-0043": {Drafted: true, Attached: true, ReproPath: "experience/repro/exp-0043-io-ioutil"},
	}}
	persisted := map[string]bool{}
	persist := func(_ string, rec *record.Record) error { persisted[rec.ID] = true; return nil }

	st, err := DraftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("DraftCorpus: %v", err)
	}
	if len(runner.calls) != 1 || st.Attached != 1 || st.Skipped != 0 {
		t.Errorf("negative-only record must be gated, not skipped; calls=%v stats=%+v", runner.calls, st)
	}
}

func TestDraftCorpus_RunErrorAbortsWithoutPersist(t *testing.T) {
	recs := []*record.Record{qrec("exp-0043", "quarantined")}
	runner := &fakeRunner{err: map[string]error{"exp-0043": errors.New("gate crashed")}}
	persist := func(string, *record.Record) error {
		t.Fatal("persist must not be called when the gate errors")
		return nil
	}
	if _, err := DraftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{}); err == nil {
		t.Fatal("a gate error must abort the run")
	}
}

// When the gate proved a repro (Attached) but persisting the record fails, the
// orphan repro directory the drafter wrote must be rolled back so a failed
// persist leaves no dangling files in the corpus.
func TestDraftCorpus_PersistErrorRollsBackRepro(t *testing.T) {
	corpus := t.TempDir()
	reproRel := "experience/repro/exp-0043-io-ioutil"
	reproAbs := filepath.Join(corpus, filepath.FromSlash(reproRel))
	if err := os.MkdirAll(reproAbs, 0o755); err != nil {
		t.Fatal(err)
	}
	recs := []*record.Record{qrec("exp-0043", "quarantined")}
	runner := &fakeRunner{out: map[string]drafter.Outcome{
		"exp-0043": {Drafted: true, Attached: true, ReproPath: reproRel},
	}}
	persist := func(string, *record.Record) error { return errors.New("disk full") }

	if _, err := DraftCorpus(context.Background(), corpus, recs, runner, persist, &bytes.Buffer{}); err == nil {
		t.Fatal("persist failure must surface as an error")
	}
	if _, err := os.Stat(reproAbs); !os.IsNotExist(err) {
		t.Errorf("orphan repro dir must be removed on persist failure; stat err = %v", err)
	}
}

// dry-run lists the quarantined candidates and writes nothing — and crucially
// it never constructs the broker, so it runs without Docker/runsc.

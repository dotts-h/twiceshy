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

	st, err := draftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("draftCorpus: %v", err)
	}

	// Only the candidates were gated; the validated and already-proven records were not.
	if got := strings.Join(runner.calls, ","); got != "exp-0043,exp-0044,exp-0045" {
		t.Errorf("gated %q; want only the three quarantined repro-less records", got)
	}
	if len(persisted) != 1 || !persisted["exp-0043"] {
		t.Errorf("persisted = %v; want only the held record exp-0043", persisted)
	}
	want := draftStats{attached: 1, rejected: 1, unsupported: 1, skipped: 1}
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

	st, err := draftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("draftCorpus: %v", err)
	}
	if len(runner.calls) != 1 || st.attached != 1 || st.skipped != 0 {
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
	if _, err := draftCorpus(context.Background(), "corpus", recs, runner, persist, &bytes.Buffer{}); err == nil {
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

	if _, err := draftCorpus(context.Background(), corpus, recs, runner, persist, &bytes.Buffer{}); err == nil {
		t.Fatal("persist failure must surface as an error")
	}
	if _, err := os.Stat(reproAbs); !os.IsNotExist(err) {
		t.Errorf("orphan repro dir must be removed on persist failure; stat err = %v", err)
	}
}

// dry-run lists the quarantined candidates and writes nothing — and crucially
// it never constructs the broker, so it runs without Docker/runsc.
func TestRunDraftDryRunListsCandidatesAndWritesNothing(t *testing.T) {
	dir := tempCorpus(t)
	// Seed the corpus with quarantined records via the existing importer.
	if err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db")}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("ingest go: %v", err)
	}

	var out bytes.Buffer
	if err := run(context.Background(), []string{"draft", "-corpus", dir, "-dry-run"}, &out, noEnv); err != nil {
		t.Fatalf("draft -dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "candidate") || !strings.Contains(out.String(), "dry-run") {
		t.Errorf("dry-run should list candidates; output = %q", out.String())
	}
	// No repro directories were created.
	if entries, _ := os.ReadDir(filepath.Join(dir, "experience", "repro")); len(entries) != 0 {
		t.Errorf("dry-run wrote repro artifacts: %v", entries)
	}
	// Records are untouched (no guard attached).
	recs, err := record.LoadCorpus(dir)
	if err != nil {
		t.Fatalf("LoadCorpus after dry-run: %v", err)
	}
	for _, r := range recs {
		if r.Guard != nil && len(r.Guard.Repros) > 0 {
			t.Errorf("dry-run must not attach a repro to %s", r.ID)
		}
	}
}

func TestRunDraftBadFlag(t *testing.T) {
	if err := run(context.Background(), []string{"draft", "-nope"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("an unknown flag must error")
	}
}

func TestRunDraftRejectsInvalidCorpus(t *testing.T) {
	if err := run(context.Background(), []string{"draft", "-corpus", t.TempDir(), "-dry-run"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("a corpus without experience/ must fail")
	}
}

// TestDraftersFrom covers the env-gated drafter chain (#0026 slice 3):
// deterministic-only by default, with the model drafter appended (model id
// defaulted, then honored) when TWICESHY_DRAFTER_URL is configured.
func TestDraftersFrom(t *testing.T) {
	// No model endpoint → deterministic-only (a bare checkout is unchanged).
	ds := draftersFrom(noEnv)
	if len(ds) != 1 {
		t.Fatalf("no env → deterministic drafter only; got %d", len(ds))
	}
	if ds[0].Name() != "go-deprecation-template" {
		t.Errorf("first drafter should be the deterministic template; got %q", ds[0].Name())
	}

	// TWICESHY_DRAFTER_URL set → model drafter appended; model id defaults.
	env := map[string]string{"TWICESHY_DRAFTER_URL": "http://localhost:11434"}
	ds = draftersFrom(func(k string) string { return env[k] })
	if len(ds) != 2 {
		t.Fatalf("with drafter url → deterministic + model; got %d", len(ds))
	}
	if got := ds[1].Name(); got != "model-drafter(qwen2.5-coder:14b)" {
		t.Errorf("model drafter should default the model id; got %q", got)
	}

	// An explicit model id is honored.
	env["TWICESHY_DRAFTER_MODEL"] = "custom:7b"
	ds = draftersFrom(func(k string) string { return env[k] })
	if got := ds[1].Name(); got != "model-drafter(custom:7b)" {
		t.Errorf("explicit model id should be used; got %q", got)
	}
}

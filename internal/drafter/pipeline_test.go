// SPDX-License-Identifier: AGPL-3.0-only

package drafter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// fakeBroker records the jobs it ran and returns a scripted Result, so the
// pipeline's draft→gate→attach wiring can be tested without runsc.
type fakeBroker struct {
	result repro.Result
	jobs   []repro.Job
}

func (f *fakeBroker) Run(_ context.Context, job repro.Job) (repro.Result, error) {
	f.jobs = append(f.jobs, job)
	return f.result, nil
}

func passing() repro.Result {
	return repro.Result{Prepare: repro.PhaseResult{ExitCode: 0}, Execute: repro.PhaseResult{ExitCode: 0}}
}

func newPipeline(t *testing.T, root string, res repro.Result) (*drafter.Pipeline, *fakeBroker) {
	t.Helper()
	b := &fakeBroker{result: res}
	rv := repro.NewRevalidator(b, root)
	return drafter.NewPipeline(drafter.NewGoDeprecationDrafter(), rv, root), b
}

func TestPipeline_AttachesProvenRepro(t *testing.T) {
	root := t.TempDir()
	p, b := newPipeline(t, root, passing())
	rec := goDeprecationRecord("exp-0050", "io/ioutil",
		"SA1019: ioutil.ReadFile is deprecated: As of Go 1.16, this function simply calls os.ReadFile.")

	out, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Drafted || !out.Attached {
		t.Fatalf("want drafted+attached; got %+v", out)
	}
	// The proven repro is now in the record's guard, still quarantined.
	if rec.Guard == nil || len(rec.Guard.Repros) != 1 {
		t.Fatalf("want one attached repro; guard=%+v", rec.Guard)
	}
	got := rec.Guard.Repros[0]
	if got.Path != out.ReproPath || got.Kind != "positive" {
		t.Errorf("attached repro wrong: %+v (path want %q)", got, out.ReproPath)
	}
	if rec.Status != "quarantined" {
		t.Errorf("attach must not promote; status=%q", rec.Status)
	}
	// The gate drove a prepare phase (prepare.sh) + an offline execute (repro.sh).
	if len(b.jobs) != 1 {
		t.Fatalf("want 1 gated job, got %d", len(b.jobs))
	}
	if len(b.jobs[0].Prepare) == 0 {
		t.Errorf("prepare phase not driven; job=%+v", b.jobs[0])
	}
	// The drafted dir is kept on disk.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(out.ReproPath))); err != nil {
		t.Errorf("attached repro dir should remain on disk: %v", err)
	}
}

func TestPipeline_RejectsAndCleansUpWhenGateFails(t *testing.T) {
	root := t.TempDir()
	// Execute fails → the draft did not truly reproduce → auto-rejected.
	failing := repro.Result{Prepare: repro.PhaseResult{ExitCode: 0}, Execute: repro.PhaseResult{ExitCode: 1}}
	p, _ := newPipeline(t, root, failing)
	rec := goDeprecationRecord("exp-0051", "io/ioutil", "SA1019: deprecated")

	out, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Attached {
		t.Fatal("a rejected draft must not be attached")
	}
	if rec.Guard != nil && len(rec.Guard.Repros) != 0 {
		t.Errorf("rejected repro must not linger in guard: %+v", rec.Guard.Repros)
	}
	// The orphan dir is removed so the corpus isn't polluted by failed drafts.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(out.ReproPath))); !os.IsNotExist(err) {
		t.Errorf("rejected draft dir should be removed; stat err=%v", err)
	}
}

func TestPipeline_PrepareFailureIsRejectedNotAttached(t *testing.T) {
	root := t.TempDir()
	// Prepare fails → broken, never a false hold (the offline execute is meaningless).
	prepFail := repro.Result{Prepare: repro.PhaseResult{ExitCode: 1}, Execute: repro.PhaseResult{ExitCode: 0}}
	p, _ := newPipeline(t, root, prepFail)
	rec := goDeprecationRecord("exp-0052", "io/ioutil", "SA1019: deprecated")

	out, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Attached {
		t.Fatal("a failed prepare must not attach")
	}
}

func TestPipeline_UnsupportedRecordIsSkipped(t *testing.T) {
	root := t.TempDir()
	p, b := newPipeline(t, root, passing())
	rec := goDeprecationRecord("exp-0053", "net/http", "SA1019: not cataloged")

	out, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatalf("Run on unsupported should not error, got %v", err)
	}
	if out.Drafted || out.Attached {
		t.Errorf("unsupported record must be skipped, not drafted; got %+v", out)
	}
	if out.Reason == "" {
		t.Error("skip should carry a reason")
	}
	if len(b.jobs) != 0 {
		t.Errorf("an unsupported record must never reach the gate; jobs=%d", len(b.jobs))
	}
	if rec.Guard != nil {
		t.Errorf("unsupported record's guard must be untouched; got %+v", rec.Guard)
	}
}

func TestPipeline_DraftErrorPropagates(t *testing.T) {
	root := t.TempDir()
	p, _ := newPipeline(t, root, passing())
	// Cataloged package but drifted diagnostic → Draft returns a hard error
	// (not ErrUnsupported); the pipeline surfaces it rather than silently skipping.
	rec := goDeprecationRecord("exp-0054", "io/ioutil", "no sa code here")

	if _, err := p.Run(context.Background(), rec); err == nil {
		t.Fatal("want the drift error to propagate from Run")
	}
}

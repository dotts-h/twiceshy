// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// fakeBroker returns programmed results and records the jobs it was asked to run.
type fakeBroker struct {
	run  func(job Job) (Result, error)
	jobs []Job
}

func (f *fakeBroker) Run(_ context.Context, job Job) (Result, error) {
	f.jobs = append(f.jobs, job)
	return f.run(job)
}

func exit(code int) Result { return Result{Execute: PhaseResult{ExitCode: code}} }

// writeRepro writes a repro script under root and returns its corpus-relative path.
func writeRepro(t *testing.T, root, name, body string) string {
	t.Helper()
	rel := filepath.Join("experience", "repro", name)
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func recWithRepro(id, status, reproPath, kind string) *record.Record {
	return &record.Record{
		ID:     id,
		Status: status,
		Path:   "experience/2026/" + id + "-x.md",
		Guard:  &record.Guard{Repros: []record.Repro{{Path: reproPath, Kind: kind}}},
	}
}

func newReval(b Broker, root string) *Revalidator {
	return NewRevalidator(b, root,
		WithClock(func() time.Time { return time.Unix(1750000000, 0).UTC() }))
}

func TestRevalidate_HoldingPositiveProposesPromotion(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0001.sh", "#!/bin/sh\nexit 0\n")
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, root)

	rep, atts, err := rv.RunWithAttestations(context.Background(),
		[]*record.Record{recWithRepro("exp-0001", "quarantined", p, "positive")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Findings) != 1 || len(atts) != 1 {
		t.Fatalf("want 1 finding+att, got %d/%d", len(rep.Findings), len(atts))
	}
	if !atts[0].Holds || atts[0].Inconclusive {
		t.Errorf("attestation: Holds=%v Inconclusive=%v", atts[0].Holds, atts[0].Inconclusive)
	}
	if got := atts[0].ReproducedUnder; len(got) != 1 || got[0] != "go1.25" {
		t.Errorf("ReproducedUnder=%v, want [go1.25]", got)
	}
	if !contains2(rep.Findings[0].Proposal, "promote to validated") {
		t.Errorf("proposal=%q, want a promotion", rep.Findings[0].Proposal)
	}
}

func TestRevalidate_BrokenReproProposesStale(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0002.sh", "#!/bin/sh\nexit 1\n")
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(1), nil }}
	rv := newReval(b, root)
	rep, atts, _ := rv.RunWithAttestations(context.Background(),
		[]*record.Record{recWithRepro("exp-0002", "validated", p, "positive")})
	if atts[0].Holds {
		t.Error("a broken repro must not hold")
	}
	if !contains2(rep.Findings[0].Proposal, "stale") {
		t.Errorf("proposal=%q, want a stale proposal", rep.Findings[0].Proposal)
	}
}

func TestRevalidate_SkippedIsInconclusive(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0003.sh", "#!/bin/sh\nexit 75\n")
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(75), nil }}
	rv := newReval(b, root)
	rep, atts, _ := rv.RunWithAttestations(context.Background(),
		[]*record.Record{recWithRepro("exp-0003", "quarantined", p, "positive")})
	if !atts[0].Inconclusive || atts[0].Holds {
		t.Errorf("skip-only run must be inconclusive: %+v", atts[0])
	}
	if !contains2(rep.Findings[0].Issue, "inconclusive") {
		t.Errorf("issue=%q", rep.Findings[0].Issue)
	}
}

func TestRevalidate_NeverMutatesRecords(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0004.sh", "#!/bin/sh\nexit 0\n")
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, root)
	rec := recWithRepro("exp-0004", "quarantined", p, "positive")
	if _, _, err := rv.RunWithAttestations(context.Background(), []*record.Record{rec}); err != nil {
		t.Fatal(err)
	}
	if rec.Status != "quarantined" || rec.Provenance.ValidatedAt != nil {
		t.Errorf("revalidator must be report-only; record was mutated: status=%q validated_at=%v",
			rec.Status, rec.Provenance.ValidatedAt)
	}
}

func TestRevalidate_RecordsWithoutReprosAreSkipped(t *testing.T) {
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, t.TempDir())
	rec := &record.Record{ID: "exp-0009", Status: "quarantined"} // no Guard
	rep, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if len(rep.Findings) != 0 || len(atts) != 0 {
		t.Errorf("a record with no repros must be skipped, got %d findings", len(rep.Findings))
	}
	if len(b.jobs) != 0 {
		t.Errorf("no broker job should run for a record with no repros")
	}
}

func TestRevalidate_PathTraversalIsError(t *testing.T) {
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, t.TempDir())
	rec := recWithRepro("exp-0010", "quarantined", "../../etc/passwd", "positive")
	_, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if atts[0].Holds {
		t.Error("a traversing path must not validate")
	}
	if len(b.jobs) != 0 {
		t.Error("a traversing path must never reach the broker")
	}
}

func TestRevalidate_MultiEntryMatrixRequiresAllToHold(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0005.sh", "#!/bin/sh\n# value comes from broker\n")
	// Holds on go1.24, broken on go1.25 (keyed off the label env the reval sets).
	b := &fakeBroker{run: func(job Job) (Result, error) {
		if job.Env["TWICESHY_MATRIX_LABEL"] == "go1.25" {
			return exit(1), nil
		}
		return exit(0), nil
	}}
	img := PinnedGoImage
	rv := NewRevalidator(b, root,
		WithClock(func() time.Time { return time.Unix(1750000000, 0).UTC() }),
		WithMatrix([]MatrixEntry{{Label: "go1.24", Image: img}, {Label: "go1.25", Image: img}}))
	_, atts, _ := rv.RunWithAttestations(context.Background(),
		[]*record.Record{recWithRepro("exp-0005", "validated", p, "positive")})
	if atts[0].Holds {
		t.Error("must not hold when one matrix entry is broken")
	}
	if got := atts[0].ReproducedUnder; len(got) != 1 || got[0] != "go1.24" {
		t.Errorf("ReproducedUnder=%v, want [go1.24]", got)
	}
}

func TestRevalidate_LegacySingleReproTreatedAsPositive(t *testing.T) {
	root := t.TempDir()
	p := writeRepro(t, root, "0006.sh", "#!/bin/sh\nexit 0\n")
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, root)
	rec := &record.Record{ID: "exp-0006", Status: "quarantined", Path: "x.md",
		Guard: &record.Guard{Repro: &p}}
	_, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if !atts[0].Holds {
		t.Errorf("legacy guard.repro should be run as a positive: %+v", atts[0])
	}
	if len(b.jobs) != 1 {
		t.Errorf("legacy repro should run once, got %d jobs", len(b.jobs))
	}
}

func contains2(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

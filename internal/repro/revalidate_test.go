// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func (f *fakeBroker) Healthy(context.Context) error { return nil }

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
	// Non-empty stderr so the per-repro Detail is meaningful: runRepro's exit-code
	// switch (revalidate.go:226-235) maps exit!=0/75 → "broken" with the firstLine
	// of stderr as Detail. firstLine trims to the first line.
	b := &fakeBroker{run: func(Job) (Result, error) {
		return Result{Execute: PhaseResult{ExitCode: 1, Stderr: "boom: assertion failed\nmore"}}, nil
	}}
	rv := newReval(b, root)
	rep, atts, _ := rv.RunWithAttestations(context.Background(),
		[]*record.Record{recWithRepro("exp-0002", "validated", p, "positive")})
	if atts[0].Holds {
		t.Error("a broken repro must not hold")
	}
	// The structured per-repro outcome is the load-bearing artifact a reviewer
	// reads — assert the status/exit/detail, not just the coarse Holds=false.
	out := atts[0].Matrix[0].Repros[0]
	if out.Status != "broken" {
		t.Errorf("repro status=%q, want \"broken\"", out.Status)
	}
	if out.ExitCode != 1 {
		t.Errorf("repro exit=%d, want 1", out.ExitCode)
	}
	if out.Detail != "boom: assertion failed" {
		t.Errorf("repro detail=%q, want the trimmed first line of stderr", out.Detail)
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
	// exit 75 (EX_TEMPFAIL) maps to a "skipped" status with the canonical detail
	// (revalidate.go:229-231): a regression mapping it to "broken"/"holds" must fail.
	out := atts[0].Matrix[0].Repros[0]
	if out.Status != "skipped" {
		t.Errorf("repro status=%q, want \"skipped\"", out.Status)
	}
	if out.Detail != "environment cannot run repro (EX_TEMPFAIL)" {
		t.Errorf("repro detail=%q, want the EX_TEMPFAIL skip detail", out.Detail)
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
	rep, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if atts[0].Holds {
		t.Error("a traversing path must not validate")
	}
	if len(b.jobs) != 0 {
		t.Error("a traversing path must never reach the broker")
	}
	// A resolve failure is an "error", NOT a benign "skipped"/Inconclusive: the
	// security-relevant mapping is resolve-failure → error → propose-stale (no
	// promotion). If a refactor surfaced traversal as "skipped" it would become a
	// no-op finding (Inconclusive + "no change"), silently dropping the signal.
	if atts[0].Inconclusive {
		t.Error("a resolve failure must be an error, not inconclusive/skipped")
	}
	if got := atts[0].Matrix[0].Repros[0].Status; got != "error" {
		t.Errorf("traversal repro status=%q, want \"error\"", got)
	}
	if !contains2(rep.Findings[0].Proposal, "stale") {
		t.Errorf("proposal=%q, want a non-promotion stale proposal", rep.Findings[0].Proposal)
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

func TestRevalidate_DirectoryReproDrivesPrepareAndExecute(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join("experience", "repro", "0007-dep")
	abs := filepath.Join(root, dir)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"prepare.sh": "#!/bin/sh\ngo install example/tool\n",
		"repro.sh":   "#!/bin/sh\ntool ./...\n",
		"go.mod":     "module dep\ngo 1.25\n",
		"a.go":       "package dep\n",
	} {
		if err := os.WriteFile(filepath.Join(abs, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := &fakeBroker{run: func(Job) (Result, error) {
		return Result{Prepare: PhaseResult{ExitCode: 0}, Execute: PhaseResult{ExitCode: 0}}, nil
	}}
	rv := newReval(b, root)
	rec := recWithRepro("exp-0007", "quarantined", filepath.ToSlash(dir), "positive")
	if _, atts, err := rv.RunWithAttestations(context.Background(), []*record.Record{rec}); err != nil {
		t.Fatalf("Run: %v", err)
	} else if !atts[0].Holds {
		t.Fatalf("directory repro should hold: %+v", atts[0])
	}
	if len(b.jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(b.jobs))
	}
	job := b.jobs[0]
	// prepare.sh present → the networked prepare phase is driven.
	if len(job.Prepare) == 0 || job.Prepare[len(job.Prepare)-1] != workDir+"/prepare.sh" {
		t.Errorf("prepare phase not wired from prepare.sh: %v", job.Prepare)
	}
	if job.Execute[len(job.Execute)-1] != workDir+"/repro.sh" {
		t.Errorf("execute phase not wired from repro.sh: %v", job.Execute)
	}
	// All directory files are staged for the sandbox.
	for _, f := range []string{"repro.sh", "prepare.sh", "go.mod", "a.go"} {
		if _, ok := job.Files[f]; !ok {
			t.Errorf("staged files missing %q; got %v", f, keysOf(job.Files))
		}
	}
}

func TestRevalidate_DirectoryReproWithoutReproShIsError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join("experience", "repro", "0008-bad")
	if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{run: func(Job) (Result, error) { return exit(0), nil }}
	rv := newReval(b, root)
	rec := recWithRepro("exp-0008", "quarantined", filepath.ToSlash(dir), "positive")
	_, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if atts[0].Holds {
		t.Error("a directory repro without repro.sh must not hold")
	}
	if len(b.jobs) != 0 {
		t.Error("a malformed directory repro must never reach the broker")
	}
}

func TestRevalidate_PrepareFailureIsBrokenNotHolds(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join("experience", "repro", "0009-dep")
	if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"prepare.sh", "repro.sh"} {
		if err := os.WriteFile(filepath.Join(root, dir, n), []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Prepare fails; execute would be meaningless — must report broken, not holds.
	// Non-empty Prepare.Stderr so the "prepare failed: ..." Detail is meaningful.
	b := &fakeBroker{run: func(Job) (Result, error) {
		return Result{
			Prepare: PhaseResult{ExitCode: 1, Stderr: "go: download failed\nnetwork down"},
			Execute: PhaseResult{ExitCode: 0},
		}, nil
	}}
	rv := newReval(b, root)
	rec := recWithRepro("exp-0009", "quarantined", filepath.ToSlash(dir), "positive")
	_, atts, _ := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if atts[0].Holds {
		t.Error("a failed prepare must not produce holds")
	}
	// The prepare-failed branch (revalidate.go:219-223) sets status "broken" and a
	// "prepare failed: ..." detail — a passing execute exit 0 must NOT mask it.
	out := atts[0].Matrix[0].Repros[0]
	if out.Status != "broken" {
		t.Errorf("repro status=%q, want \"broken\" on prepare failure", out.Status)
	}
	if !strings.HasPrefix(out.Detail, "prepare failed:") {
		t.Errorf("repro detail=%q, want a \"prepare failed:\" prefix", out.Detail)
	}
}

func keysOf(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
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

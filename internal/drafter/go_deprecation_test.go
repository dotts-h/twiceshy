// SPDX-License-Identifier: AGPL-3.0-only

package drafter_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/record"
)

func goDeprecationRecord(id, pkg, diagnostic string) *record.Record {
	return &record.Record{
		ID:        id,
		Status:    "quarantined",
		Path:      "experience/2026/" + id + ".md",
		Symptom:   &record.Symptom{ErrorSignatures: []string{diagnostic}},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: pkg}},
	}
}

// driftDiagnostic is the staticcheck-bearing signature the cataloged io/ioutil
// template expects, reused by the applies_to-selection cases below.
const ioutilDiagnostic = "SA1019: ioutil.ReadAll is deprecated: As of Go 1.16, this function simply calls io.ReadAll."

// TestGoDeprecation_SelectsFirstGoApplesTo pins goPackage's selection rule: it
// returns the FIRST applies_to entry whose Ecosystem EqualFold "Go" with a
// non-empty Package — skipping non-Go entries and empty-package Go entries,
// case-insensitively. goDeprecationRecord forces a single Go entry, so these
// realistic multi-ecosystem shapes are built by hand.
func TestGoDeprecation_SelectsFirstGoApplesTo(t *testing.T) {
	d := drafter.NewGoDeprecationDrafter()
	cases := []struct {
		name      string
		appliesTo []record.AppliesTo
		wantDraft bool // true: a cataloged Go pkg is selected and drafts; false: ErrUnsupported
	}{
		{
			// A leading non-Go (PyPI) entry must be SKIPPED; the lowercase "go"
			// entry that follows is matched case-insensitively and wins.
			name: "skips leading non-Go entry, EqualFold lowercase go",
			appliesTo: []record.AppliesTo{
				{Ecosystem: "PyPI", Package: "numpy"},
				{Ecosystem: "go", Package: "io/ioutil"},
			},
			wantDraft: true,
		},
		{
			// A leading Go entry with an EMPTY package is skipped (the a.Package != ""
			// guard) and the next valid Go entry wins — pins skip-and-continue directly.
			name: "skips empty-package Go entry, next valid wins",
			appliesTo: []record.AppliesTo{
				{Ecosystem: "Go", Package: ""},
				{Ecosystem: "go", Package: "io/ioutil"},
			},
			wantDraft: true,
		},
		{
			// Only an empty-package Go entry: goPackage returns "" which is not in the
			// catalog → ErrUnsupported.
			name: "only empty-package Go entry is unsupported",
			appliesTo: []record.AppliesTo{
				{Ecosystem: "Go", Package: ""},
			},
			wantDraft: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rec := &record.Record{
				ID:        "exp-0050",
				Status:    "quarantined",
				Path:      "experience/2026/exp-0050.md",
				Symptom:   &record.Symptom{ErrorSignatures: []string{ioutilDiagnostic}},
				AppliesTo: tc.appliesTo,
			}
			dir, err := d.Draft(context.Background(), root, rec)
			if tc.wantDraft {
				if err != nil {
					t.Fatalf("Draft: %v", err)
				}
				if !strings.HasPrefix(dir, "experience/repro/") {
					t.Errorf("expected a staged repro dir, got %q", dir)
				}
			} else {
				if !errors.Is(err, drafter.ErrUnsupported) {
					t.Fatalf("want ErrUnsupported, got %v", err)
				}
				if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
					t.Errorf("unsupported draft must write nothing; found %d entries", len(entries))
				}
			}
		})
	}
}

func TestGoDeprecation_DraftsCatalogedPackage(t *testing.T) {
	root := t.TempDir()
	d := drafter.NewGoDeprecationDrafter()
	rec := goDeprecationRecord("exp-0042", "io/ioutil",
		"SA1019: ioutil.ReadAll is deprecated: As of Go 1.16, this function simply calls io.ReadAll.")

	dir, err := d.Draft(context.Background(), root, rec)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	// Corpus-relative, slash, namespaced by record id + package.
	if !strings.HasPrefix(dir, "experience/repro/") || strings.Contains(dir, "\\") {
		t.Errorf("dir not a clean corpus-relative path: %q", dir)
	}

	abs := filepath.Join(root, filepath.FromSlash(dir))
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	// Two self-contained modules: a parent module with sub-packages doesn't
	// resolve cleanly for `staticcheck .` offline (#0026 slice-1 gotcha).
	for _, f := range []string{"trap/go.mod", "trap/main.go", "fix/go.mod", "fix/main.go", "prepare.sh", "repro.sh"} {
		if _, err := os.Stat(filepath.Join(abs, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing staged file %q: %v", f, err)
		}
	}

	// The trap exercises the deprecated package; the fix uses the replacement.
	if !strings.Contains(read("trap/main.go"), `"io/ioutil"`) {
		t.Errorf("trap should import the deprecated package; got:\n%s", read("trap/main.go"))
	}
	if strings.Contains(read("fix/main.go"), `"io/ioutil"`) {
		t.Errorf("fix must NOT import the deprecated package; got:\n%s", read("fix/main.go"))
	}

	// prepare warms staticcheck (networked); repro asserts the trap is flagged and
	// the fix is clean, keyed on the staticcheck code.
	if !strings.Contains(read("prepare.sh"), "staticcheck") {
		t.Errorf("prepare.sh should install staticcheck; got:\n%s", read("prepare.sh"))
	}
	repro := read("repro.sh")
	if !strings.Contains(repro, "SA1019") {
		t.Errorf("repro.sh should grep the staticcheck code SA1019; got:\n%s", repro)
	}
	// The fix-check must assert a clean exit code, not merely the absence of the
	// code — else a staticcheck crash on a non-compiling fix would be a false hold.
	if !strings.Contains(repro, "fixrc") || !strings.Contains(repro, "-ne 0") {
		t.Errorf("repro.sh fix-check must assert staticcheck exited cleanly; got:\n%s", repro)
	}
	// noexec-/tmp trap (exp-0017): a Go compile-then-exec needs an exec-able TMPDIR.
	if !strings.Contains(repro+read("prepare.sh"), "/work") {
		t.Errorf("scripts should pin caches/TMPDIR under the work volume")
	}

	// Scripts must be executable.
	for _, s := range []string{"prepare.sh", "repro.sh"} {
		info, err := os.Stat(filepath.Join(abs, s))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s not executable: %v", s, info.Mode())
		}
	}
}

func TestGoDeprecation_DraftsThirdPartyFix(t *testing.T) {
	root := t.TempDir()
	d := drafter.NewGoDeprecationDrafter()
	// strings.Title (Go 1.18): unlike the stdlib→stdlib entries, the fix needs a
	// THIRD-PARTY module (golang.org/x/text/cases). The networked prepare phase must
	// warm that module so the offline execute phase can still type-check the fix
	// (exp-0044). This is the slice-2.5 extension to the deterministic catalog.
	rec := goDeprecationRecord("exp-0044", "strings",
		"SA1019: strings.Title is deprecated: The rule Title uses for word boundaries does not handle Unicode punctuation properly.")

	dir, err := d.Draft(context.Background(), root, rec)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	abs := filepath.Join(root, filepath.FromSlash(dir))
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	// The trap uses the deprecated stdlib symbol; the fix uses the third-party replacement.
	if !strings.Contains(read("trap/main.go"), `"strings"`) {
		t.Errorf("trap should import the deprecated package strings; got:\n%s", read("trap/main.go"))
	}
	if !strings.Contains(read("fix/main.go"), "golang.org/x/text") {
		t.Errorf("fix should import the third-party replacement golang.org/x/text; got:\n%s", read("fix/main.go"))
	}
	// The fix module must DECLARE the third-party require so it resolves.
	if !strings.Contains(read("fix/go.mod"), "require golang.org/x/text") {
		t.Errorf("fix/go.mod must require golang.org/x/text; got:\n%s", read("fix/go.mod"))
	}
	// prepare (networked) must warm the fix's third-party module into the offline-
	// readable module cache AND still install staticcheck.
	prep := read("prepare.sh")
	if !strings.Contains(prep, "staticcheck") {
		t.Errorf("prepare.sh should install staticcheck; got:\n%s", prep)
	}
	if !strings.Contains(prep, "go mod") || !strings.Contains(prep, "fix") {
		t.Errorf("prepare.sh should warm the fix module (go mod …) so execute builds offline; got:\n%s", prep)
	}
	// repro keyed on the staticcheck code, with the clean-exit fix assertion.
	repro := read("repro.sh")
	if !strings.Contains(repro, "SA1019") {
		t.Errorf("repro.sh should grep SA1019; got:\n%s", repro)
	}
	if !strings.Contains(repro, "fixrc") || !strings.Contains(repro, "-ne 0") {
		t.Errorf("repro.sh fix-check must assert a clean staticcheck exit; got:\n%s", repro)
	}
}

func TestGoDeprecation_UnsupportedPackageReturnsErrUnsupported(t *testing.T) {
	root := t.TempDir()
	d := drafter.NewGoDeprecationDrafter()
	rec := goDeprecationRecord("exp-0099", "net/http", "SA1019: something not in the catalog")

	_, err := d.Draft(context.Background(), root, rec)
	if err == nil {
		t.Fatal("want ErrUnsupported for an uncataloged package, got nil")
	}
	if !errors.Is(err, drafter.ErrUnsupported) {
		t.Errorf("want errors.Is(err, ErrUnsupported); got %v", err)
	}
	// Nothing must be written for an unsupported record.
	if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
		t.Errorf("unsupported draft must write nothing; found %d entries", len(entries))
	}
}

func TestGoDeprecation_DiagnosticDriftIsRejected(t *testing.T) {
	root := t.TempDir()
	d := drafter.NewGoDeprecationDrafter()
	// Cataloged package, but the record's diagnostic no longer matches the
	// template's staticcheck code — the fact drifted; refuse to draft a stale repro.
	rec := goDeprecationRecord("exp-0043", "io/ioutil", "some unrelated diagnostic with no SA code")

	_, err := d.Draft(context.Background(), root, rec)
	if err == nil {
		t.Fatal("want an error when the record diagnostic doesn't match the template check")
	}
	// Drift is a HARD error, not a skip: it MUST NOT be ErrUnsupported, or the
	// pipeline would silently skip a drifted record instead of propagating the
	// fault (TestPipeline_DraftErrorPropagates depends on this distinction).
	if errors.Is(err, drafter.ErrUnsupported) {
		t.Errorf("diagnostic drift must be a hard error, not ErrUnsupported; got %v", err)
	}
	// The rendered message should name the drifted staticcheck code so the
	// fault is diagnosable (secondary to the ErrUnsupported guard above).
	if !strings.Contains(err.Error(), "SA1019") {
		t.Errorf("drift error should name the template check SA1019; got %v", err)
	}
	// Nothing must be written when drift is detected.
	if entries, _ := os.ReadDir(filepath.Join(root, "experience", "repro")); len(entries) != 0 {
		t.Errorf("drifted draft must write nothing; found %d entries", len(entries))
	}
}

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

	if _, err := d.Draft(context.Background(), root, rec); err == nil {
		t.Fatal("want an error when the record diagnostic doesn't match the template check")
	}
}

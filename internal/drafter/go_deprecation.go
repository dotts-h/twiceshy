// SPDX-License-Identifier: AGPL-3.0-only

package drafter

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// goModVersion is the `go` directive the generated modules declare. It pairs
// with the digest-pinned Go repro image (repro.PinnedGoImage) under
// GOTOOLCHAIN=local, so the toolchain is the image's, never downloaded.
const goModVersion = "1.25"

// staticcheckVersion is the pinned staticcheck the prepare phase installs. Pin so
// a generated repro's verdict is reproducible across runs.
const staticcheckVersion = "2025.1"

// goDeprecation is one cataloged template: a Go stdlib deprecation whose trap
// (using the deprecated symbol) staticcheck flags with `check`, and whose fix
// (using the stdlib replacement) is clean. Both bodies are twiceshy's own minimal
// code — the executed original test is the licensing firewall (ADR-0011 §8).
type goDeprecation struct {
	check string // the staticcheck code the trap must raise, e.g. "SA1019"
	trap  string // full source of trap/main.go (imports the deprecated package)
	fix   string // full source of fix/main.go (imports the replacement only)
}

// goDeprecationCatalog keys templates by applies_to package. It deliberately
// covers only the cleanest class — stdlib-deprecated → stdlib-replacement, so the
// fix compiles offline with no third-party module. Harder cases (e.g. a fix that
// needs golang.org/x/text) are out of scope for the deterministic drafter and
// fall to the model drafter (#0026 slice 3).
var goDeprecationCatalog = map[string]goDeprecation{
	"io/ioutil": {
		check: "SA1019",
		trap:  "package main\n\nimport \"io/ioutil\"\n\nfunc main() {\n\t_, _ = ioutil.ReadFile(\"/dev/null\")\n}\n",
		fix:   "package main\n\nimport \"os\"\n\nfunc main() {\n\t_, _ = os.ReadFile(\"/dev/null\")\n}\n",
	},
	"math/rand": {
		check: "SA1019",
		trap:  "package main\n\nimport \"math/rand\"\n\nfunc main() {\n\trand.Seed(1)\n}\n",
		fix:   "package main\n\nimport \"math/rand\"\n\nfunc main() {\n\t_ = rand.New(rand.NewSource(1))\n}\n",
	},
}

// GoDeprecationDrafter is a deterministic template drafter: given a quarantined
// Go-stdlib-deprecation record, it emits a self-contained, gateable repro
// directory (a trap module + a fix module + prepare.sh + repro.sh). It is
// stateless — the corpus root is supplied per Draft call.
type GoDeprecationDrafter struct {
	catalog map[string]goDeprecation
}

// NewGoDeprecationDrafter returns a drafter backed by the built-in catalog.
func NewGoDeprecationDrafter() *GoDeprecationDrafter {
	return &GoDeprecationDrafter{catalog: goDeprecationCatalog}
}

// Name implements Drafter.
func (*GoDeprecationDrafter) Name() string { return "go-deprecation-template" }

// Draft implements Drafter: write the candidate repro for rec under root and
// return its corpus-relative path, or ErrUnsupported if the record's package is
// not cataloged.
func (d *GoDeprecationDrafter) Draft(_ context.Context, root string, rec *record.Record) (string, error) {
	pkg := goPackage(rec)
	tmpl, ok := d.catalog[pkg]
	if !ok {
		return "", fmt.Errorf("package %q: %w", pkg, ErrUnsupported)
	}
	// Guard against fact drift: the record's diagnostic must still carry the
	// staticcheck code the template asserts, else we'd attach a stale repro.
	if !diagnosticMatches(rec, tmpl.check) {
		return "", fmt.Errorf("record %s diagnostic does not mention %s — fact drifted from the %q template",
			rec.ID, tmpl.check, pkg)
	}

	dir := path.Join("experience", "repro", slug(rec.ID, pkg))
	abs := filepath.Join(root, filepath.FromSlash(dir))
	files := map[string]struct {
		body string
		exec bool
	}{
		"trap/go.mod":  {goMod("deptrap"), false},
		"trap/main.go": {tmpl.trap, false},
		"fix/go.mod":   {goMod("depfix"), false},
		"fix/main.go":  {tmpl.fix, false},
		"prepare.sh":   {prepareScript(), true},
		"repro.sh":     {reproScript(tmpl.check), true},
	}
	for rel, f := range files {
		full := filepath.Join(abs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		mode := os.FileMode(0o644)
		if f.exec {
			mode = 0o755
		}
		if err := os.WriteFile(full, []byte(f.body), mode); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// goPackage returns the first Go applies_to package, or "" if none.
func goPackage(rec *record.Record) string {
	for _, a := range rec.AppliesTo {
		if strings.EqualFold(a.Ecosystem, "Go") && a.Package != "" {
			return a.Package
		}
	}
	return ""
}

// diagnosticMatches reports whether any of the record's error signatures mentions
// the staticcheck code.
func diagnosticMatches(rec *record.Record, check string) bool {
	if rec.Symptom == nil {
		return false
	}
	for _, sig := range rec.Symptom.ErrorSignatures {
		if strings.Contains(sig, check) {
			return true
		}
	}
	return false
}

// slug builds a filesystem-safe directory name from the record id and package.
func slug(id, pkg string) string {
	s := strings.ToLower(id + "-" + pkg)
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, s)
}

func goMod(name string) string {
	return fmt.Sprintf("module %s\n\ngo %s\n", name, goModVersion)
}

// scriptEnv pins the Go caches and an exec-able TMPDIR under the work volume.
// /tmp is mounted noexec and the Go toolchain compiles-then-execs from TMPDIR
// (exp-0017 / #0026 slice 1), so both phases point HOME/TMPDIR/caches at /work.
const scriptEnv = "export GOTOOLCHAIN=local GOCACHE=/work/.gocache GOPATH=/work/.gopath GOBIN=/work/bin TMPDIR=/work\n"

func prepareScript() string {
	return "#!/bin/sh\nset -e\n" + scriptEnv +
		"go install honnef.co/go/tools/cmd/staticcheck@" + staticcheckVersion + "\n"
}

func reproScript(check string) string {
	return "#!/bin/sh\nset -u\n" + scriptEnv +
		"command -v go >/dev/null 2>&1 || { echo SKIP; exit 75; }\n" +
		"[ -x /work/bin/staticcheck ] || { echo 'SKIP: staticcheck not warmed'; exit 75; }\n" +
		// The trap must be flagged with the deprecation code. A staticcheck crash
		// here yields no match → "NOT REPRODUCED" (the safe direction: don't attach).
		"if ! (cd /work/trap && /work/bin/staticcheck .) 2>&1 | grep -q " + check + "; then echo 'NOT REPRODUCED: trap not flagged'; exit 1; fi\n" +
		// The fix must analyze CLEANLY — assert a zero exit, not merely the absence
		// of the code. Any non-zero (the deprecation code, another finding, or a
		// compile error / staticcheck crash) means the replacement is not proven
		// clean; keying only on absence would treat a crash as a false hold.
		"fixout=$(cd /work/fix && /work/bin/staticcheck . 2>&1); fixrc=$?\n" +
		"if [ \"$fixrc\" -ne 0 ]; then echo \"FIX BROKEN: staticcheck did not pass cleanly (rc=$fixrc): $fixout\"; exit 1; fi\n" +
		"echo OK; exit 0\n"
}

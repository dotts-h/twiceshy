// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/selfaudit"
)

// runSelfAudit dogfoods twiceshy on its own dependencies (#0014): it matches the
// modules in go.mod against the vulnerability advisories the importer has ingested
// (#0007) and reports any dependency a Go-ecosystem advisory flags as affected at
// its current version. It exits non-zero on a hit, so a timer (OnFailure→notify)
// or a CI step turns a match into an alert. Read-only — the product on itself.
func runSelfAudit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("self-audit", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	gomod := fs.String("gomod", "go.mod", "path to the go.mod to audit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	f, err := os.Open(*gomod)
	if err != nil {
		return fmt.Errorf("opening go.mod: %w", err)
	}
	defer func() { _ = f.Close() }()
	deps, err := selfaudit.ParseGoMod(f)
	if err != nil {
		return err
	}

	hits := selfaudit.Audit(deps, recs)
	if len(hits) == 0 {
		_, _ = fmt.Fprintf(out, "self-audit: %d dependencies checked against %d records — no advisory matches\n",
			len(deps), len(recs))
		return nil
	}

	_, _ = fmt.Fprintf(out, "self-audit: %d advisory match(es) across %d dependencies:\n", len(hits), len(deps))
	for _, h := range hits {
		fixed := h.Fixed
		if fixed == "" {
			fixed = "(none published)"
		}
		_, _ = fmt.Fprintf(out, "  %s@%s — %s (%s): introduced %s, fixed %s\n",
			h.Dep.Path, h.Dep.Version, h.AdvisoryID, h.RecordID, h.Introduced, fixed)
	}
	return fmt.Errorf("self-audit: %d dependency advisory match(es) — see report above", len(hits))
}

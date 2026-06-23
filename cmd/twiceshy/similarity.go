// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/similarity"
)

// runSimilarity is the optional ADR-0011 §5 net (#0090): it flags an authored
// record whose prose runs near-verbatim to a supplied reference text — the leak a
// model can introduce by emitting a memorized public snippet while "authoring".
// It is a LEAD for human review, never an auto-reject, so it always exits 0; the
// reviewer supplies the suspected source(s) with -against (repeatable). The
// primary control stays author-from-spec discipline (docs/AUTHORING.md).
//
// -record is the record's corpus-relative path (experience/YYYY/NNNN-slug.md); run
// from the corpus checkout root. Only the authored prose is compared — error
// signatures (observed strings) and the repro code (original by construction +
// execution-validated, §5 mitigation 2) are excluded.
func runSimilarity(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("similarity", flag.ContinueOnError)
	recordPath := fs.String("record", "", "corpus-relative path of the authored record to check (experience/YYYY/NNNN-slug.md)")
	n := fs.Int("n", similarity.DefaultN, "shingle size: consecutive words that count as one near-verbatim phrase")
	threshold := fs.Float64("threshold", 0.15, "containment at or above which a reference is flagged as a lead")
	var refs stringSlice
	fs.Var(&refs, "against", "reference text file to compare against (repeatable)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *recordPath == "" {
		return fmt.Errorf("similarity: -record is required")
	}
	if len(refs) == 0 {
		return fmt.Errorf("similarity: at least one -against reference is required")
	}

	data, err := os.ReadFile(*recordPath)
	if err != nil {
		return fmt.Errorf("similarity: reading record: %w", err)
	}
	rec, err := record.Parse(corpusRelPath(*recordPath), data)
	if err != nil {
		return fmt.Errorf("similarity: parsing record: %w", err)
	}
	prose := authoredProse(rec)

	flagged := false
	for _, refPath := range refs {
		refData, err := os.ReadFile(refPath)
		if err != nil {
			return fmt.Errorf("similarity: reading reference %s: %w", refPath, err)
		}
		rep := similarity.Assess(prose, string(refData), *n)
		if rep.Flagged(*threshold) {
			flagged = true
			_, _ = fmt.Fprintf(out, "FLAGGED %s — %.0f%% of the record's %d-word phrases appear in this reference:\n",
				refPath, rep.Containment*100, rep.N)
			for _, m := range rep.Matches {
				_, _ = fmt.Fprintf(out, "  · %q\n", m)
			}
		} else {
			_, _ = fmt.Fprintf(out, "clean   %s — %.0f%% containment (below the %.0f%% threshold)\n",
				refPath, rep.Containment*100, *threshold*100)
		}
	}

	if flagged {
		_, _ = fmt.Fprintln(out, "\nsimilarity: near-verbatim overlap found — a LEAD for human review (ADR-0011 §5), not an auto-reject.")
	} else {
		_, _ = fmt.Fprintln(out, "\nsimilarity: no near-verbatim overlap above the threshold.")
	}
	return nil
}

// authoredProse is the record's human-written narrative — the surface where a
// memorized public snippet would land. It deliberately excludes error signatures
// (factual observed strings) and the repro code (original by construction +
// execution-validated, ADR-0011 §5 mitigation 2).
func authoredProse(r *record.Record) string {
	var b strings.Builder
	w := func(s string) {
		if s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	w(r.Title)
	if r.Symptom != nil {
		w(r.Symptom.Summary)
	}
	if r.Resolution != nil {
		w(r.Resolution.RootCause)
		w(r.Resolution.Fix)
		for _, d := range r.Resolution.DeadEnds {
			w(d.Tried)
			w(d.WhyItFailed)
		}
	}
	w(r.Body)
	return strings.TrimSpace(b.String())
}

// corpusRelPath trims a filesystem path down to its corpus-relative
// experience/YYYY/NNNN-slug.md tail so record.Parse's path rules accept it whether
// the user passed a relative path (run from the corpus root) or an absolute path
// into a corpus checkout. A path with no experience/ segment is returned as-is, so
// the parser surfaces the path-format error.
func corpusRelPath(p string) string {
	p = filepath.ToSlash(p)
	if i := strings.LastIndex(p, "experience/"); i >= 0 {
		return p[i:]
	}
	return p
}

// stringSlice is a repeatable string flag, so -against can be given more than once.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

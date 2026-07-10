// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/dotts-h/twiceshy/internal/corpusquality"
	"github.com/dotts-h/twiceshy/internal/record"
)

func runCorpusQuality(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("corpus-quality", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	rep := corpusquality.Build(*corpus, recs)
	if *asJSON {
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(body))
		return nil
	}

	_, _ = fmt.Fprintf(out, "corpus quality: %d records\n", rep.TotalRecords)
	printNamedCounts(out, "status", record.Statuses, rep.StatusCounts)
	printNamedCounts(out, "kind", record.Kinds, rep.KindCounts)
	_, _ = fmt.Fprintf(out, "  validated actionable behavioral: %d\n", rep.ValidatedActionableBehavioral)
	_, _ = fmt.Fprintf(out, "  records with guard:             %d\n", rep.RecordsWithGuard)
	_, _ = fmt.Fprintf(out, "  repros declared/runnable:       %d/%d\n", rep.DeclaredRepros, rep.RunnableRepros)
	_, _ = fmt.Fprintf(out, "  provenance coverage:            %d/%d\n", rep.Coverage.Provenance, rep.TotalRecords)
	_, _ = fmt.Fprintf(out, "  source-license coverage:        %d/%d\n", rep.Coverage.SourceLicense, rep.TotalRecords)
	_, _ = fmt.Fprintf(out, "  source-URL coverage:            %d/%d\n", rep.Coverage.SourceURL, rep.TotalRecords)
	licenses := make([]string, 0, len(rep.LicenseCounts))
	for license := range rep.LicenseCounts {
		licenses = append(licenses, license)
	}
	sort.Strings(licenses)
	_, _ = fmt.Fprintln(out, "licenses:")
	for _, license := range licenses {
		_, _ = fmt.Fprintf(out, "  %-32s %d\n", license, rep.LicenseCounts[license])
	}
	return nil
}

func printNamedCounts(out io.Writer, label string, names []string, counts map[string]int) {
	_, _ = fmt.Fprintf(out, "%s:\n", label)
	for _, name := range names {
		_, _ = fmt.Fprintf(out, "  %-16s %d\n", name, counts[name])
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dotts-h/twiceshy/internal/idf"
)

// runIdfBuild builds an idf document-frequency table from a manifest of
// licensed data sources and writes the gzip TSV table to -out.
func runIdfBuild(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("idf-build", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to the idf source manifest YAML (required)")
	outPath := fs.String("out", "", "path to write the gzip TSV idf table to (required)")
	maxWords := fs.Int("max-words", 200000, "maximum number of highest document-frequency words to keep")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *manifestPath == "" {
		return fmt.Errorf("idf-build requires -manifest <path>")
	}
	if *outPath == "" {
		return fmt.Errorf("idf-build requires -out <path>")
	}

	f, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", *outPath, err)
	}

	if err := idf.Build(*manifestPath, *maxWords, f); err != nil {
		_ = f.Close()
		_ = os.Remove(*outPath)
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", *outPath, err)
	}

	_, _ = fmt.Fprintf(out, "idf-build: wrote table to %s\n", *outPath)
	return nil
}

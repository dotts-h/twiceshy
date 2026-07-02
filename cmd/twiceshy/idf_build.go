// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/dotts-h/twiceshy/internal/idf"
	"gopkg.in/yaml.v3"
)

// idfManifestSource is one data source entry in an idf build manifest,
// mirroring internal/idf's (unexported) ManifestSource fields.
type idfManifestSource struct {
	Name    string `yaml:"name"`
	Path    string `yaml:"path"`
	License string `yaml:"license"`
}

// idfManifest is the parsed form of a YAML manifest file listing idf data
// sources.
type idfManifest struct {
	Sources []idfManifestSource `yaml:"sources"`
}

// loadManifest reads the YAML manifest file at path and parses it into an
// idfManifest.
func loadManifest(path string) (*idfManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest idfManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// idfAllowedLicenses is the permissive SPDX license id allowlist idf sources
// must carry, mirroring internal/idf's (unexported) allowlist.
var idfAllowedLicenses = map[string]bool{
	"MIT":          true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"Apache-2.0":   true,
	"ISC":          true,
	"Python-2.0":   true,
	"Unlicense":    true,
	"CC-BY-4.0":    true,
}

// validateLicenses checks every source in the manifest against the
// permissive license allowlist, returning an error naming the offending
// source's name and license if any source's license is not allowed.
func validateLicenses(m *idfManifest) error {
	for _, src := range m.Sources {
		if !idfAllowedLicenses[src.License] {
			return fmt.Errorf("source %q has disallowed license %q", src.Name, src.License)
		}
	}
	return nil
}

// buildDocFreq walks each source's directory root recursively, treating one
// file as one document. Unreadable files and binary files (detected via a
// NUL byte in the first 512 bytes) are skipped. Each distinct
// idf.Tokenize-derived token is counted once per document into docFreq, and
// totalDocs is incremented once per counted document.
func buildDocFreq(sources []idfManifestSource) (map[string]uint64, uint64, error) {
	docFreq := make(map[string]uint64)
	var totalDocs uint64

	for _, source := range sources {
		walkErr := filepath.WalkDir(source.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Unreadable directory entries are skipped rather than
				// aborting the whole walk.
				return nil
			}
			if d.IsDir() {
				return nil
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			if idfIsBinary(data) {
				return nil
			}

			seen := make(map[string]struct{})
			for _, tok := range idf.Tokenize(string(data)) {
				seen[tok] = struct{}{}
			}

			for tok := range seen {
				docFreq[tok]++
			}
			totalDocs++

			return nil
		})
		if walkErr != nil {
			return nil, 0, walkErr
		}
	}

	return docFreq, totalDocs, nil
}

// idfIsBinary reports whether data looks like binary content, detected via
// the presence of a NUL byte within the first 512 bytes.
func idfIsBinary(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	return bytes.IndexByte(data[:n], 0) != -1
}

// idfDFEntry pairs a word with its document frequency.
type idfDFEntry struct {
	Word string
	DF   uint64
}

// topN returns the top n entries from df sorted by descending document
// frequency, breaking ties lexicographically by word ascending.
func topN(df map[string]uint64, n int) []idfDFEntry {
	entries := make([]idfDFEntry, 0, len(df))
	for word, count := range df {
		entries = append(entries, idfDFEntry{Word: word, DF: count})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DF != entries[j].DF {
			return entries[i].DF > entries[j].DF
		}
		return entries[i].Word < entries[j].Word
	})

	if n < len(entries) {
		entries = entries[:n]
	}
	return entries
}

// writeTable gzip-writes a TSV table to w: a "docs\t<totalDocs>" header line
// followed by one "word\t<df>" line per entry, in the given slice order.
func writeTable(w io.Writer, totalDocs uint64, entries []idfDFEntry) error {
	gw := gzip.NewWriter(w)

	writeLine := func(format string, args ...any) error {
		if _, err := fmt.Fprintf(gw, format, args...); err != nil {
			gw.Close()
			return err
		}
		return nil
	}

	if err := writeLine("docs\t%d\n", totalDocs); err != nil {
		return err
	}

	for _, e := range entries {
		if err := writeLine("%s\t%d\n", e.Word, e.DF); err != nil {
			return err
		}
	}

	return gw.Close()
}

// runIdfBuild builds an idf document-frequency table from a manifest of
// licensed data sources: load the manifest, refuse to run if any source
// carries a non-allowlisted license, walk every source computing per-word
// document frequencies, truncate to -max-words highest-df words, and write
// the gzip TSV table to -out.
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

	manifest, err := loadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	if err := validateLicenses(manifest); err != nil {
		return fmt.Errorf("validating licenses: %w", err)
	}

	docFreq, totalDocs, err := buildDocFreq(manifest.Sources)
	if err != nil {
		return fmt.Errorf("building document frequencies: %w", err)
	}

	entries := topN(docFreq, *maxWords)

	f, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", *outPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := writeTable(f, totalDocs, entries); err != nil {
		return fmt.Errorf("writing table to %s: %w", *outPath, err)
	}

	_, _ = fmt.Fprintf(out, "idf-build: wrote %d word(s) over %d document(s) to %s\n", len(entries), totalDocs, *outPath)
	return nil
}

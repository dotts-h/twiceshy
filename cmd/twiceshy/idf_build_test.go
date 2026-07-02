// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// idfBuildFixturePaths returns the absolute paths of the sourceA and sourceB
// fixture corpora shared with internal/idf's own buildDocFreq tests
// (internal/idf/testdata/build): sourceA holds doc1.txt ("cat cat cat dog"),
// doc2.txt ("dog bird") and sub/doc4.txt ("eagle eagle"); sourceB holds
// doc3.txt ("cat fish fish fish") and a binary.dat that must be skipped.
// Aggregated over both sources: docFreq = {cat:2, dog:2, bird:1, eagle:1,
// fish:1}, totalDocs = 4.
func idfBuildFixturePaths(t *testing.T) (sourceA, sourceB string) {
	t.Helper()
	a, err := filepath.Abs(filepath.Join("..", "..", "internal", "idf", "testdata", "build", "sourceA"))
	if err != nil {
		t.Fatalf("Abs(sourceA): %v", err)
	}
	b, err := filepath.Abs(filepath.Join("..", "..", "internal", "idf", "testdata", "build", "sourceB"))
	if err != nil {
		t.Fatalf("Abs(sourceB): %v", err)
	}
	return a, b
}

// writeIdfManifest renders a minimal manifest YAML file (matching
// internal/idf's ManifestSource fields: name/path/license) into t.TempDir()
// and returns its path.
func writeIdfManifest(t *testing.T, sources []struct{ Name, Path, License string }) string {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("sources:\n")
	for _, s := range sources {
		fmt.Fprintf(&sb, "  - name: %s\n    path: %q\n    license: %s\n", s.Name, s.Path, s.License)
	}
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest): %v", err)
	}
	return path
}

// readIdfTable gunzip-decompresses and parses the TSV table at path — a
// "docs\t<N>" header line followed by "word\t<df>" rows — mirroring the
// format internal/idf's (unexported) writeTable/parseTable round-trip on,
// without depending on that package's unexported symbols across a package
// boundary. Returns the total-docs count and the per-word document
// frequencies, in encounter order for word count assertions.
func readIdfTable(t *testing.T, path string) (totalDocs uint64, df map[string]uint64, order []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip.NewReader(%s): %v", path, err)
	}
	defer gr.Close()

	sc := bufio.NewScanner(gr)
	if !sc.Scan() {
		t.Fatalf("table at %s has no header line", path)
	}
	header := sc.Text()
	fields := strings.SplitN(header, "\t", 2)
	if len(fields) != 2 || fields[0] != "docs" {
		t.Fatalf("table header = %q, want \"docs\\t<N>\"", header)
	}
	totalDocs, err = strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		t.Fatalf("parsing docs count from header %q: %v", header, err)
	}

	df = make(map[string]uint64)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		row := strings.SplitN(line, "\t", 2)
		if len(row) != 2 {
			t.Fatalf("table row = %q, want \"word\\t<df>\"", line)
		}
		count, err := strconv.ParseUint(row[1], 10, 64)
		if err != nil {
			t.Fatalf("parsing df from row %q: %v", line, err)
		}
		df[row[0]] = count
		order = append(order, row[0])
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning table at %s: %v", path, err)
	}
	return totalDocs, df, order
}

// TestRunIdfBuild_RoundTripsDocFreqAndTotalDocs verifies runIdfBuild loads
// the manifest, walks every source's document tree via buildDocFreq, and
// writes a table to -out whose totalDocs and per-word document frequencies
// match the fixture corpus exactly — not just a single constant.
func TestRunIdfBuild_RoundTripsDocFreqAndTotalDocs(t *testing.T) {
	sourceA, sourceB := idfBuildFixturePaths(t)
	manifest := writeIdfManifest(t, []struct{ Name, Path, License string }{
		{Name: "sourceA", Path: sourceA, License: "MIT"},
		{Name: "sourceB", Path: sourceB, License: "MIT"},
	})
	out := filepath.Join(t.TempDir(), "table.tsv.gz")

	var stdout bytes.Buffer
	if err := runIdfBuild([]string{
		"-manifest", manifest,
		"-out", out,
		"-max-words", "100",
	}, &stdout); err != nil {
		t.Fatalf("runIdfBuild(...) returned error: %v", err)
	}

	totalDocs, df, _ := readIdfTable(t, out)

	// 4 documents total (doc1, doc2, sub/doc4 from sourceA + doc3 from
	// sourceB; binary.dat is skipped) — distinct from any per-word count
	// below, so this alone rules out a stub that always writes one value.
	if totalDocs != 4 {
		t.Fatalf("TotalDocs = %d, want 4", totalDocs)
	}

	// "cat" appears in doc1.txt (3x) and doc3.txt (1x) -> per-document df 2.
	if got, ok := df["cat"]; !ok || got != 2 {
		t.Fatalf(`df["cat"] = (%d, ok=%v), want (2, true)`, got, ok)
	}

	// "fish" appears only in doc3.txt (3x in one document) -> per-document
	// df 1. A different, non-trivially-related value from both totalDocs (4)
	// and df["cat"] (2), so the two assertions can't be satisfied by a
	// function returning one hardcoded number everywhere.
	if got, ok := df["fish"]; !ok || got != 1 {
		t.Fatalf(`df["fish"] = (%d, ok=%v), want (1, true)`, got, ok)
	}
}

// TestRunIdfBuild_MaxWordsTruncatesOutput verifies -max-words is applied via
// topN, truncating the on-disk word count to (at most) the requested value —
// keeping the highest document-frequency words, "cat" and "dog" (both df 2)
// over "bird"/"eagle"/"fish" (all df 1).
func TestRunIdfBuild_MaxWordsTruncatesOutput(t *testing.T) {
	sourceA, sourceB := idfBuildFixturePaths(t)
	manifest := writeIdfManifest(t, []struct{ Name, Path, License string }{
		{Name: "sourceA", Path: sourceA, License: "MIT"},
		{Name: "sourceB", Path: sourceB, License: "MIT"},
	})
	out := filepath.Join(t.TempDir(), "table.tsv.gz")

	if err := runIdfBuild([]string{
		"-manifest", manifest,
		"-out", out,
		"-max-words", "2",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runIdfBuild(...) returned error: %v", err)
	}

	_, df, order := readIdfTable(t, out)

	// Exactly 2 rows on disk, not the full 5-word vocabulary — proves
	// -max-words actually truncates rather than being ignored.
	if len(order) != 2 {
		t.Fatalf("word count on disk = %d (%v), want 2", len(order), order)
	}
	if _, ok := df["cat"]; !ok {
		t.Fatalf("truncated table missing %q, want present (highest df)", "cat")
	}
	if _, ok := df["dog"]; !ok {
		t.Fatalf("truncated table missing %q, want present (tied-highest df)", "dog")
	}
	if _, ok := df["fish"]; ok {
		t.Fatalf("truncated table contains %q, want truncated away (lower df)", "fish")
	}
}

// TestRunIdfBuild_RefusesDisallowedLicense verifies runIdfBuild calls
// validateLicenses and refuses to run — without writing an output file —
// when any manifest source carries a license outside the allowlist,
// returning an error that names the offending source.
func TestRunIdfBuild_RefusesDisallowedLicense(t *testing.T) {
	sourceA, sourceB := idfBuildFixturePaths(t)
	manifest := writeIdfManifest(t, []struct{ Name, Path, License string }{
		{Name: "sourceA", Path: sourceA, License: "MIT"},
		{Name: "badSource", Path: sourceB, License: "GPL-3.0-only"},
	})
	out := filepath.Join(t.TempDir(), "table.tsv.gz")

	err := runIdfBuild([]string{
		"-manifest", manifest,
		"-out", out,
		"-max-words", "100",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runIdfBuild(...) returned nil error, want error naming the disallowed-license source")
	}
	if !strings.Contains(err.Error(), "badSource") {
		t.Fatalf("runIdfBuild(...) error = %q, want it to name the offending source %q", err.Error(), "badSource")
	}

	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatalf("output file %s was written despite the license check failing", out)
	}
}

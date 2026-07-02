package idf

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

// TestWriteTable_RoundTripsHeaderAndRows verifies writeTable gzip-writes a
// TSV stream to w consisting of a "docs\t<totalDocs>" header line followed
// by one "word\t<df>" line per entry, in the given slice order.
func TestWriteTable_RoundTripsHeaderAndRows(t *testing.T) {
	entries := []dfEntry{
		{Word: "apple", DF: 7},
		{Word: "banana", DF: 19},
	}

	var buf bytes.Buffer
	if err := writeTable(&buf, 42, entries); err != nil {
		t.Fatalf("writeTable(...) returned error: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip.NewReader(output) returned error: %v", err)
	}
	defer gr.Close()

	raw, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading gunzipped output returned error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(lines), lines)
	}

	if lines[0] != "docs\t42" {
		t.Fatalf("header line = %q, want %q", lines[0], "docs\t42")
	}
	if lines[1] != "apple\t7" {
		t.Fatalf("row[0] = %q, want %q", lines[1], "apple\t7")
	}
	if lines[2] != "banana\t19" {
		t.Fatalf("row[1] = %q, want %q", lines[2], "banana\t19")
	}
}

// TestWriteTable_DistinctInputsProduceDistinctOutput verifies writeTable
// reflects a different totalDocs and a different, differently-ordered entry
// slice into correspondingly different header and row output, ruling out an
// implementation that always emits a single constant table.
func TestWriteTable_DistinctInputsProduceDistinctOutput(t *testing.T) {
	entries := []dfEntry{
		{Word: "zebra", DF: 3},
		{Word: "cherry", DF: 100},
	}

	var buf bytes.Buffer
	if err := writeTable(&buf, 9, entries); err != nil {
		t.Fatalf("writeTable(...) returned error: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip.NewReader(output) returned error: %v", err)
	}
	defer gr.Close()

	raw, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading gunzipped output returned error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(lines), lines)
	}

	if lines[0] != "docs\t9" {
		t.Fatalf("header line = %q, want %q", lines[0], "docs\t9")
	}
	if lines[1] != "zebra\t3" {
		t.Fatalf("row[0] = %q, want %q", lines[1], "zebra\t3")
	}
	if lines[2] != "cherry\t100" {
		t.Fatalf("row[1] = %q, want %q", lines[2], "cherry\t100")
	}
}

package idf

import (
	"strings"
	"testing"
)

// TestParseTable_RoundTripsFixture verifies parseTable reads the
// "docs\t<N>" header line followed by "word\t<df>" rows from a decompressed
// TSV stream, populating TotalDocs() and per-word DF() lookups.
func TestParseTable_RoundTripsFixture(t *testing.T) {
	fixture := "docs\t42\napple\t7\nbanana\t19\n"

	table, err := parseTable(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseTable(fixture) returned error: %v", err)
	}

	if got := table.TotalDocs(); got != 42 {
		t.Fatalf("TotalDocs() = %d, want 42", got)
	}

	appleDF, ok := table.DF("apple")
	if !ok {
		t.Fatalf("DF(%q) ok = false, want true", "apple")
	}
	if appleDF != 7 {
		t.Fatalf("DF(%q) = %d, want 7", "apple", appleDF)
	}

	bananaDF, ok := table.DF("banana")
	if !ok {
		t.Fatalf("DF(%q) ok = false, want true", "banana")
	}
	if bananaDF != 19 {
		t.Fatalf("DF(%q) = %d, want 19", "banana", bananaDF)
	}

	if _, ok := table.DF("cherry"); ok {
		t.Fatalf("DF(%q) ok = true, want false for word absent from fixture", "cherry")
	}
}

// TestParseTable_DFIsCaseInsensitive verifies DF lookups normalize case, so
// a word stored lowercase in the table is still found when queried with any
// mixed-case or upper-case spelling.
func TestParseTable_DFIsCaseInsensitive(t *testing.T) {
	fixture := "docs\t10\nwidget\t3\n"

	table, err := parseTable(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseTable(fixture) returned error: %v", err)
	}

	for _, query := range []string{"widget", "WIDGET", "Widget", "wIdGeT"} {
		df, ok := table.DF(query)
		if !ok {
			t.Fatalf("DF(%q) ok = false, want true", query)
		}
		if df != 3 {
			t.Fatalf("DF(%q) = %d, want 3", query, df)
		}
	}
}

// TestParseTable_Available reports whether a header-only table (docs count
// present but zero data rows) is distinguished from a populated table: the
// header-only case must report Available() == false, while a table with at
// least one word/df row must report Available() == true.
func TestParseTable_Available(t *testing.T) {
	headerOnly, err := parseTable(strings.NewReader("docs\t0\n"))
	if err != nil {
		t.Fatalf("parseTable(header-only) returned error: %v", err)
	}
	if headerOnly.Available() {
		t.Fatalf("Available() = true for header-only table, want false")
	}

	populated, err := parseTable(strings.NewReader("docs\t5\nonly\t1\n"))
	if err != nil {
		t.Fatalf("parseTable(populated) returned error: %v", err)
	}
	if !populated.Available() {
		t.Fatalf("Available() = false for populated table, want true")
	}
}

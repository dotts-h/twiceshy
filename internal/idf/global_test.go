package idf

import "testing"

// TestGlobal_ReturnsCachedEmptyTable verifies Global() lazily
// gzip-decompresses and parses the embedded table.tsv.gz asset via
// sync.Once, returning a non-nil *Table. The shipped asset is a valid but
// empty table (header "docs\t0", no rows), so the parsed result must report
// TotalDocs() == 0 and Available() == false: an empty asset signals "no
// data available" rather than "every word is rare". Repeated calls must
// return the exact same cached *Table instance rather than re-parsing.
func TestGlobal_ReturnsCachedEmptyTable(t *testing.T) {
	first := Global()
	if first == nil {
		t.Fatalf("Global() = nil, want non-nil *Table")
	}

	if got := first.TotalDocs(); got != 0 {
		t.Fatalf("Global().TotalDocs() = %d, want 0", got)
	}

	if first.Available() {
		t.Fatalf("Global().Available() = true, want false for empty embedded table")
	}

	second := Global()
	if second != first {
		t.Fatalf("Global() returned a different *Table instance on second call, want the same cached pointer")
	}
}

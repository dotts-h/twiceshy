package idf

import "testing"

// TestGlobal_ReturnsCachedRealTable verifies Global() lazily
// gzip-decompresses and parses the embedded table.tsv.gz asset via
// sync.Once, returning a non-nil *Table. The shipped asset is the real
// dev-code corpus table (ADR-0017 phase 1; provenance in
// scripts/idf-manifest.yaml), so the parsed result must be Available()
// with a positive TotalDocs(), a generic dev word ("the") must be present
// with a high document ratio, and a token absent from the corpus must
// report ok=false. Repeated calls must return the exact same cached
// *Table instance rather than re-parsing.
func TestGlobal_ReturnsCachedRealTable(t *testing.T) {
	first := Global()
	if first == nil {
		t.Fatalf("Global() = nil, want non-nil *Table")
	}

	total := first.TotalDocs()
	if total == 0 {
		t.Fatalf("Global().TotalDocs() = 0, want > 0 for the shipped corpus table")
	}

	if !first.Available() {
		t.Fatalf("Global().Available() = false, want true for the shipped corpus table")
	}

	df, ok := first.DF("the")
	if !ok {
		t.Fatalf(`Global().DF("the") reported absent, want present in any dev-code corpus`)
	}
	if ratio := float64(df) / float64(total); ratio < 0.5 {
		t.Errorf(`DF("the")/TotalDocs() = %.3f, want >= 0.5 — "the" must read as generic`, ratio)
	}

	if _, ok := first.DF("zzz-no-such-token-zzz"); ok {
		t.Errorf(`Global().DF("zzz-no-such-token-zzz") reported present, want absent`)
	}

	second := Global()
	if second != first {
		t.Fatalf("Global() returned a different *Table instance on second call, want the same cached pointer")
	}
}

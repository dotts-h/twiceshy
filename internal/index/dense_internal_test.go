// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"math"
	"reflect"
	"testing"
)

func TestEncodeDecodeVecRoundTrip(t *testing.T) {
	in := []float32{0, 1, -1, 3.5, 1e-7, -2.25, 768.0}
	got := decodeVec(encodeVec(in))
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], in[i])
		}
	}
}

func TestCosine(t *testing.T) {
	if got := cosine([]float32{1, 0}, []float32{1, 0}); math.Abs(got-1) > 1e-9 {
		t.Errorf("identical = %v, want 1", got)
	}
	if got := cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(got) > 1e-9 {
		t.Errorf("orthogonal = %v, want 0", got)
	}
	if got := cosine([]float32{1, 1}, []float32{2, 2}); math.Abs(got-1) > 1e-9 {
		t.Errorf("parallel = %v, want 1", got)
	}
	if got := cosine([]float32{1}, []float32{1, 2}); got != 0 {
		t.Errorf("len mismatch = %v, want 0", got)
	}
	if got := cosine([]float32{0, 0}, []float32{1, 1}); got != 0 {
		t.Errorf("zero vector = %v, want 0", got)
	}
}

func TestRRFFusePrefersHitsRankedHighInBothLists(t *testing.T) {
	lex := []Hit{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	den := []Hit{{ID: "b"}, {ID: "a"}, {ID: "d"}}
	out := rrfFuse(lex, den)
	if len(out) != 4 {
		t.Fatalf("want 4 fused, got %d", len(out))
	}
	// a (ranks 1,2) and b (ranks 2,1) tie EXACTLY (both 1/(60+1)+1/(60+2)), so
	// the result order falls entirely to the id tie-break (ascending). Pin it:
	// a must precede b, not "a or b in either order" — a reversed or unstable
	// tie-break must fail here.
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("tie-break must order by id ascending: out[0]=%q out[1]=%q, want a,b", out[0].ID, out[1].ID)
	}
	// And the single-list hits (c, d) stay below the doubly-ranked a/b.
	if out[2].ID == "a" || out[2].ID == "b" {
		t.Errorf("single-list hit ranked above a/b: %+v", out)
	}
	// Pin a's exact fused score: rank 1 in lex + rank 2 in den → 1/(rrfK+1)+1/(rrfK+2).
	wantTop := 1.0/(rrfK+1) + 1.0/(rrfK+2)
	if got := out[0].Score; math.Abs(got-wantTop) > 1e-12 {
		t.Errorf("top fused score = %v, want %v (1/(rrfK+1)+1/(rrfK+2))", got, wantTop)
	}
}

// Asymmetric lists give every id a DISTINCT fused score, so the full result
// order is fully determined by score (no tie-break) — pin it exactly.
func TestRRFFuseOrdersByDistinctFusedScore(t *testing.T) {
	lex := []Hit{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	den := []Hit{{ID: "a"}, {ID: "c"}, {ID: "d"}}
	out := rrfFuse(lex, den)
	// a: ranks 1,1 → 1/61+1/61. c: ranks 3,2 → 1/63+1/62. b: rank 1 (lex only,
	// rank 2) → 1/62. d: rank 3 (den only) → 1/63. Ordering: a > c > b > d.
	gotIDs := make([]string, len(out))
	for i, h := range out {
		gotIDs[i] = h.ID
	}
	want := []string{"a", "c", "b", "d"}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("fused order = %v, want %v", gotIDs, want)
	}
}

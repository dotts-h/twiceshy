// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"math"
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
	// a (ranks 1,2) and b (ranks 2,1) appear in both → outrank c/d (one list each).
	if out[0].ID != "a" && out[0].ID != "b" {
		t.Errorf("top = %q, want a or b (in both lists)", out[0].ID)
	}
	if out[2].ID == "a" || out[2].ID == "b" {
		t.Errorf("single-list hit ranked above a/b: %+v", out)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package index

import "testing"

// fakeIDFProvider is a test double for the idfProvider seam: it implements
// the same small method subset of *idf.Table (Available, TotalDocs, DF) so
// globallyCommonWord's behavior can be triangulated without touching the
// real embedded idf.Global() table.
type fakeIDFProvider struct {
	available bool
	totalDocs uint64
	df        map[string]uint64
}

func (f fakeIDFProvider) Available() bool  { return f.available }
func (f fakeIDFProvider) TotalDocs() uint64 { return f.totalDocs }
func (f fakeIDFProvider) DF(word string) (uint64, bool) {
	v, ok := f.df[word]
	return v, ok
}

// TestGloballyCommonWord_RatioCeiling pins the phase-1 idfMaxDocRatio=0.10
// ceiling (ADR-0017): a token is globally common only when the seam provider
// is available, the token is present via DF, and df/totalDocs strictly
// exceeds the ceiling. Multiple non-trivially-related expected values here
// so a stub always returning one constant can't satisfy the table.
func TestGloballyCommonWord_RatioCeiling(t *testing.T) {
	orig := idfProvider
	defer func() { idfProvider = orig }()

	tests := []struct {
		name     string
		provider idfTableProvider
		token    string
		want     bool
	}{
		{
			name:     "unavailable provider is never common, regardless of df",
			provider: fakeIDFProvider{available: false, totalDocs: 100, df: map[string]uint64{"the": 90}},
			token:    "the",
			want:     false,
		},
		{
			name:     "ratio strictly above ceiling (0.15 > 0.10) is common",
			provider: fakeIDFProvider{available: true, totalDocs: 100, df: map[string]uint64{"the": 15}},
			token:    "the",
			want:     true,
		},
		{
			name:     "ratio exactly at ceiling (0.10 == 0.10) is NOT common",
			provider: fakeIDFProvider{available: true, totalDocs: 100, df: map[string]uint64{"boundary": 10}},
			token:    "boundary",
			want:     false,
		},
		{
			name:     "ratio strictly above ceiling at a different scale (101/1000) is common",
			provider: fakeIDFProvider{available: true, totalDocs: 1000, df: map[string]uint64{"common": 101}},
			token:    "common",
			want:     true,
		},
		{
			name:     "token absent from DF is never common even with docs loaded",
			provider: fakeIDFProvider{available: true, totalDocs: 1000, df: map[string]uint64{}},
			token:    "ghost",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idfProvider = tt.provider
			if got := globallyCommonWord(tt.token); got != tt.want {
				t.Errorf("globallyCommonWord(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

// TestIdfMaxDocRatio_Value pins the phase-1 conservative ceiling literal
// itself, separate from the predicate's control flow above.
func TestIdfMaxDocRatio_Value(t *testing.T) {
	if idfMaxDocRatio != 0.10 {
		t.Errorf("idfMaxDocRatio = %v, want 0.10", idfMaxDocRatio)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeIDFProvider is a test double for the idfProvider seam: it implements
// the same small method subset of *idf.Table (Available, TotalDocs, DF) so
// globallyCommonWord's behavior can be triangulated without touching the
// real embedded idf.Global() table.
type fakeIDFProvider struct {
	available bool
	totalDocs uint64
	df        map[string]uint64
}

func (f fakeIDFProvider) Available() bool   { return f.available }
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

// TestDiscriminativeTokensVia_GlobalIDFFilter drives the behavior change this
// test pins: discriminativeTokensVia must additionally reject a token that
// passes every existing local check (stripControl/hasAlnum/pushStopwords/
// ecosystem names/seen dedup, eligible df in [1, pushMaxDF]) when
// globallyCommonWord(tok) is true, and must report how many tokens were
// dropped by that global check ALONE as a new int result. Three
// non-trivially-related cases triangulate the behavior so a stub that always
// returns one constant (e.g. always dropped=0, or always the full/empty
// slice) cannot satisfy the table:
//
//  1. regression: with the injection seam left at its default (the shipped
//     embedded idf table, which ships with zero word/df rows and so is
//     never Available()), the returned tokens and the new drop count are
//     byte-identical to pre-change behavior — nothing is ever globally
//     filtered.
//  2. an injected table makes exactly one locally-rare token globally
//     common: that token is dropped from the slice and the count is 1.
//  3. an injected table that does not contain a given token at all must
//     NOT filter it — DF's "not found" must never be misread as common.
func TestDiscriminativeTokensVia_GlobalIDFFilter(t *testing.T) {
	ix, err := Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = ix.Close() }()

	ctx := context.Background()
	const text = "raretoken globcommon anotherrare"
	// Every token is locally eligible (df=1, within [1, pushMaxDF]) regardless
	// of which word is asked about — isolates the assertions to the NEW global
	// filter, not the existing local eligibility gate.
	localDF := func(_ context.Context, _ string) (int, error) { return 1, nil }

	t.Run("regression: default shipped-empty table filters nothing", func(t *testing.T) {
		// idfProvider is intentionally left untouched here: this is the
		// process default (idf.Global()), the shipped embedded table with
		// zero rows, so Available() is false and globallyCommonWord is
		// false for every token — pre-change behavior, byte-identical.
		gotTokens, gotDropped, err := ix.discriminativeTokensVia(ctx, text, localDF)
		if err != nil {
			t.Fatalf("discriminativeTokensVia: %v", err)
		}
		wantTokens := []string{"raretoken", "globcommon", "anotherrare"}
		if !reflect.DeepEqual(gotTokens, wantTokens) {
			t.Errorf("tokens = %v, want %v", gotTokens, wantTokens)
		}
		if gotDropped != 0 {
			t.Errorf("dropped = %d, want 0", gotDropped)
		}
	})

	t.Run("injected table drops the one globally-common token, count=1", func(t *testing.T) {
		orig := idfProvider
		defer func() { idfProvider = orig }()
		idfProvider = fakeIDFProvider{
			available: true,
			totalDocs: 100,
			df:        map[string]uint64{"globcommon": 50}, // 50/100 = 0.50 > 0.10 ceiling
		}

		gotTokens, gotDropped, err := ix.discriminativeTokensVia(ctx, text, localDF)
		if err != nil {
			t.Fatalf("discriminativeTokensVia: %v", err)
		}
		wantTokens := []string{"raretoken", "anotherrare"}
		if !reflect.DeepEqual(gotTokens, wantTokens) {
			t.Errorf("tokens = %v, want %v", gotTokens, wantTokens)
		}
		if gotDropped != 1 {
			t.Errorf("dropped = %d, want 1", gotDropped)
		}
	})

	t.Run("token absent from an available table is not filtered", func(t *testing.T) {
		orig := idfProvider
		defer func() { idfProvider = orig }()
		idfProvider = fakeIDFProvider{
			available: true,
			totalDocs: 100,
			// None of this query's tokens are present in the table at all.
			df: map[string]uint64{"unrelatedword": 90},
		}

		gotTokens, gotDropped, err := ix.discriminativeTokensVia(ctx, text, localDF)
		if err != nil {
			t.Fatalf("discriminativeTokensVia: %v", err)
		}
		wantTokens := []string{"raretoken", "globcommon", "anotherrare"}
		if !reflect.DeepEqual(gotTokens, wantTokens) {
			t.Errorf("tokens = %v, want %v", gotTokens, wantTokens)
		}
		if gotDropped != 0 {
			t.Errorf("dropped = %d, want 0", gotDropped)
		}
	})
}

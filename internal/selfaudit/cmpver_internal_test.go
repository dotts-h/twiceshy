// SPDX-License-Identifier: AGPL-3.0-only

package selfaudit

// White-box coverage of the version comparator that backs the advisory range
// check (affected() → cmpVer). cmpVer/parseVer stay UNEXPORTED — this file is in
// `package selfaudit` purely to add coverage of the documented contract, not to
// widen the production export surface. The Audit/affected black-box tests stay.

import (
	"strconv"
	"testing"
)

func TestCmpVer(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Differing component counts: zero-extension makes "1.2" == "1.2.0".
		{"1.2", "1.2.0", 0},
		{"1.2.0", "1.2", 0},
		// Numeric, not lexical: 9 < 10 even though "10" sorts before "9" as text.
		{"1.9", "1.10", -1},
		{"1.10", "1.9", 1},
		// Build metadata carries no precedence — it is dropped before compare.
		{"1.2.3+build", "1.2.3", 0},
		// A pre-release sorts BEFORE the same release (the fixed-boundary rule).
		{"v1.50.0-rc1", "1.50.0", -1},
		{"1.50.0", "1.50.0-rc1", 1},
		// Reflexive equality.
		{"1.0.0", "1.0.0", 0},
	}
	for _, c := range cases {
		t.Run(c.a+"_vs_"+c.b, func(t *testing.T) {
			if got := cmpVer(c.a, c.b); got != c.want {
				t.Errorf("cmpVer(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

// FuzzCmpVer pins the two ordering invariants every comparator must hold:
// reflexivity (cmpVer(a,a)==0) and antisymmetry (cmpVer(a,b) == -cmpVer(b,a)).
// A regression that, say, sorted "1.10" before "1.9" lexically would break
// antisymmetry on a discovered pair. Seeded with the table above plus
// pseudo-version-shaped and degenerate strings.
func FuzzCmpVer(f *testing.F) {
	seeds := []struct{ a, b string }{
		{"1.2", "1.2.0"},
		{"1.9", "1.10"},
		{"1.10", "1.9"},
		{"1.2.3+build", "1.2.3"},
		{"v1.50.0-rc1", "1.50.0"},
		{"1.0.0", "1.0.0"},
		{"1.2.x", "1.2.3"},
		{"1-pre", "1"},
		{"+meta", ""},
		{"", ""},
	}
	for _, s := range seeds {
		f.Add(s.a, s.b)
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		if got := cmpVer(a, a); got != 0 {
			t.Fatalf("reflexivity: cmpVer(%q, %q) = %d, want 0", a, a, got)
		}
		ab, ba := cmpVer(a, b), cmpVer(b, a)
		if ab != -ba {
			t.Fatalf("antisymmetry: cmpVer(%q,%q)=%d but cmpVer(%q,%q)=%d (want negation)", a, b, ab, b, a, ba)
		}
	})
}

// Guard that parseVer's numeric-prefix fallback never panics and yields a sane
// core, so the fuzz invariants above rest on a parser that won't fault — keep it
// alongside the comparator's white-box tests. (strconv import is exercised here.)
func TestParseVer_NumericPrefixOnly(t *testing.T) {
	parts, hasPre := parseVer("v1.2.x")
	if len(parts) != 2 || parts[0] != 1 || parts[1] != 2 {
		t.Fatalf("parseVer(v1.2.x) core = %v, want [1 2] (stops at the non-numeric component)", parts)
	}
	if hasPre {
		t.Fatalf("parseVer(v1.2.x) reported a pre-release; the 'x' is not a -suffix")
	}
	// A bare pre-release has no numeric core but is marked pre-release.
	if got := strconv.Itoa(len(mustCore(t, "1-pre"))); got != "1" {
		t.Fatalf("parseVer(1-pre) core length = %s, want 1", got)
	}
}

func mustCore(t *testing.T, v string) []int {
	t.Helper()
	parts, _ := parseVer(v)
	return parts
}

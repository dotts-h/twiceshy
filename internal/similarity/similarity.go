// SPDX-License-Identifier: AGPL-3.0-only

// Package similarity flags authored-record prose that is near-verbatim to a
// supplied reference text — the optional ADR-0011 §5 net (#0090) against a model
// emitting a memorized public snippet while "authoring". It is a LEAD for human
// review, never an auto-reject: cheap word-shingle (n-gram) overlap, stdlib only.
// The primary control is author-from-spec discipline (docs/AUTHORING.md); this is
// an extra net behind it.
package similarity

import (
	"sort"
	"strings"
	"unicode"
)

// DefaultN is the shingle size: five consecutive words is a strong near-verbatim
// signal that still tolerates an incidental shared phrase.
const DefaultN = 5

// words splits text into lowercased tokens — maximal runs of letters/digits — so
// punctuation and whitespace are separators and "foo, foo" tokenizes like "foo foo".
func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// Shingles returns the set of word n-grams in text, each the n words joined by a
// single space. Text with fewer than n words yields the empty set; n < 1 is
// treated as 1.
func Shingles(text string, n int) map[string]struct{} {
	if n < 1 {
		n = 1
	}
	w := words(text)
	out := make(map[string]struct{})
	for i := 0; i+n <= len(w); i++ {
		out[strings.Join(w[i:i+n], " ")] = struct{}{}
	}
	return out
}

// Report is one draft-vs-reference assessment (#0090). Containment is the fraction
// of the draft's distinct shingles that also occur in the reference — the
// plagiarism direction ("how much of the draft is copied"), so a small copied
// passage inside a large reference still scores high (Jaccard would dilute it).
type Report struct {
	N           int
	Containment float64
	Matches     []string // draft shingles found verbatim in the reference, sorted
}

// Assess shingles draft and ref at size n and reports how much of the draft is
// contained in the reference, plus the matching n-gram passages to show a
// reviewer. A draft with fewer than n words reports containment 0 and no matches
// — never a divide by zero. Pure; stdlib only.
func Assess(draft, ref string, n int) Report {
	if n < 1 {
		n = 1
	}
	ds := Shingles(draft, n)
	rep := Report{N: n}
	if len(ds) == 0 {
		return rep
	}
	rs := Shingles(ref, n)
	for s := range ds {
		if _, ok := rs[s]; ok {
			rep.Matches = append(rep.Matches, s)
		}
	}
	rep.Containment = float64(len(rep.Matches)) / float64(len(ds))
	sort.Strings(rep.Matches)
	return rep
}

// Flagged reports whether the containment is at or above threshold. Advisory — the
// caller decides; this is a lead for review, not a gate.
func (r Report) Flagged(threshold float64) bool {
	return r.Containment >= threshold
}

// SPDX-License-Identifier: AGPL-3.0-only

package similarity_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/similarity"
)

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestShinglesWordNGrams(t *testing.T) {
	got := similarity.Shingles("The quick brown fox", 2)
	for _, w := range []string{"the quick", "quick brown", "brown fox"} {
		if _, ok := got[w]; !ok {
			t.Errorf("missing shingle %q in %v", w, keys(got))
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d shingles, want 3: %v", len(got), keys(got))
	}
}

// Tokenization is case-insensitive and splits on punctuation/whitespace, so
// "Foo, foo!  foo foo" is four identical words and its 2-grams dedup to one.
func TestShinglesNormalizeAndDedup(t *testing.T) {
	got := similarity.Shingles("Foo, foo!  foo foo", 2)
	if len(got) != 1 {
		t.Fatalf("want 1 deduped shingle, got %d: %v", len(got), keys(got))
	}
	if _, ok := got["foo foo"]; !ok {
		t.Errorf("want 'foo foo', got %v", keys(got))
	}
}

// Fewer than n words yields no shingles (never a panic or a short gram).
func TestShinglesShortText(t *testing.T) {
	if got := similarity.Shingles("one two", 5); len(got) != 0 {
		t.Errorf("want 0 shingles for sub-n text, got %v", keys(got))
	}
}

func TestAssessIdenticalIsFullyContained(t *testing.T) {
	r := similarity.Assess("alpha beta gamma delta", "alpha beta gamma delta", 2)
	if r.Containment != 1.0 {
		t.Errorf("identical containment = %v, want 1.0", r.Containment)
	}
}

func TestAssessDisjointIsZero(t *testing.T) {
	r := similarity.Assess("alpha beta gamma", "one two three", 2)
	if r.Containment != 0 {
		t.Errorf("disjoint containment = %v, want 0", r.Containment)
	}
	if len(r.Matches) != 0 {
		t.Errorf("disjoint matches = %v, want none", r.Matches)
	}
}

// Containment is draft-in-reference: of the draft's two 2-grams, only "the cat"
// appears in the reference, so containment is 1/2 and the match is surfaced.
func TestAssessPartialOverlapSurfacesMatches(t *testing.T) {
	r := similarity.Assess("the cat sat", "the cat ran away", 2)
	if r.Containment != 0.5 {
		t.Errorf("containment = %v, want 0.5", r.Containment)
	}
	if len(r.Matches) != 1 || r.Matches[0] != "the cat" {
		t.Errorf("matches = %v, want [the cat]", r.Matches)
	}
}

// A single edited word in a long phrase leaves most shingles verbatim — exactly
// the near-verbatim reproduction this is the net for: high containment, flagged.
func TestAssessNearVerbatimFlags(t *testing.T) {
	draft := "to be or not to be that is the question"
	ref := "to be or not to be that is the answer"
	r := similarity.Assess(draft, ref, 3)
	if r.Containment < 0.7 {
		t.Errorf("near-verbatim containment = %v, want >= 0.7", r.Containment)
	}
	if !r.Flagged(0.5) {
		t.Errorf("near-verbatim must flag at 0.5, containment=%v", r.Containment)
	}
}

// Containment must measure the draft against the reference, NOT be diluted by a
// long reference: a fully-copied draft buried in a large public text is still 1.0.
func TestAssessContainmentNotDilutedByReferenceSize(t *testing.T) {
	draft := "memorized snippet from training"
	ref := "lots of unrelated preamble and then memorized snippet from training plus a long tail of more words"
	r := similarity.Assess(draft, ref, 3)
	if r.Containment != 1.0 {
		t.Errorf("containment = %v, want 1.0 (draft fully inside a large reference)", r.Containment)
	}
}

func TestAssessEmptyDraftNoDivideByZero(t *testing.T) {
	r := similarity.Assess("", "anything here at all", 3)
	if r.Containment != 0 {
		t.Errorf("empty-draft containment = %v, want 0", r.Containment)
	}
	if r.Flagged(0.01) {
		t.Error("empty draft must never flag")
	}
}

package idf

import (
	"reflect"
	"testing"
)

// TestTokenize_LowercasesMixedCase verifies Tokenize splits on whitespace
// (strings.Fields) and lowercases every surviving token.
func TestTokenize_LowercasesMixedCase(t *testing.T) {
	got := Tokenize("Hello WORLD FooBar")
	want := []string{"hello", "world", "foobar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(mixed case) = %#v, want %#v", got, want)
	}
}

// TestTokenize_DropsPunctuationOnlyTokens verifies tokens containing no
// letter or digit are dropped entirely, mirroring internal/index's ftsQuery
// hasAlnum filter, while tokens that mix punctuation with an alnum rune
// survive (lowercased, unstripped) — hasAlnum only tests for presence of a
// letter/digit, it does not strip surrounding punctuation.
func TestTokenize_DropsPunctuationOnlyTokens(t *testing.T) {
	got := Tokenize("Hello, --- WORLD! ??? foo123")
	want := []string{"hello,", "world!", "foo123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(punctuation mix) = %#v, want %#v", got, want)
	}
}

// TestTokenize_AllPunctuationYieldsNoTokens verifies an input made entirely
// of punctuation/symbol tokens produces zero output tokens, matching the
// same hasAlnum granularity rule used by internal/index's ftsQuery when
// building its OR-matched FTS5 query from push text.
func TestTokenize_AllPunctuationYieldsNoTokens(t *testing.T) {
	got := Tokenize("!!! ??? ... @#$%")
	if len(got) != 0 {
		t.Fatalf("Tokenize(all punctuation) = %#v, want empty slice", got)
	}
}

// TestTokenize_UnicodeLetterAndDigitEquivalence verifies non-ASCII letters
// and digit-bearing tokens are treated as alnum (kept) just like
// internal/index's hasAlnum, which uses unicode.IsLetter / unicode.IsDigit
// rather than an ASCII-only check.
func TestTokenize_UnicodeLetterAndDigitEquivalence(t *testing.T) {
	got := Tokenize("café €100 ***")
	want := []string{"café", "€100"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize(unicode) = %#v, want %#v", got, want)
	}
}

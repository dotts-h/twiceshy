// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate_CutsByRuneNotByte(t *testing.T) {
	// Multibyte runes: truncating mid-codepoint must never emit invalid UTF-8.
	got := truncate("café—世界テスト", 4)
	if !utf8.ValidString(got) {
		t.Errorf("truncate split a multibyte rune (invalid UTF-8): %q", got)
	}
	if kept := []rune(strings.TrimSuffix(got, "…")); len(kept) != 4 {
		t.Errorf("want 4 runes kept, got %d (%q)", len(kept), got)
	}
	if truncate("ab", 5) != "ab" {
		t.Error("a string shorter than the budget must be returned unchanged")
	}
}

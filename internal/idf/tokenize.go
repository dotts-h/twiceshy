// Package idf provides shared text-tokenization helpers for term-frequency /
// document-frequency style computations.
package idf

import (
	"strings"
	"unicode"
)

// Tokenize splits text on whitespace (strings.Fields), drops any token
// containing no letter or digit — mirroring internal/index's ftsQuery
// hasAlnum filter — and lowercases the surviving tokens. Punctuation
// attached to an alnum rune is preserved (not stripped), only tokens with
// zero letters/digits are dropped entirely.
func Tokenize(text string) []string {
	fields := strings.Fields(text)
	tokens := make([]string, 0, len(fields))
	for _, tok := range fields {
		if !hasAlnum(tok) {
			continue
		}
		tokens = append(tokens, strings.ToLower(tok))
	}
	return tokens
}

func hasAlnum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Package fingerprint implements the normative signature-normalization and
// fingerprinting algorithm from docs/SCHEMA.md ("Fingerprints (normative
// algorithm)"). The same code runs at index time (over a record's
// error_signatures) and at query time (over incoming error text), so
// fingerprint-exact retrieval is a hash-equality lookup.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// The replacement order is normative: UUIDs before long hex runs (a UUID's
// segments would otherwise shred into <hex>/<num> fragments), hex before
// digits, paths after hex (a path may contain a hex run; the path token
// swallows it either way).
var (
	reUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reAddr = regexp.MustCompile(`0x[0-9a-f]+`)
	reHex  = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)
	// Only tokens that *start* like a filesystem path count: identifiers
	// such as modernc.org/sqlite are deliberately not paths.
	rePath = regexp.MustCompile(`(^|\s)(?:~|\.{1,2})?/\S+`)
	// Standalone runs only: digits embedded in identifiers (fts5, utf8,
	// sha256) are discriminative and must survive.
	reNum = regexp.MustCompile(`\b[0-9]+\b`)
	reWS  = regexp.MustCompile(`\s+`)
)

// Normalize canonicalizes an error signature for fingerprinting.
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = reUUID.ReplaceAllString(s, "<uuid>")
	s = reAddr.ReplaceAllString(s, "<addr>")
	s = reHex.ReplaceAllString(s, "<hex>")
	s = rePath.ReplaceAllString(s, "${1}<path>")
	s = reNum.ReplaceAllString(s, "<num>")
	return strings.TrimSpace(reWS.ReplaceAllString(s, " "))
}

// Generic returns the stack-generic fingerprint of a signature: it matches
// across repositories.
func Generic(signature string) string {
	return hash("generic\n" + Normalize(signature))
}

// App returns the repo-specific fingerprint of a signature, scoped to the
// originating repository identifier (e.g. "github.com/dotts-h/twiceshy").
func App(repo, signature string) string {
	return hash("app\n" + repo + "\n" + Normalize(signature))
}

func hash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SPDX-License-Identifier: AGPL-3.0-only

package fingerprint_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/fingerprint"
)

// FuzzNormalizeIdempotent generalizes TestNormalizeIsIdempotent from a single
// hand-picked example to a property: for ANY signature, Normalize must be a
// fixed point after one pass — Normalize(Normalize(x)) == Normalize(x). This
// guards the index-time == query-time invariant the package exists for
// (fingerprint.go:5-7): if normalizing already-normalized text changed it, a
// signature could fingerprint to two different values depending on how many
// times it passed through the pipeline, silently breaking fingerprint-exact
// retrieval.
//
// The seed corpus mixes realistic signatures with the UUID/hex/address/path
// cases from TestNormalize so the property is exercised against every
// replacement rule, including the multi-path ${1} capture-group path.
//
// KNOWN GAP (real product bug, intentionally NOT seeded here): inputs that
// begin with a unicode whitespace char that the regex \s class collapses but
// strings.TrimSpace also strips — notably a vertical tab "\v" — are NOT
// idempotent today. e.g. Normalize("\v/0") == "/<num>", and a second pass
// yields "<path>", because the leading "\v" survives reWS+TrimSpace on the
// first pass (it sits before "/0", so "/0" is not anchored as a path) but is
// trimmed on the second, re-exposing "/<num>" as a "<path>" anchor. Fixing
// that requires a change in fingerprint.go (reconciling the rePath "(^|\s)"
// anchor / reWS \s class with TrimSpace's unicode.IsSpace), which is product
// code and out of scope for this test-hardening pass. Adding "\v/0" (actual
// vertical tab) as a seed here would correctly fail. It is documented rather
// than seeded so this suite stays green; the product fix should add the seed.
func FuzzNormalizeIdempotent(f *testing.F) {
	seeds := []string{
		"",
		"   ",
		"FTS5: Syntax Error",
		`FTS5: syntax error near "." at offset 21`,
		"session 6ba7b810-9dad-11d1-80b4-00c04fd430c8 died",
		"panic at 0xDEADBEEF",
		"commit deadbeefcafe1234 failed",
		"code ab12 ok",
		"open /etc/passwd failed",
		"open /etc/passwd and /var/log/x failed",
		"/usr/bin/x crashed",
		"read ~/.config/app.toml: denied",
		"exec ./run.sh crashed",
		"stat ../secrets env",
		"error near modernc.org/sqlite driver",
		"exit status 137 on line 42",
		"  a \t b\n c  ",
		`panic at 0xDEAD in /usr/lib/thing 42 times, session 6ba7b810-9dad-11d1-80b4-00c04fd430c8`,
		// Adversarial whitespace that the pipeline DOES handle idempotently —
		// trailing/interior \v\f\r runs that reWS collapses to a single space
		// and TrimSpace then removes uniformly on every pass.
		"\f\r  trailing space   \v",
		"path \v\f\r here",
		// Regression (now fixed by the leading TrimSpace): Unicode whitespace the
		// regex \s class does NOT match (vertical tab, NEL, no-break space) leading a
		// /path — previously survived pass 1 then was trimmed, re-anchoring the path.
		"\v/0",
		"\u0085/etc/passwd",
		"\u00a0/var/log/x crashed",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, x string) {
		once := fingerprint.Normalize(x)
		if twice := fingerprint.Normalize(once); twice != once {
			t.Fatalf("Normalize not idempotent for %q: %q -> %q", x, once, twice)
		}
	})
}

// SPDX-License-Identifier: AGPL-3.0-only

package fingerprint_test

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/dotts-h/twiceshy/internal/fingerprint"
)

// TestNormalize pins the normative algorithm from docs/SCHEMA.md
// ("Fingerprints (normative algorithm)"), step by step and in order.
func TestNormalize(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"lowercases", "FTS5: Syntax Error", "fts5: syntax error"},
		{
			"uuid collapses to one token, not hex fragments",
			"session 6ba7b810-9dad-11d1-80b4-00c04fd430c8 died",
			"session <uuid> died",
		},
		{"hex address", "panic at 0xDEADBEEF", "panic at <addr>"},
		{"long hex run", "commit deadbeefcafe1234 failed", "commit <hex> failed"},
		{"digits embedded in identifiers survive", "code ab12 ok", "code ab12 ok"},
		{"absolute path", "open /etc/passwd failed", "open <path> failed"},
		{"home path swallows trailing token chars", "read ~/.config/app.toml: denied", "read <path> denied"},
		{"relative dot path", "exec ./run.sh crashed", "exec <path> crashed"},
		{"parent-relative path", "stat ../secrets env", "stat <path> env"},
		{
			"package identifier is not a path",
			"error near modernc.org/sqlite driver",
			"error near modernc.org/sqlite driver",
		},
		{"digit runs", "exit status 137 on line 42", "exit status <num> on line <num>"},
		{"whitespace collapses and trims", "  a \t b\n c  ", "a b c"},
		{
			"a realistic signature",
			`FTS5: syntax error near "." at offset 21`,
			`fts5: syntax error near "." at offset <num>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fingerprint.Normalize(tt.in); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	in := `panic at 0xDEAD in /usr/lib/thing 42 times, session 6ba7b810-9dad-11d1-80b4-00c04fd430c8`
	once := fingerprint.Normalize(in)
	if twice := fingerprint.Normalize(once); twice != once {
		t.Errorf("not idempotent: %q -> %q", once, twice)
	}
}

func TestFingerprintFormatAndDomainSeparation(t *testing.T) {
	const (
		sig  = `fts5: syntax error near "."`
		repo = "github.com/dotts-h/twiceshy"
	)
	g := fingerprint.Generic(sig)
	a := fingerprint.App(repo, sig)

	format := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	for _, fp := range []string{g, a} {
		if !format.MatchString(fp) {
			t.Errorf("fingerprint %q does not match %s", fp, format)
		}
	}
	if g == a {
		t.Error("app and generic fingerprints must differ (domain separation)")
	}
	if a2 := fingerprint.App("github.com/other/repo", sig); a2 == a {
		t.Error("app fingerprints for different repos must differ")
	}

	// The construction is part of the spec: sha256 over the domain-separated
	// normalized signature.
	sum := sha256.Sum256([]byte("generic\n" + fingerprint.Normalize(sig)))
	if want := "sha256:" + hex.EncodeToString(sum[:]); g != want {
		t.Errorf("Generic(%q) = %s, want %s", sig, g, want)
	}
	sum = sha256.Sum256([]byte("app\n" + repo + "\n" + fingerprint.Normalize(sig)))
	if want := "sha256:" + hex.EncodeToString(sum[:]); a != want {
		t.Errorf("App(%q, %q) = %s, want %s", repo, sig, a, want)
	}
}

// Index-time and query-time fingerprints must agree across cosmetic
// variation — that equality is the whole point of normalization.
func TestFingerprintMatchesAcrossCosmeticVariation(t *testing.T) {
	stored := fingerprint.Generic(`fts5: syntax error near "." at offset 21`)
	queried := fingerprint.Generic(`FTS5:  syntax error near "."   at offset 7`)
	if stored != queried {
		t.Error("cosmetically different signatures should fingerprint identically")
	}
}

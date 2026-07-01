// SPDX-License-Identifier: AGPL-3.0-only

package fingerprint_test

import (
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
		{
			// Two paths in one signature: the rePath capture group + ${1}
			// replacement must preserve the whitespace separator between
			// them, not merge the two tokens.
			"two absolute paths keep their separator",
			"open /etc/passwd and /var/log/x failed",
			"open <path> and <path> failed",
		},
		{"path at line start", "/usr/bin/x crashed", "<path> crashed"},
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

// Regression: unicode whitespace that strings.TrimSpace strips but the regex \s
// class does NOT match (vertical tab, NEL, no-break space). A leading one of these
// survived the first pass but was trimmed by the final TrimSpace, re-exposing a
// /path token at ^ on the second pass — breaking idempotence.
func TestNormalizeIdempotentOnLeadingUnicodeWhitespace(t *testing.T) {
	for _, in := range []string{"\v/0", "\u0085/etc/passwd", "\u00a0/var/log/x crashed"} {
		once := fingerprint.Normalize(in)
		if twice := fingerprint.Normalize(once); twice != once {
			t.Errorf("not idempotent for %q: %q -> %q", in, once, twice)
		}
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
	// normalized signature (docs/SCHEMA.md). Pinned literals, verified once
	// against sha256("generic\n"+Normalize(sig)) / sha256("app\n"+repo+"\n"+
	// Normalize(sig)) — recomputing the same formula here would let a shared
	// bug in both sides cancel out.
	const (
		wantGeneric = "sha256:db700532dd2f92ca0652919910c384609f3909ad59fb95ac6344908c209ce8b2"
		wantApp     = "sha256:c200471236ddac7205af32e693e1ba8c3898003b26539a731558e3f93139054d"
	)
	if g != wantGeneric {
		t.Errorf("Generic(%q) = %s, want %s", sig, g, wantGeneric)
	}
	if a != wantApp {
		t.Errorf("App(%q, %q) = %s, want %s", repo, sig, a, wantApp)
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

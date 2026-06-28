// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunScreen_FlagsSecretAndExitsNonZero(t *testing.T) {
	// Secret-shaped value assembled at run time, never a literal token in any commit
	// (CONVENTIONS: gitleaks scans the whole history).
	secret := "AKIA" + strings.Repeat("A", 16)
	var buf bytes.Buffer
	if err := runScreen(nil, strings.NewReader("agent printed "+secret), &buf); err == nil {
		t.Fatal("want a non-nil error (non-zero exit) when a secret is present")
	}
	// Pin the EXACT detector, not just the "secret:" prefix: an AKIA key must flag
	// under aws-access-key (screen rule, screen.go:37). Asserting the full
	// "category:rule" line means a regex that stops matching AKIA keys — or relabels
	// them under another secret rule (e.g. assigned-high-entropy) — fails here
	// instead of silently passing on any-secret-found.
	if !strings.Contains(buf.String(), "secret:aws-access-key") {
		t.Errorf("want the AWS key flagged as secret:aws-access-key; got %q", buf.String())
	}
}

// runScreen blocks ONLY on a secret. harmful-code is documented (screen.go doc,
// internal/screen.HasSecret) as "flag but MUST NOT block" — a shell snippet like a
// pipe-to-installer is legitimately present in a coding transcript, so the
// SessionEnd hook must not reject it. A regression that made harmful-code blocking
// would break that hook and ship green without this guard.
func TestRunScreen_HarmfulCodeFlagsButDoesNotBlock(t *testing.T) {
	// Assembled at run time so no harmful literal lands in a commit. The URL has no
	// '@' and no RFC-1918 IP, so harmful-code:pipe-to-shell is the ONLY finding —
	// the nil-return assertion is unambiguous (no incidental secret to block on).
	snippet := "to bootstrap, the agent ran curl " + "https://example.com/install | sh"
	var buf bytes.Buffer
	if err := runScreen(nil, strings.NewReader(snippet), &buf); err != nil {
		t.Errorf("harmful-code must flag but not block (exit zero); got %v", err)
	}
	if !strings.Contains(buf.String(), "harmful-code:") {
		t.Errorf("want the harmful-code snippet flagged; got %q", buf.String())
	}
}

func TestRunScreen_CleanInputExitsZero(t *testing.T) {
	var buf bytes.Buffer
	if err := runScreen(nil, strings.NewReader("a normal transcript about fts5 quoting"), &buf); err != nil {
		t.Errorf("clean input must exit zero; got %v", err)
	}
}

// harmful-code / pii flag but must NOT block — they are expected in a coding
// transcript (private IPs, shell snippets), mirroring the /retro endpoint policy.
func TestRunScreen_NonSecretFindingsDoNotBlock(t *testing.T) {
	var buf bytes.Buffer
	if err := runScreen(nil, strings.NewReader("ran the build against 192.168.50.244"), &buf); err != nil {
		t.Errorf("a private IP must flag but not block; got %v", err)
	}
	if !strings.Contains(buf.String(), "pii:") {
		t.Errorf("want the private IP flagged; got %q", buf.String())
	}
}

func TestRunScreen_RedactsPII(t *testing.T) {
	ip := "10.1.2.3"
	email := "a@b.com"
	var buf bytes.Buffer
	if err := runScreen([]string{"-redact"}, strings.NewReader("contact "+email+" at "+ip), &buf); err != nil {
		t.Fatalf("redacting PII: %v", err)
	}
	want := "contact <REDACTED-EMAIL> at <REDACTED-IP>"
	if buf.String() != want {
		t.Errorf("redacted output = %q, want %q", buf.String(), want)
	}
	if strings.Contains(buf.String(), ip) || strings.Contains(buf.String(), email) {
		t.Errorf("redacted output leaked raw PII: %q", buf.String())
	}
}

func TestRunScreen_RedactWithSecretDoesNotEmitBody(t *testing.T) {
	secret := "AKIA" + strings.Repeat("A", 16)
	body := "contact a@b.com at 10.1.2.3 with key " + secret
	var buf bytes.Buffer
	if err := runScreen([]string{"-redact"}, strings.NewReader(body), &buf); err == nil {
		t.Fatal("want a non-nil error when redact input contains a secret")
	}
	if strings.Contains(buf.String(), "<REDACTED-") || strings.Contains(buf.String(), secret) {
		t.Errorf("redact mode emitted body when a secret was present: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "secret:aws-access-key") {
		t.Errorf("want the AWS key flagged as secret:aws-access-key; got %q", buf.String())
	}
}

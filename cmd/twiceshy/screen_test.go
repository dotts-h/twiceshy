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
	if !strings.Contains(buf.String(), "secret:") {
		t.Errorf("want a secret flag printed; got %q", buf.String())
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

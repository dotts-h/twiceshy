// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSimRecord writes a valid quarantined authored record with distinctive prose
// at a real experience/ path under a temp dir, and returns its absolute path.
func writeSimRecord(t *testing.T) string {
	t.Helper()
	const md = `---
schema_version: 1
id: exp-0090
kind: trap
status: quarantined
title: a distinctive authored trap about quasar buffers
symptom:
  summary: the quasar buffer overflows when the flux capacitor desyncs mid tick
applies_to:
  - ecosystem: Go
    package: example.com/x
resolution:
  root_cause: "the desync arises because the flux capacitor latch resets on every single tick"
  fix: "pin the latch high and drain the quasar buffer before the desync window opens"
provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-23
  valid: { from: 2026-06-23, until: null }
---

A narrative about quasar buffers and flux capacitor desync, written from scratch.
`
	dir := t.TempDir()
	recPath := filepath.Join(dir, "experience", "2026", "0090-quasar-buffer-desync.md")
	if err := os.MkdirAll(filepath.Dir(recPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return recPath
}

func writeRef(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ref.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A reference reproducing the record's prose near-verbatim must be FLAGGED as a
// lead — the §5 reproduction risk the check exists for.
func TestSimilarityFlagsNearVerbatimReference(t *testing.T) {
	rec := writeSimRecord(t)
	ref := writeRef(t, "Some unrelated preamble. "+
		"the quasar buffer overflows when the flux capacitor desyncs mid tick. "+
		"the desync arises because the flux capacitor latch resets on every single tick. "+
		"pin the latch high and drain the quasar buffer before the desync window opens. Trailing text.")

	var buf bytes.Buffer
	if err := runSimilarity([]string{"-record", rec, "-against", ref}, &buf); err != nil {
		t.Fatalf("runSimilarity: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "FLAGGED") {
		t.Errorf("near-verbatim reference must be FLAGGED, got:\n%s", out)
	}
	if !strings.Contains(out, "near-verbatim overlap found") {
		t.Errorf("missing the lead summary, got:\n%s", out)
	}
}

// An unrelated reference must read clean — no false lead.
func TestSimilarityCleanOnUnrelatedReference(t *testing.T) {
	rec := writeSimRecord(t)
	ref := writeRef(t, "How to center a div in CSS and other entirely unrelated frontend advice.")

	var buf bytes.Buffer
	if err := runSimilarity([]string{"-record", rec, "-against", ref}, &buf); err != nil {
		t.Fatalf("runSimilarity: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "FLAGGED") {
		t.Errorf("unrelated reference must not be flagged, got:\n%s", out)
	}
	if !strings.Contains(out, "no near-verbatim overlap") {
		t.Errorf("missing the clean summary, got:\n%s", out)
	}
}

// -against is repeatable; one flagged among several makes the overall verdict a lead.
func TestSimilarityRepeatableAgainst(t *testing.T) {
	rec := writeSimRecord(t)
	clean := writeRef(t, "totally unrelated text about gardening and compost")
	dirty := writeRef(t, "the desync arises because the flux capacitor latch resets on every single tick")

	var buf bytes.Buffer
	if err := runSimilarity([]string{"-record", rec, "-against", clean, "-against", dirty}, &buf); err != nil {
		t.Fatalf("runSimilarity: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "FLAGGED") || !strings.Contains(out, "near-verbatim overlap found") {
		t.Errorf("a flagged reference among several must surface as a lead, got:\n%s", out)
	}
}

func TestSimilarityRequiresFlags(t *testing.T) {
	rec := writeSimRecord(t)
	ref := writeRef(t, "x")
	if err := runSimilarity([]string{"-against", ref}, &bytes.Buffer{}); err == nil {
		t.Error("missing -record must error")
	}
	if err := runSimilarity([]string{"-record", rec}, &bytes.Buffer{}); err == nil {
		t.Error("missing -against must error")
	}
}

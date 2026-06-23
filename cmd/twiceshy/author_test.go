// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// author writes a parseable, fillable skeleton under -corpus and reports it.
func TestAuthorWritesParseableSkeleton(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := runAuthor([]string{
		"-corpus", dir, "-id", "exp-0091", "-slug", "quasar-buffer-desync",
		"-title", "the quasar buffer overflows when the flux capacitor desyncs",
	}, &out)
	if err != nil {
		t.Fatalf("runAuthor: %v", err)
	}

	// The record landed at its corpus path and parses as a valid quarantined draft.
	recRel := filepath.Join("experience", "2026", "0091-quasar-buffer-desync.md")
	data, err := os.ReadFile(filepath.Join(dir, recRel))
	if err != nil {
		t.Fatalf("record not written: %v", err)
	}
	rec, err := record.Parse(filepath.ToSlash(recRel), data)
	if err != nil {
		t.Fatalf("written record must parse: %v", err)
	}
	if rec.Status != "quarantined" || rec.Provenance.SourceLicense != record.SourceLicenseAuthoredInternal {
		t.Errorf("want quarantined + authored-internal, got status=%q license=%q", rec.Status, rec.Provenance.SourceLicense)
	}

	// The positive repro skeleton was written and is executable.
	reproPath := filepath.Join(dir, "experience", "repro", "0091-quasar-buffer-desync.sh")
	fi, err := os.Stat(reproPath)
	if err != nil {
		t.Fatalf("repro not written: %v", err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("repro skeleton must be executable, mode=%v", fi.Mode())
	}
	if !strings.Contains(out.String(), "wrote experience/2026/0091-quasar-buffer-desync.md") {
		t.Errorf("output should report what it wrote, got:\n%s", out.String())
	}
}

// Re-running over an existing record refuses to overwrite (all-or-nothing).
func TestAuthorRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	args := []string{"-corpus", dir, "-id", "exp-0091", "-slug", "my-trap",
		"-title", "a sufficiently long trap title here"}
	if err := runAuthor(args, &bytes.Buffer{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	err := runAuthor(args, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second run must refuse to overwrite, got %v", err)
	}
}

func TestAuthorRejectsBadInvocation(t *testing.T) {
	// missing required slug/title -> Scaffold errors, nothing written.
	if err := runAuthor([]string{"-corpus", t.TempDir(), "-id", "exp-0091"}, &bytes.Buffer{}); err == nil {
		t.Error("missing slug/title must error")
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func TestSafeJoin(t *testing.T) {
	base := filepath.Join(t.TempDir(), "corpus")
	ok := []string{"experience/2026/0001-x.md", "a/b/c.md", "x.md"}
	for _, rel := range ok {
		if _, err := safeJoin(base, rel); err != nil {
			t.Errorf("safeJoin(%q) should be allowed: %v", rel, err)
		}
	}
	bad := []string{"../escape.md", "../../etc/passwd", "a/../../b.md", "/etc/passwd"}
	for _, rel := range bad {
		got, err := safeJoin(base, rel)
		if err == nil {
			t.Errorf("safeJoin(%q) must be refused, got %q", rel, got)
		}
		if strings.Contains(err.Error(), base) {
			t.Errorf("safeJoin error must not leak the absolute base: %v", err)
		}
	}
}

func TestWriteRecordRefusesEscapingPath(t *testing.T) {
	corpus := t.TempDir()
	rec := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0001",
		Kind:          "convention",
		Status:        "quarantined",
		Title:         "escaping path attempt that is long enough",
		AppliesTo:     []record.AppliesTo{{Ecosystem: "Go"}},
		Provenance: record.Provenance{
			Source:     record.Source{Author: "x"},
			RecordedAt: "2026-06-18",
			Valid:      record.Validity{From: "2026-06-18"},
		},
		Body: "body",
		Path: "../escape.md",
	}
	if err := writeRecord(corpus, rec); err == nil {
		t.Fatal("writeRecord must refuse a path that escapes the corpus root")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(corpus), "escape.md")); err == nil {
		t.Fatal("escaping record was written outside the corpus root")
	}
}

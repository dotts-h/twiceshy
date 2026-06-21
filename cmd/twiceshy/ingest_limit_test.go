// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// osv-live is a wired ingest source. NewOSVLiveSource performs no network I/O
// until Drafts() is called, so resolving + naming it needs no fixture.
func TestImportSourceOSVLive(t *testing.T) {
	src, err := importSource("osv-live", "")
	if err != nil {
		t.Fatalf("osv-live must be a known source: %v", err)
	}
	if src.Name() != "osv-live" {
		t.Errorf("Name() = %q, want osv-live", src.Name())
	}
}

// -limit bounds how many new records a single import writes, so a scheduled
// import grows the corpus gradually instead of dumping the whole feed at once.
// Uses the embedded "go" source (no network) which yields more than one record.
func TestRunIngestLimitBounds(t *testing.T) {
	dir := tempCorpus(t)
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-limit", "1"}, &out, noEnv)
	if err != nil {
		t.Fatalf("ingest go -limit 1: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "experience", "2026", "*.md"))
	if len(matches) != 1 {
		t.Fatalf("-limit 1 must write exactly 1 record, got %d: %v", len(matches), matches)
	}
	if !strings.Contains(out.String(), "created 1") {
		t.Errorf("output = %q", out.String())
	}
}

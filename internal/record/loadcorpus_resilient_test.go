// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// LoadCorpusResilient is the RUN-path loader (#0053, §D3): a single poison /
// unparseable record must NOT kill the whole promote/adapt run — it is skipped
// and reported, and the survivors load. The strict LoadCorpus stays fatal-on-bad
// for index/serve/doctor/CI.

func writeCorpusFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCorpusResilient_SkipsPoisonRecord(t *testing.T) {
	root := t.TempDir()

	good := importerDraft() // provenance_license_test.go (same package)
	md, err := record.Marshal(good)
	if err != nil {
		t.Fatalf("marshal good record: %v", err)
	}
	writeCorpusFile(t, root, good.Path, md)
	writeCorpusFile(t, root, "experience/2026/9999-poison.md", []byte("this is not a record: {{{ broken frontmatter"))

	recs, skipped, err := record.LoadCorpusResilient(root)
	if err != nil {
		t.Fatalf("a poison record must not make the resilient load fatal: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != good.ID {
		t.Fatalf("survivors = %v, want exactly the good record %s", recs, good.ID)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "9999-poison.md") {
		t.Fatalf("skipped = %v, want the poison file reported", skipped)
	}
}

func TestLoadCorpusResilient_AllGoodNoneSkipped(t *testing.T) {
	root := t.TempDir()
	good := importerDraft()
	md, _ := record.Marshal(good)
	writeCorpusFile(t, root, good.Path, md)

	recs, skipped, err := record.LoadCorpusResilient(root)
	if err != nil {
		t.Fatalf("LoadCorpusResilient: %v", err)
	}
	if len(recs) != 1 || len(skipped) != 0 {
		t.Fatalf("clean corpus: recs=%d skipped=%d, want 1/0", len(recs), len(skipped))
	}
}

// A missing/unreadable corpus tree is still fatal — that is not a single poison
// record, it means there is nothing to run against.
func TestLoadCorpusResilient_MissingCorpusIsFatal(t *testing.T) {
	_, _, err := record.LoadCorpusResilient(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("a missing corpus tree must be a fatal error, not a silent empty load")
	}
}

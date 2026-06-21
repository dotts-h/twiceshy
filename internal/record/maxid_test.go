// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// MaxID reads the highest exp-NNNN id from the source-of-truth corpus tree on
// disk (the canonical record-path layout), not the possibly-stale index — the
// robustness #0059 needs. It scans paths only, so a malformed record body can
// never break id allocation.
func TestMaxID(t *testing.T) {
	t.Run("missing corpus is zero", func(t *testing.T) {
		got, err := record.MaxID(t.TempDir()) // no experience/ subtree
		if err != nil {
			t.Fatalf("MaxID: %v", err)
		}
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("max across years, ignoring non-record files", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "experience/2025/0001-alpha.md")
		writeFile(t, root, "experience/2026/0016-bravo-vault.md")
		writeFile(t, root, "experience/2026/0042-charlie.md")
		writeFile(t, root, "experience/2026/0042-charlie.repro.sh") // repro script, not a record
		writeFile(t, root, "experience/2026/README.md")             // scratch doc
		writeFile(t, root, "experience/notes.md")                   // wrong depth
		got, err := record.MaxID(root)
		if err != nil {
			t.Fatalf("MaxID: %v", err)
		}
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})

	t.Run("ids wider than four digits", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "experience/2026/0099-small.md")
		writeFile(t, root, "experience/2026/01234-big.md")
		got, err := record.MaxID(root)
		if err != nil {
			t.Fatalf("MaxID: %v", err)
		}
		if got != 1234 {
			t.Errorf("got %d, want 1234", got)
		}
	})
}

func writeFile(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

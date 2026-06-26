// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
)

// ingest.NextID allocates the next exp-NNNN id robustly against a live index
// that has drifted behind the committed corpus: it returns one past the max of
// the index's view and the on-disk corpus tree, so a stale index can never hand
// back an id that already exists on disk (#0059). It does not address the
// concurrency hazard — two simultaneous callers can still observe the same max
// (TECH_DEBT M3); the propose-only write path catches that in PR review.
func TestNextID(t *testing.T) {
	ctx := context.Background()

	t.Run("disk ahead of a stale index wins (regression #0059)", func(t *testing.T) {
		root := t.TempDir()
		writeDisk(t, root, "experience/2026/0016-stale-drift.md")
		ix := openIx(t) // empty index → its NextID is the stale exp-0001
		got, err := ingest.NextID(ctx, ix, root)
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0017" {
			t.Errorf("got %q, want exp-0017 (one past disk max, not the stale index's exp-0001)", got)
		}
	})

	t.Run("index ahead of disk wins", func(t *testing.T) {
		root := t.TempDir()
		writeDisk(t, root, "experience/2026/0016-on-disk.md")
		ix := openIx(t, mkRec(t, "0042", "forty two", "the forty-second record body text", "ERR-42"))
		got, err := ingest.NextID(ctx, ix, root)
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0043" {
			t.Errorf("got %q, want exp-0043", got)
		}
	})

	t.Run("empty corpus root falls back to the index", func(t *testing.T) {
		ix := openIx(t, mkRec(t, "0007", "seventh record", "the seventh record body text here", "ERR-7"))
		got, err := ingest.NextID(ctx, ix, "")
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0008" {
			t.Errorf("got %q, want exp-0008", got)
		}
	})

	t.Run("empty index and empty corpus start at exp-0001", func(t *testing.T) {
		got, err := ingest.NextID(ctx, openIx(t), t.TempDir())
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0001" {
			t.Errorf("got %q, want exp-0001", got)
		}
	})
}

func TestNextID_UsesBaseRefMaxWhenLocalIsStale(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	gitNextID(t, repo, "init", "-q")
	gitNextID(t, repo, "config", "user.email", "test@example.com")
	gitNextID(t, repo, "config", "user.name", "Test User")
	writeDisk(t, repo, "experience/2026/2768-base.md")
	gitNextID(t, repo, "add", ".")
	gitNextID(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitNextID(t, repo, "rev-parse", "HEAD"))

	if err := os.Remove(filepath.Join(repo, "experience/2026/2768-base.md")); err != nil {
		t.Fatal(err)
	}
	writeDisk(t, repo, "experience/2026/2758-local.md")
	got, err := ingest.NextIDWithBase(ctx, openIx(t), repo, base)
	if err != nil {
		t.Fatalf("NextIDWithBase: %v", err)
	}
	if got != "exp-2769" {
		t.Fatalf("got %q, want exp-2769", got)
	}
}

func writeDisk(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitNextID(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

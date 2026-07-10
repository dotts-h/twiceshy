// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// #0150: the live MCP write allocator must use the same external high-water
// floor as the batch allocators. The resolver is cached because record_experience
// and report_outcome share this hot path and must not scan Forgejo on every call.
func TestAllocateNextIDUsesCachedExternalFloor(t *testing.T) {
	h, _ := newUsageHandlers(t)
	var calls atomic.Int32
	h.idFloorResolver = func(context.Context) (int, error) {
		calls.Add(1)
		return 4546, nil
	}
	h.idFloorTTL = time.Minute
	h.idNow = func() time.Time { return time.Unix(100, 0) }

	id1, err := h.allocateNextID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id2, err := h.allocateNextID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id1 != "exp-4547" || id2 != "exp-4548" {
		t.Fatalf("ids = %q, %q; want exp-4547, exp-4548", id1, id2)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("floor resolver calls = %d, want 1 within TTL", got)
	}
}

func TestLiveWriteToolsShareMergeSafeFloor(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.idFloorResolver = func(context.Context) (int, error) { return 4546, nil }
	h.idFloorTTL = time.Minute

	_, proposed, err := h.record(context.Background(), nil, RecordArgs{
		Kind: "trap", Title: "Novel live floor allocation collision guard",
		Summary: "a unique live allocation symptom", ErrorSignatures: []string{"live-floor-unique-4551"},
		RootCause: "open PR ids were not visible", Fix: "allocate above the open PR floor",
		Body: "A distinct narrative for the server allocation regression.", Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("record_experience: %v", err)
	}
	_, reported, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "the served lesson still failed", Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("report_outcome: %v", err)
	}
	if proposed.RecordID != "exp-4547" || reported.RecordID != "exp-4548" {
		t.Fatalf("record_experience/report_outcome ids = %q/%q, want exp-4547/exp-4548", proposed.RecordID, reported.RecordID)
	}
}

// A Forgejo blip must not reject a propose-only write. Allocation falls back
// to the configured merge base plus the local tree/index and logs loudly.
func TestAllocateNextIDForgeFailureFallsBackToBaseAndLogs(t *testing.T) {
	h, _ := newUsageHandlers(t)
	repo := gitRepoWithBaseRecord(t, "0300")
	var logs bytes.Buffer
	h.corpus = repo
	h.idBaseRef = "allocation-base"
	h.idFloorResolver = func(context.Context) (int, error) { return 0, errors.New("forge unavailable") }
	h.idFloorTTL = time.Minute
	h.idNow = func() time.Time { return time.Unix(100, 0) }
	h.logger = slog.New(slog.NewTextHandler(&logs, nil))

	id, err := h.allocateNextID(context.Background())
	if err != nil {
		t.Fatalf("Forgejo failure must degrade to base/local allocation: %v", err)
	}
	if id != "exp-0301" {
		t.Fatalf("id = %q, want exp-0301 from configured base", id)
	}
	if got := logs.String(); !strings.Contains(got, "forge unavailable") || !strings.Contains(got, "ID floor") {
		t.Fatalf("fallback must log the floor failure loudly; log=%q", got)
	}
}

func gitRepoWithBaseRecord(t *testing.T, id string) string {
	t.Helper()
	repo := t.TempDir()
	runGitAllocation(t, repo, "init", "-q")
	runGitAllocation(t, repo, "config", "user.email", "test@example.com")
	runGitAllocation(t, repo, "config", "user.name", "Test User")
	p := filepath.Join(repo, "experience", "2026", id+"-base.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("base allocation fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitAllocation(t, repo, "add", ".")
	runGitAllocation(t, repo, "commit", "-qm", "base")
	runGitAllocation(t, repo, "branch", "allocation-base")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	return repo
}

func runGitAllocation(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

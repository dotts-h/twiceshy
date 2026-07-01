// SPDX-License-Identifier: AGPL-3.0-only

package spool_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/spool"
)

func TestEnqueueListReadRemove_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "queue")
	r := spool.Report{RecordID: "exp-0200", Outcome: "failed", Evidence: "boom", Author: "agent-x", ReportedAt: "2026-06-19T12:00:00Z"}
	if _, err := spool.Enqueue(dir, r); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	paths, err := spool.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 queued report, got %d", len(paths))
	}

	got, err := spool.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != r {
		t.Fatalf("round-trip lost data: got %+v want %+v", got, r)
	}

	if err := spool.Remove(paths[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if paths, _ := spool.List(dir); len(paths) != 0 {
		t.Fatalf("queue not drained: %v", paths)
	}
}

// A missing queue directory is an empty queue, not an error (a fresh deploy).
func TestList_MissingDirIsEmpty(t *testing.T) {
	paths, err := spool.List(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("List of a missing dir must be empty, not error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("want empty, got %v", paths)
	}
}

// Two reports queue as two distinct entries (no filename collision).
func TestEnqueue_TwoReportsTwoEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "queue")
	for i := 0; i < 2; i++ {
		if _, err := spool.Enqueue(dir, spool.Report{RecordID: "exp-0200", Outcome: "failed", Author: "a", ReportedAt: "2026-06-19T12:00:00Z"}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	paths, _ := spool.List(dir)
	if len(paths) != 2 {
		t.Fatalf("two reports must produce two queue entries, got %d", len(paths))
	}
}

// sanitize must produce a filesystem-safe filename prefix: a ReportedAt timestamp
// like "2026-06-19T12:00:00Z" carries ':' (and could carry '/', '\\', ' ') which
// break on some filesystems. A regression leaving any of those in the prefix would
// otherwise pass — this pins the contract and the post-sanitize round-trip.
func TestEnqueue_SanitizesPrefixForFilesystemSafety(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "queue")
	r := spool.Report{RecordID: "exp-0200", Outcome: "failed", Author: "a", ReportedAt: "2026-06-19T12:00:00Z"}
	if _, err := spool.Enqueue(dir, r); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	paths, err := spool.List(dir)
	if err != nil || len(paths) != 1 {
		t.Fatalf("List: %v paths=%v", err, paths)
	}
	base := filepath.Base(paths[0])
	if strings.ContainsAny(base, ":/\\ ") {
		t.Fatalf("prefix not filesystem-safe: %q contains a forbidden char (sanitize regressed)", base)
	}
	if got, err := spool.Read(paths[0]); err != nil || got != r {
		t.Fatalf("round-trip after sanitize: got %+v err %v want %+v", got, err, r)
	}
}

// A corrupt queue entry must surface a decode error, not a silently zeroed
// Report — the intake drivers rely on this to skip-and-log rather than
// misprocess a malformed file (#0042).
func TestRead_MalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := spool.Read(path)
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("Read of malformed JSON: got %v, want *json.SyntaxError", err)
	}
}

// A missing queue entry (already drained, or a bad path) surfaces the
// filesystem error rather than a zero-value Report.
func TestRead_MissingFileErrors(t *testing.T) {
	_, err := spool.Read(filepath.Join(t.TempDir(), "nope.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read of a missing file: got %v, want os.ErrNotExist", err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package spool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnqueueReadTranscript_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	tr := Transcript{
		SessionID:  "sess-123",
		Author:     "claude",
		Reason:     "logout",
		Transcript: "agent hit fts5: syntax error on a dotted token modernc.org/sqlite",
		CapturedAt: "2026-06-22T10:00:00Z",
	}

	path, err := EnqueueTranscript(dir, tr)
	if err != nil {
		t.Fatalf("EnqueueTranscript: %v", err)
	}
	if got := filepath.Dir(path); got != dir {
		t.Errorf("enqueued path %q not under dir %q", path, dir)
	}

	files, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("List = %d files, want 1", len(files))
	}

	got, err := ReadTranscript(files[0])
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if got != tr {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, tr)
	}

	if err := Remove(files[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if files, _ := List(dir); len(files) != 0 {
		t.Errorf("after Remove, List = %d files, want 0", len(files))
	}
}

// Transcript and Report queues are decoupled: a Report file must not decode as a
// Transcript with content (the intake drivers point at separate dirs, but a
// misroute must not silently produce an empty-bodied draft).
func TestReadTranscript_OnReportEntry_HasNoBody(t *testing.T) {
	dir := t.TempDir()
	if _, err := Enqueue(dir, Report{RecordID: "exp-0001", Outcome: "failed", Author: "x", ReportedAt: "2026-06-22T10:00:00Z"}); err != nil {
		t.Fatalf("Enqueue report: %v", err)
	}
	files, _ := List(dir)
	got, err := ReadTranscript(files[0])
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if got.Transcript != "" {
		t.Errorf("report misrouted as transcript decoded a body %q, want empty (driver skips empties)", got.Transcript)
	}
}

// A corrupt transcript entry must surface a decode error, not a silently
// zeroed Transcript — retro-intake relies on this to skip-and-log.
func TestReadTranscript_MalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ReadTranscript(path)
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("ReadTranscript of malformed JSON: got %v, want *json.SyntaxError", err)
	}
}

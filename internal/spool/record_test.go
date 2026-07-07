// SPDX-License-Identifier: AGPL-3.0-only

package spool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnqueueReadRecord_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	d := RecordDraft{
		Kind:            "advisory",
		Title:           "FTS5 dot token issue",
		Summary:         "a summary of the issue",
		ErrorSignatures: []string{"syntax error near \".\""},
		Ecosystem:       "go",
		Package:         "modernc.org/sqlite",
		RootCause:       "broken parser",
		Fix:             "update parser",
		GuardingTest:    "TestParser",
		Body:            "full explanation",
		Author:          "test-author",
		Session:         "test-sess",
		ReportedAt:      "2026-07-07T19:22:00Z",
	}

	path, err := EnqueueRecord(dir, d)
	if err != nil {
		t.Fatalf("EnqueueRecord: %v", err)
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

	got, err := ReadRecord(files[0])
	if err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}
	// Compare slices and fields
	if got.Kind != d.Kind || got.Title != d.Title || got.Summary != d.Summary ||
		len(got.ErrorSignatures) != len(d.ErrorSignatures) || got.ErrorSignatures[0] != d.ErrorSignatures[0] ||
		got.Ecosystem != d.Ecosystem || got.Package != d.Package || got.RootCause != d.RootCause ||
		got.Fix != d.Fix || got.GuardingTest != d.GuardingTest || got.Body != d.Body ||
		got.Author != d.Author || got.Session != d.Session || got.ReportedAt != d.ReportedAt {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, d)
	}

	if err := Remove(files[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if files, _ := List(dir); len(files) != 0 {
		t.Errorf("after Remove, List = %d files, want 0", len(files))
	}
}

func TestReadRecord_OnReportEntry_HasNoTitle(t *testing.T) {
	dir := t.TempDir()
	if _, err := Enqueue(dir, Report{RecordID: "exp-0001", Outcome: "failed", Author: "x", ReportedAt: "2026-06-22T10:00:00Z"}); err != nil {
		t.Fatalf("Enqueue report: %v", err)
	}
	files, _ := List(dir)
	got, err := ReadRecord(files[0])
	if err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}
	if got.Title != "" {
		t.Errorf("report misrouted as record draft decoded a title %q, want empty", got.Title)
	}
}

func TestReadRecord_MalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ReadRecord(path)
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("ReadRecord of malformed JSON: got %v, want *json.SyntaxError", err)
	}
}

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

func TestEnqueueReadIssue_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	iss := spool.Issue{
		Title:           "fts5 dotted token misparse",
		Description:     "a package path like modernc.org/sqlite trips the tokenizer",
		Category:        "bug",
		RelatedRecordID: "exp-0001",
		Author:          "claude",
		Session:         "sess-123",
		ReportedAt:      "2026-06-22T10:00:00Z",
	}

	path, err := spool.EnqueueIssue(dir, iss)
	if err != nil {
		t.Fatalf("EnqueueIssue: %v", err)
	}

	files, err := spool.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Fatalf("List = %v, want exactly [%q]", files, path)
	}

	got, err := spool.ReadIssue(path)
	if err != nil {
		t.Fatalf("ReadIssue: %v", err)
	}
	if got != iss {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, iss)
	}

	if err := spool.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if files, _ := spool.List(dir); len(files) != 0 {
		t.Errorf("after Remove, List = %v, want empty", files)
	}
}

// Issue and Report queues are decoupled: a Report file must not decode as an
// Issue with a title — the same misroute guard as TestReadTranscript_OnReportEntry_HasNoBody.
func TestReadIssue_OnReportEntry_HasNoTitle(t *testing.T) {
	dir := t.TempDir()
	if _, err := spool.Enqueue(dir, spool.Report{RecordID: "exp-0001", Outcome: "failed", Author: "x", ReportedAt: "2026-06-22T10:00:00Z"}); err != nil {
		t.Fatalf("Enqueue report: %v", err)
	}
	files, _ := spool.List(dir)
	got, err := spool.ReadIssue(files[0])
	if err != nil {
		t.Fatalf("ReadIssue: %v", err)
	}
	if got.Title != "" {
		t.Errorf("report misrouted as issue decoded a title %q, want empty", got.Title)
	}
}

// A corrupt issue entry must surface a decode error, not a silently zeroed
// Issue — intake-issues relies on this to skip-and-log rather than materialize
// a blank docs/issues file (#0066).
func TestReadIssue_MalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := spool.ReadIssue(path)
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("ReadIssue of malformed JSON: got %v, want *json.SyntaxError", err)
	}
}

// RenderBody's optional clauses (session, related record, security flags) are
// each independently gated — the #0066/#0075 shared-renderer contract that both
// report_issue paths (server no-queue fallback, intake-issues drainer) rely on.
func TestIssue_RenderBody(t *testing.T) {
	base := spool.Issue{
		Description: "  a description with padding  ",
		Category:    "bug",
		Author:      "claude",
	}
	cases := []struct {
		name    string
		issue   spool.Issue
		flags   []string
		want    []string // substrings that must be present
		missing []string // substrings that must be absent
	}{
		{
			name:    "minimal",
			issue:   base,
			want:    []string{"## Summary\n", "a description with padding", "category: bug", "by claude"},
			missing: []string{"session", "Related record", "SECURITY flags"},
		},
		{
			name: "with session",
			issue: func() spool.Issue {
				i := base
				i.Session = "sess-123"
				return i
			}(),
			want:    []string{"(session sess-123)"},
			missing: []string{"Related record", "SECURITY flags"},
		},
		{
			name: "with related record",
			issue: func() spool.Issue {
				i := base
				i.RelatedRecordID = "exp-0042"
				return i
			}(),
			want:    []string{"Related record: exp-0042."},
			missing: []string{"session", "SECURITY flags"},
		},
		{
			name:    "with security flags",
			issue:   base,
			flags:   []string{"secret", "pii"},
			want:    []string{"SECURITY flags: secret, pii."},
			missing: []string{"session", "Related record"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.issue.RenderBody("2026-06-22", tc.flags)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("RenderBody() = %q, want substring %q", got, want)
				}
			}
			for _, absent := range tc.missing {
				if strings.Contains(got, absent) {
					t.Errorf("RenderBody() = %q, must not contain %q", got, absent)
				}
			}
		})
	}
	// Description is trimmed of surrounding whitespace before rendering.
	if got := base.RenderBody("2026-06-22", nil); strings.Contains(got, "  a description") {
		t.Errorf("RenderBody() = %q, description padding must be trimmed", got)
	}
}

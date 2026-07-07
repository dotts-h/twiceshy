// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/spool"
)

func TestRecordExperience_QueuesForIntakeWhenConfigured(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200
	queue := t.TempDir()
	h.recordQueue = queue

	_, res, err := h.record(context.Background(), nil, RecordArgs{
		Kind:            "trap",
		Title:           "snowflake-novel-signature-spool-test",
		Summary:         "a novel issue summary",
		ErrorSignatures: []string{"novel-sig-123"},
		RootCause:       "novel root cause",
		Fix:             "novel fix",
		Body:            "novel body description",
		Author:          "agent-xyz",
		Session:         "sess-xyz",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	if !strings.Contains(res.Message, "queued") {
		t.Fatalf("a queued record must say so in the message: %q", res.Message)
	}
	if !strings.Contains(res.Message, "Contribution queued for moderation — it enters review quarantined; the record id is provisional until intake.") {
		t.Errorf("unexpected message: %q", res.Message)
	}

	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued record draft, got %d", len(paths))
	}

	draft, err := spool.ReadRecord(paths[0])
	if err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}

	if draft.Kind != "trap" || draft.Title != "snowflake-novel-signature-spool-test" ||
		draft.Summary != "a novel issue summary" || len(draft.ErrorSignatures) != 1 ||
		draft.ErrorSignatures[0] != "novel-sig-123" || draft.Body != "novel body description" ||
		draft.Author != "agent-xyz" || draft.Session != "sess-xyz" ||
		draft.RootCause != "novel root cause" || draft.Fix != "novel fix" {
		t.Fatalf("queued draft lost/mismatched data: %+v", draft)
	}
}

func TestRecordExperience_NoQueueReturnsMarkdown(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200
	// recordQueue remains empty/unset

	_, res, err := h.record(context.Background(), nil, RecordArgs{
		Kind:            "trap",
		Title:           "snowflake-novel-signature-spool-test-noqueue",
		Summary:         "a novel issue summary",
		ErrorSignatures: []string{"novel-sig-456"},
		RootCause:       "novel root cause",
		Fix:             "novel fix",
		Body:            "novel body description",
		Author:          "agent-xyz",
		Session:         "sess-xyz",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	if res.Markdown == "" {
		t.Fatal("the legacy path must return the draft markdown to PR")
	}
	if strings.Contains(res.Message, "queued") {
		t.Fatalf("the no-queue path must not claim the record was queued: %q", res.Message)
	}
}

func TestRecordExperience_KnownDuplicateDoesNotSpool(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200 (validated)
	queue := t.TempDir()
	h.recordQueue = queue

	// We try to record something that is a known duplicate.
	// usageFixture() returns exp-0200 with:
	// Title: "usage wiring fixture record with a sufficiently long title"
	// ErrorSignatures: []string{"zzdistinct-usage-signature-marker"}
	// Kind: "trap"
	// Body: ""
	_, res, err := h.record(context.Background(), nil, RecordArgs{
		Kind:            "trap",
		Title:           "usage wiring fixture record with a sufficiently long title",
		Summary:         "a distinctive symptom for the usage wiring test",
		ErrorSignatures: []string{"zzdistinct-usage-signature-marker"},
		RootCause:       "novel root cause",
		Fix:             "novel fix",
		Body:            "some body",
		Author:          "agent-xyz",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	if res.Novelty != "known" {
		t.Fatalf("expected novelty known, got %q", res.Novelty)
	}

	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("known duplicate must not spool anything, got %d files", len(paths))
	}
}

func TestRecordExperience_SpoolEnqueueFailureReturnsError(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	// Use an unwritable queue directory path.
	dir := filepath.Join(t.TempDir(), "file-spool")
	if err := os.WriteFile(dir, []byte("im a file not a dir"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	h.recordQueue = dir

	_, _, err := h.record(context.Background(), nil, RecordArgs{
		Kind:            "trap",
		Title:           "snowflake-novel-signature-spool-fail",
		Summary:         "a novel issue summary",
		ErrorSignatures: []string{"novel-sig-fail"},
		RootCause:       "novel root cause",
		Fix:             "novel fix",
		Body:            "novel body description",
		Author:          "agent-xyz",
	})
	if err == nil {
		t.Fatal("expected enqueue failure to return tool error, got nil")
	}
	if !strings.Contains(err.Error(), "queueing record draft for intake") &&
		!strings.Contains(err.Error(), "not a directory") &&
		!strings.Contains(err.Error(), "exists") {
		t.Errorf("unexpected error message: %v", err)
	}
}

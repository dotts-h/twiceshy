// SPDX-License-Identifier: AGPL-3.0-only

// Package spool is the intake queue for the two write-back channels that defer
// record-building to a drain step. report_outcome enqueues an outcome Report
// (ADR-0013 §E1) drained by `intake-reports`; the session-retro hook enqueues a
// Transcript (ADR-0018, #0065) drained by `retro-intake`. Either queue stores the
// REQUEST (not a built record), so the record id is allocated against the live
// corpus at intake time — never colliding across entries queued before a drain.
package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Report is one queued outcome report — the report_outcome arguments plus when it
// was filed. It is intentionally the request, not a record: the counter-record is
// built at intake against a fresh corpus id.
type Report struct {
	RecordID   string `json:"record_id"`
	Outcome    string `json:"outcome"`
	Evidence   string `json:"evidence,omitempty"`
	Author     string `json:"author"`
	Session    string `json:"session,omitempty"`
	ReportedAt string `json:"reported_at"`
}

// Enqueue writes outcome report r as a JSON file in dir, prefixed with its
// ReportedAt so List returns roughly time order.
func Enqueue(dir string, r Report) (string, error) {
	return enqueueJSON(dir, r.ReportedAt, r)
}

// enqueueJSON marshals v and writes it as a JSON file in dir (created if absent).
// It writes to a hidden temp file then renames to a unique `*.json` name, so a
// concurrent List never observes a half-written entry (rename is atomic within a
// directory). The filename is prefixed with timePrefix so List returns roughly
// time order.
func enqueueJSON(dir, timePrefix string, v any) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".enq-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, sanitize(timePrefix)+"-"+filepath.Base(tmpName)+".json")
	if err := os.Rename(tmpName, final); err != nil {
		return "", err
	}
	return final, nil
}

// List returns the queued report file paths in dir, sorted (stable, roughly
// chronological). A missing dir is an empty queue, not an error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Read decodes a queued report file.
func Read(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return Report{}, err
	}
	return r, nil
}

// Transcript is one queued session-retro capture (#0065, ADR-0018): the SessionEnd
// hook ships a bounded session transcript here instead of the agent authoring a
// record, and the `retro-intake` CLI drains it through an off-pool analyzer into
// quarantined experience drafts. Like Report it stores the request (transcript text
// + metadata), not a built record — ids are allocated against the live corpus at
// intake time.
type Transcript struct {
	SessionID  string `json:"session_id"`
	Author     string `json:"author"`
	Reason     string `json:"reason,omitempty"`
	Transcript string `json:"transcript"`
	CapturedAt string `json:"captured_at"`
}

// EnqueueTranscript writes t as a JSON file in dir using the same atomic
// temp-then-rename discipline as Enqueue, prefixed with CapturedAt for time order.
func EnqueueTranscript(dir string, t Transcript) (string, error) {
	return enqueueJSON(dir, t.CapturedAt, t)
}

// ReadTranscript decodes a queued transcript file.
func ReadTranscript(path string) (Transcript, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Transcript{}, err
	}
	var t Transcript
	if err := json.Unmarshal(data, &t); err != nil {
		return Transcript{}, err
	}
	return t, nil
}

// Issue is one queued agent-submitted issue (#0066): the report_issue tool ships
// half-formed input here — a problem with no fix yet, or a twiceshy-self bug —
// instead of a complete lesson, and `intake-issues` drains it into docs/issues/.
// Like Report it stores the request, not a built artifact: the issue NUMBER is
// allocated against docs/issues/ at intake, never colliding across entries queued
// before a drain. Agent-submitted issues are triage-flagged, never auto-actioned.
type Issue struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"` // bug | feature | question
	RelatedRecordID string `json:"related_record_id,omitempty"`
	Author          string `json:"author"`
	Session         string `json:"session,omitempty"`
	ReportedAt      string `json:"reported_at"`
}

// RenderBody renders the docs/issues body for an agent-submitted issue: the
// description under `## Summary`, then the provenance (category/author/session/related
// record, plus any security-screen flags) under `## Notes`. Both report_issue paths —
// the server's PR-ready no-queue fallback and the `intake-issues` drainer — share this
// so the two never drift (one fact, one home). `now` is a pre-formatted YYYY-MM-DD
// date; the title is NOT rendered here (it lives, escaped, in the frontmatter).
func (i Issue) RenderBody(now string, flags []string) string {
	var b strings.Builder
	b.WriteString("## Summary\n")
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(i.Description))
	b.WriteString("## Notes\n")
	fmt.Fprintf(&b, "Agent-submitted via report_issue (category: %s) by %s", i.Category, i.Author)
	if i.Session != "" {
		fmt.Fprintf(&b, " (session %s)", i.Session)
	}
	fmt.Fprintf(&b, " on %s. Triage-flagged: never auto-actioned (#0066).", now)
	if i.RelatedRecordID != "" {
		fmt.Fprintf(&b, " Related record: %s.", i.RelatedRecordID)
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, " SECURITY flags: %s.", strings.Join(flags, ", "))
	}
	b.WriteString("\n")
	return b.String()
}

// EnqueueIssue writes i as a JSON file in dir using the same atomic
// temp-then-rename discipline as Enqueue, prefixed with ReportedAt for time order.
func EnqueueIssue(dir string, i Issue) (string, error) {
	return enqueueJSON(dir, i.ReportedAt, i)
}

// ReadIssue decodes a queued issue file.
func ReadIssue(path string) (Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Issue{}, err
	}
	var i Issue
	if err := json.Unmarshal(data, &i); err != nil {
		return Issue{}, err
	}
	return i, nil
}

// Remove deletes a processed queue file.
func Remove(path string) error { return os.Remove(path) }

// sanitize keeps a filename-safe prefix (the ReportedAt timestamp has ':' and
// '.' that are awkward in filenames on some systems).
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ':', '/', '\\', ' ':
			return '-'
		default:
			return r
		}
	}, s)
}

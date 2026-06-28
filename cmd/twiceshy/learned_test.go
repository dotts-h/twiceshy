// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// runLearnedCLI invokes `twiceshy learned` end-to-end through the top-level dispatch
// (so the test gates the dispatch wiring too), against a fresh temp corpus + db.
func runLearnedCLI(t *testing.T, dir string, extra ...string) (string, error) {
	t.Helper()
	args := append([]string{
		"learned",
		"-corpus", dir,
		"-db", filepath.Join(dir, "twiceshy.db"),
	}, extra...)
	var out bytes.Buffer
	err := run(context.Background(), args, &out, func(string) string { return "" })
	return out.String(), err
}

func newLearnedCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func learnedRecordFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "experience", "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func parseLearnedRecord(t *testing.T, dir, path string) *record.Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := record.Parse(filepath.ToSlash(rel), data)
	if err != nil {
		t.Fatalf("written record must parse: %v", err)
	}
	return rec
}

func hasLearnedString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// rnLesson is a representative capture — a React Native trap (the empty live-dogfood
// cell, #0088). Two -error flags lock the repeatable error-signature behavior.
var rnLesson = []string{
	"-kind", "trap",
	"-title", "react-native maplibre v11 renamed MapView to Map; the old name crashes at runtime",
	"-summary", "iOS runtime crash after upgrading @maplibre/maplibre-react-native to v11",
	"-error", "Element type is invalid ... got: undefined",
	"-error", "Invariant Violation: Element type is invalid",
	"-root-cause", "v11 renamed the MapView component to Map and replaced the defaultSettings prop",
	"-fix", "import { Map, Camera } from '@maplibre/maplibre-react-native' and use mapStyle + initialViewState",
	"-verified-by", "app boots on the iOS simulator without the invalid-element crash",
	"-ecosystem", "JavaScript",
	"-package", "@maplibre/maplibre-react-native",
	"-author", "claude",
}

func TestLearnedWritesQuarantinedDraft(t *testing.T) {
	dir := newLearnedCorpus(t)
	out, err := runLearnedCLI(t, dir, rnLesson...)
	if err != nil {
		t.Fatalf("runLearned: %v\nout: %s", err, out)
	}
	files := learnedRecordFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("want exactly 1 record written, got %d: %v\nout: %s", len(files), files, out)
	}
	rec := parseLearnedRecord(t, dir, files[0])

	if rec.Status != "quarantined" {
		t.Errorf("status = %q, want quarantined", rec.Status)
	}
	if rec.Kind != "trap" {
		t.Errorf("kind = %q, want trap", rec.Kind)
	}
	if rec.Symptom == nil ||
		!hasLearnedString(rec.Symptom.ErrorSignatures, "Element type is invalid ... got: undefined") ||
		!hasLearnedString(rec.Symptom.ErrorSignatures, "Invariant Violation: Element type is invalid") {
		t.Errorf("both -error signatures must be captured (repeatable flag): %+v", rec.Symptom)
	}
	if rec.Resolution == nil || !strings.Contains(rec.Resolution.RootCause, "renamed the MapView") || rec.Resolution.Fix == "" {
		t.Errorf("resolution not captured: %+v", rec.Resolution)
	}
	if rec.Guard == nil || rec.Guard.GuardingTest == nil || *rec.Guard.GuardingTest == "" {
		t.Errorf("-verified-by must land in Guard.GuardingTest: %+v", rec.Guard)
	}
	if len(rec.AppliesTo) == 0 || rec.AppliesTo[0].Ecosystem != "JavaScript" ||
		rec.AppliesTo[0].Package != "@maplibre/maplibre-react-native" {
		t.Errorf("appliesTo not captured: %+v", rec.AppliesTo)
	}
	if rec.Provenance.Source.Author != "claude" {
		t.Errorf("author = %q, want claude", rec.Provenance.Source.Author)
	}
	if !strings.Contains(out, rec.ID) {
		t.Errorf("output should report the allocated id %q, got: %s", rec.ID, out)
	}
}

// IncludeQuarantined=true: re-capturing the same lesson is idempotent (bulk authoring
// campaigns must not pile up duplicate quarantined drafts).
func TestLearnedIsIdempotent(t *testing.T) {
	dir := newLearnedCorpus(t)
	if _, err := runLearnedCLI(t, dir, rnLesson...); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	out, err := runLearnedCLI(t, dir, rnLesson...)
	if err != nil {
		t.Fatalf("second capture: %v", err)
	}
	if files := learnedRecordFiles(t, dir); len(files) != 1 {
		t.Fatalf("idempotent capture must leave exactly 1 record, got %d: %v", len(files), files)
	}
	if !strings.Contains(strings.ToLower(out), "already covered") {
		t.Errorf("second capture should report it is already covered, got: %s", out)
	}
}

func TestLearnedStdoutPrintsAndWritesNothing(t *testing.T) {
	dir := newLearnedCorpus(t)
	out, err := runLearnedCLI(t, dir, append([]string{"-stdout"}, rnLesson...)...)
	if err != nil {
		t.Fatalf("runLearned -stdout: %v", err)
	}
	if files := learnedRecordFiles(t, dir); len(files) != 0 {
		t.Fatalf("-stdout must write no file, got %d: %v", len(files), files)
	}
	if !strings.Contains(out, "status: quarantined") {
		t.Errorf("-stdout should print the rendered quarantined draft, got: %s", out)
	}
}

// The agreed permissive bar: a capture missing root-cause/fix WARNS but is still recorded
// (the judge/promote gate is the real filter).
func TestLearnedWarnsButRecordsWithoutRootCause(t *testing.T) {
	dir := newLearnedCorpus(t)
	out, err := runLearnedCLI(t, dir,
		"-title", "a captured symptom with no diagnosis yet",
		"-error", "panic: runtime error: invalid memory address",
	)
	if err != nil {
		t.Fatalf("permissive capture must not error: %v\nout: %s", err, out)
	}
	files := learnedRecordFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("permissive capture must still record, got %d files", len(files))
	}
	if !strings.Contains(strings.ToLower(out), "warning") {
		t.Errorf("missing root-cause/fix should emit a warning, got: %s", out)
	}
	// Permissive capture must NOT smuggle an un-diagnosed trap past the promote
	// pre-gate: a record with no real root cause has to be HELD, not treated as
	// substantive. A placeholder like "unknown" would bypass HasSubstantiveRootCause
	// (its hold-set is the leading word "none"/"n/a"); the held marker must be "None…".
	rec := parseLearnedRecord(t, dir, files[0])
	if promote.HasSubstantiveRootCause(rec) {
		t.Errorf("a capture missing root-cause must be HELD by the promote pre-gate, but root_cause %q is treated as substantive (it would bypass the gate #0094 built)", rec.Resolution.RootCause)
	}
}

func TestLearnedAutoComposesBodyFromFields(t *testing.T) {
	dir := newLearnedCorpus(t)
	// No -body flag: the command composes a body from the structured fields.
	if _, err := runLearnedCLI(t, dir, rnLesson...); err != nil {
		t.Fatalf("runLearned: %v", err)
	}
	files := learnedRecordFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("want 1 record, got %d", len(files))
	}
	rec := parseLearnedRecord(t, dir, files[0])
	if strings.TrimSpace(rec.Body) == "" {
		t.Fatal("auto-composed body must be non-empty")
	}
	if !strings.Contains(rec.Body, "renamed the MapView") || !strings.Contains(rec.Body, "import { Map, Camera }") {
		t.Errorf("auto-composed body should carry the root-cause and fix, got:\n%s", rec.Body)
	}
}

func TestLearnedRequiresTitle(t *testing.T) {
	dir := newLearnedCorpus(t)
	out, err := runLearnedCLI(t, dir, "-error", "some error with no title")
	if err == nil {
		t.Fatalf("missing -title must error; out: %s", out)
	}
	if files := learnedRecordFiles(t, dir); len(files) != 0 {
		t.Fatalf("nothing should be written on a bad invocation, got %d", len(files))
	}
}

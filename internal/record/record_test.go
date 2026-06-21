// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dotts-h/twiceshy/internal/record"
)

// repoRoot is the corpus root for this repo's own records, which double as
// the spec's worked examples (docs/SCHEMA.md).
const repoRoot = "../.."

func TestParseFileParsesAFullRecord(t *testing.T) {
	rec, err := record.ParseFile(repoRoot, "experience/2026/0001-fts5-match-raw-user-input.md")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if rec.ID != "exp-0001" || rec.Kind != "trap" || rec.Status != "validated" {
		t.Errorf("got id=%s kind=%s status=%s", rec.ID, rec.Kind, rec.Status)
	}
	if rec.Symptom == nil || len(rec.Symptom.ErrorSignatures) != 4 {
		t.Fatalf("want 4 error signatures, got %+v", rec.Symptom)
	}
	if rec.Resolution == nil || len(rec.Resolution.DeadEnds) != 3 {
		t.Fatalf("want 3 dead ends, got %+v", rec.Resolution)
	}
	if rec.Guard == nil || rec.Guard.GuardingTest == nil || *rec.Guard.GuardingTest != "TestSearchQuoteEscapesFTS5Input" {
		t.Errorf("guard = %+v", rec.Guard)
	}
	if rec.Provenance.Source.Author != "horia" {
		t.Errorf("author = %q", rec.Provenance.Source.Author)
	}
	if !strings.Contains(rec.Body, "## The trap") {
		t.Error("narrative body missing")
	}
	if rec.Path != "experience/2026/0001-fts5-match-raw-user-input.md" {
		t.Errorf("path = %q", rec.Path)
	}
}

func TestParseFileAllowsRecordWithoutSymptom(t *testing.T) {
	rec, err := record.ParseFile(repoRoot, "experience/2026/0003-mcp-streamable-http-not-sse.md")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if rec.Kind != "convention" || rec.Symptom != nil {
		t.Errorf("convention record should have no symptom, got %+v", rec.Symptom)
	}
	if len(rec.AppliesTo) != 1 || rec.AppliesTo[0].Ecosystem != "MCP" {
		t.Errorf("applies_to = %+v", rec.AppliesTo)
	}
}

func TestLoadCorpusLoadsAllExamples(t *testing.T) {
	recs, err := record.LoadCorpus(repoRoot)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) < 3 {
		t.Fatalf("want at least the 3 worked examples, got %d", len(recs))
	}
	// Structural invariants that must hold as the corpus grows: ids are
	// non-empty, unique, and returned sorted ascending.
	seen := make(map[string]bool, len(recs))
	for i, r := range recs {
		if r.ID == "" {
			t.Errorf("recs[%d] has empty id", i)
		}
		if seen[r.ID] {
			t.Errorf("duplicate id %s", r.ID)
		}
		seen[r.ID] = true
		if i > 0 && recs[i-1].ID > r.ID {
			t.Errorf("not sorted ascending: %s before %s", recs[i-1].ID, r.ID)
		}
	}
	// The original worked examples must remain present.
	for _, want := range []string{"exp-0001", "exp-0002", "exp-0003"} {
		if !seen[want] {
			t.Errorf("missing worked example %s", want)
		}
	}
}

// fm builds a minimal valid frontmatter for a validated trap as a mutable
// map, so each failure case below states only its delta from valid.
func fm() map[string]any {
	return map[string]any{
		"schema_version": 1,
		"id":             "exp-0042",
		"kind":           "trap",
		"status":         "validated",
		"title":          "A perfectly plausible trap title",
		"symptom": map[string]any{
			"summary":          "something observable went wrong",
			"error_signatures": []any{"boom: kaput near line 3"},
		},
		"applies_to": []any{
			map[string]any{"ecosystem": "Go", "package": "example.com/mod"},
		},
		"resolution": map[string]any{
			"root_cause": "two factors, not one blame line",
			"fix":        "the change that worked",
		},
		"guard": map[string]any{
			"repro":         nil,
			"guarding_test": "TestSomething",
		},
		"provenance": map[string]any{
			"source":        map[string]any{"author": "horia", "session": nil, "pr": nil},
			"recorded_at":   "2026-06-12",
			"validated_at":  "2026-06-12",
			"valid":         map[string]any{"from": "2026-06-12", "until": nil},
			"superseded_by": nil,
			"usage":         map[string]any{"retrieved": 0, "confirmed_helpful": 0, "last_hit": nil},
		},
	}
}

func render(t *testing.T, front map[string]any) []byte {
	t.Helper()
	y, err := yaml.Marshal(front)
	if err != nil {
		t.Fatalf("marshal frontmatter: %v", err)
	}
	return []byte("---\n" + string(y) + "---\n\nA narrative body.\n")
}

const fmPath = "experience/2026/0042-a-perfectly-plausible-trap.md"

func TestParseAcceptsTheBaseFixture(t *testing.T) {
	if _, err := record.Parse(fmPath, render(t, fm())); err != nil {
		t.Fatalf("base fixture must be valid, got: %v", err)
	}
}

// A validated trap proven by EXECUTION (a positive repro) needs no separate
// guarding_test — the repro is the proof (ADR-0011). Both the legacy single
// guard.repro and a guard.repros positive entry satisfy this.
func TestParseAcceptsValidatedTrapWithReproAndNoGuardingTest(t *testing.T) {
	legacy := fm()
	legacy["guard"] = map[string]any{"repro": "experience/repro/0017.sh", "guarding_test": nil}
	if _, err := record.Parse(fmPath, render(t, legacy)); err != nil {
		t.Errorf("validated trap with a positive guard.repro must be valid, got: %v", err)
	}

	testSet := fm()
	testSet["guard"] = map[string]any{
		"repro":         nil,
		"guarding_test": nil,
		"repros": []any{
			map[string]any{"path": "experience/repro/0017.sh", "kind": "positive"},
		},
	}
	if _, err := record.Parse(fmPath, render(t, testSet)); err != nil {
		t.Errorf("validated trap with a positive guard.repros entry must be valid, got: %v", err)
	}

	// A validated trap with ONLY a negative repro (no positive, no guarding_test)
	// has no proof the fix holds — it must still be rejected.
	negOnly := fm()
	negOnly["guard"] = map[string]any{
		"repro":         nil,
		"guarding_test": nil,
		"repros": []any{
			map[string]any{"path": "experience/repro/neg.sh", "kind": "negative"},
		},
	}
	if _, err := record.Parse(fmPath, render(t, negOnly)); err == nil {
		t.Error("validated trap with only a negative repro and no guarding_test must be rejected")
	}
}

func TestParseRejections(t *testing.T) {
	del := func(keys ...string) func(map[string]any) {
		return func(m map[string]any) {
			cur := m
			for _, k := range keys[:len(keys)-1] {
				cur = cur[k].(map[string]any)
			}
			delete(cur, keys[len(keys)-1])
		}
	}
	set := func(v any, keys ...string) func(map[string]any) {
		return func(m map[string]any) {
			cur := m
			for _, k := range keys[:len(keys)-1] {
				cur = cur[k].(map[string]any)
			}
			cur[keys[len(keys)-1]] = v
		}
	}

	tests := []struct {
		name    string
		mutate  func(map[string]any)
		path    string
		wantErr string
	}{
		{"unknown schema version", set(2, "schema_version"), fmPath, "schema_version"},
		{"malformed id", set("expp-42", "id"), fmPath, "id"},
		{"unknown kind", set("oops", "kind"), fmPath, "kind"},
		{"unknown status", set("trusted", "status"), fmPath, "status"},
		{"missing title", del("title"), fmPath, "title"},
		{"title too short", set("nah", "title"), fmPath, "title"},
		{"title too long", set(strings.Repeat("x", 121), "title"), fmPath, "title"},
		{"trap without symptom", del("symptom"), fmPath, "symptom"},
		{"trap without resolution", del("resolution"), fmPath, "resolution"},
		{"trap without fix", del("resolution", "fix"), fmPath, "resolution.fix"},
		{"validated trap without guarding test", set(nil, "guard", "guarding_test"), fmPath, "guard"},
		{"empty symptom summary", set("", "symptom", "summary"), fmPath, "summary"},
		{"bad explicit fingerprint", set(map[string]any{"generic": []any{"sha256:nope"}}, "symptom", "fingerprints"), fmPath, "fingerprint"},
		{"applies_to entry with no anchor", set([]any{map[string]any{"versions": map[string]any{"introduced": "1.0"}}}, "applies_to"), fmPath, "applies_to"},
		{"bad date", set("June 12th", "provenance", "recorded_at"), fmPath, "recorded_at"},
		{"missing author", del("provenance", "source", "author"), fmPath, "author"},
		{"validated without validated_at", set(nil, "provenance", "validated_at"), fmPath, "validated_at"},
		{"superseded without pointer", set("superseded", "status"), fmPath, "superseded_by"},
		{"valid.until before valid.from", set("2026-01-01", "provenance", "valid", "until"), fmPath, "valid"},
		{"validated_at before recorded_at", set("2020-01-01", "provenance", "validated_at"), fmPath, "validated_at"},
		{"filename number mismatch", nil, "experience/2026/0043-a-perfectly-plausible-trap.md", "filename"},
		{"year directory mismatch", nil, "experience/2025/0042-a-perfectly-plausible-trap.md", "year"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			front := fm()
			if tt.mutate != nil {
				tt.mutate(front)
			}
			_, err := record.Parse(tt.path, render(t, front))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRejectsDeadEndKindWithoutDeadEnds(t *testing.T) {
	front := fm()
	front["kind"] = "dead-end"
	front["guard"] = nil
	if _, err := record.Parse(fmPath, render(t, front)); err == nil || !strings.Contains(err.Error(), "dead_ends") {
		t.Errorf("want dead_ends error, got %v", err)
	}
}

func TestParseRejectsUnknownFrontmatterFields(t *testing.T) {
	front := fm()
	front["serverity"] = "high" // typo'd extra field must not pass silently
	if _, err := record.Parse(fmPath, render(t, front)); err == nil {
		t.Error("unknown field must be rejected (additionalProperties: false)")
	}
}

// ParseLenient is the read/serve path: an ADDITIVE frontmatter field written by a
// newer writer must NOT make an older server fail to load the record (the outage:
// the corpus got `panel`, the deployed binary's struct lacked it, strict unmarshal
// crash-looped serve). Strict Parse still rejects it (write/CI catches typos).
func TestParseLenientToleratesUnknownFrontmatterField(t *testing.T) {
	front := fm()
	front["future_schema_field"] = "a field an older binary does not know"
	src := render(t, front)
	if _, err := record.Parse(fmPath, src); err == nil {
		t.Fatal("strict Parse must still reject an unknown field")
	}
	rec, err := record.ParseLenient(fmPath, src)
	if err != nil {
		t.Fatalf("ParseLenient must tolerate an additive field, got: %v", err)
	}
	if rec.ID != "exp-0042" {
		t.Fatalf("want exp-0042 loaded, got %q", rec.ID)
	}
}

// LoadCorpusForServe must keep the read path AVAILABLE: serve every loadable
// record (including ones with additive unknown fields) and skip+report the rest,
// never aborting the whole load on one bad file.
func TestLoadCorpusForServeSkipsBadAndToleratesUnknown(t *testing.T) {
	good := fm()
	withUnknown := fm()
	withUnknown["id"] = "exp-0043"
	withUnknown["future_schema_field"] = "additive field an older server rejects"
	root := writeCorpus(t, map[string][]byte{
		"experience/2026/0042-good-record.md":   render(t, good),
		"experience/2026/0043-unknown-field.md": render(t, withUnknown),
		"experience/2026/0044-broken-record.md": []byte("---\n\tnot: [valid yaml\n---\nbody text\n"),
	})
	recs, skipped, err := record.LoadCorpusForServe(root)
	if err != nil {
		t.Fatalf("LoadCorpusForServe must not hard-fail on one bad record: %v", err)
	}
	ids := map[string]bool{}
	for _, r := range recs {
		ids[r.ID] = true
	}
	if !ids["exp-0042"] || !ids["exp-0043"] {
		t.Fatalf("both the good and the unknown-field record must be served, got %v", ids)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "0044-broken") {
		t.Fatalf("want exactly the malformed file skipped+reported, got %d: %v", len(skipped), skipped)
	}
}

// A duplicate id is the OTHER serve-fatal class (#0060): the id is a PRIMARY KEY,
// so a second record with the same id would abort the index rebuild → crash-loop.
// The serve loader must keep the first and skip+report the dup, staying up.
func TestLoadCorpusForServeSkipsDuplicateID(t *testing.T) {
	a := fm()
	b := fm() // same id exp-0042, different file
	root := writeCorpus(t, map[string][]byte{
		"experience/2026/0042-first.md":  render(t, a),
		"experience/2026/0042-second.md": render(t, b),
	})
	recs, skipped, err := record.LoadCorpusForServe(root)
	if err != nil {
		t.Fatalf("LoadCorpusForServe must not hard-fail on a duplicate id: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != "exp-0042" {
		t.Fatalf("want exactly one exp-0042 served, got %d: %v", len(recs), recs)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "duplicate id exp-0042") {
		t.Fatalf("want the duplicate skipped+reported, got %d: %v", len(skipped), skipped)
	}
}

func TestParseRejectsStructuralBreakage(t *testing.T) {
	cases := map[string]string{
		"no frontmatter fence": "schema_version: 1\n",
		"unterminated fence":   "---\nschema_version: 1\n",
		"empty body":           "---\n" + "schema_version: 1\n" + "---\n   \n",
		"invalid yaml":         "---\n\t- {{\n---\nbody\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := record.Parse(fmPath, []byte(src)); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

// writeCorpus lays out a temp corpus root with the given record files.
func writeCorpus(t *testing.T, files map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	for rel, data := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadCorpusRejectsDanglingSupersededBy(t *testing.T) {
	front := fm()
	front["status"] = "superseded"
	front["provenance"].(map[string]any)["superseded_by"] = "exp-9999"
	front["provenance"].(map[string]any)["valid"].(map[string]any)["until"] = "2026-06-12"
	root := writeCorpus(t, map[string][]byte{fmPath: render(t, front)})
	if _, err := record.LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "exp-9999") {
		t.Errorf("want dangling superseded_by error, got %v", err)
	}
}

func TestLoadCorpusRejectsDuplicateIDs(t *testing.T) {
	root := writeCorpus(t, map[string][]byte{
		fmPath: render(t, fm()),
		"experience/2026/0042-same-id-different-slug.md": render(t, fm()),
	})
	if _, err := record.LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("want duplicate id error, got %v", err)
	}
}

func TestLoadCorpusRejectsMissingReproFile(t *testing.T) {
	front := fm()
	front["guard"].(map[string]any)["repro"] = "experience/repro/0042-nope.sh"
	root := writeCorpus(t, map[string][]byte{fmPath: render(t, front)})
	if _, err := record.LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "repro") {
		t.Errorf("want missing repro error, got %v", err)
	}
}

func TestLoadCorpusIgnoresReproDirAndNonRecordFiles(t *testing.T) {
	root := writeCorpus(t, map[string][]byte{
		fmPath:                       render(t, fm()),
		"experience/repro/0042-x.sh": []byte("#!/bin/sh\n"),
		"experience/README.md":       []byte("not a record\n"),
		"experience/2026/notes.txt":  []byte("scratch\n"),
	})
	recs, err := record.LoadCorpus(root)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Errorf("want 1 record, got %d", len(recs))
	}
}

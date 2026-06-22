// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

// Marshal output must satisfy the *normative* JSON Schema
// (schema/experience-record.v1.schema.json), not merely round-trip through
// Parse. SCHEMA.md declares the schema machine-checkable, and
// record_experience hands Marshal's output to an agent as a ready-to-open PR —
// so a schema-invalid serialization (e.g. `symptom: null`, `runtime: {}`,
// `applies_to: []`) is a real defect that lands an invalid record in the corpus.
// These tests guard the omitempty contract: optional blocks are *omitted*, not
// null/empty-materialized.

const schemaPath = "../../schema/experience-record.v1.schema.json"

func loadRecordSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return resolved
}

// frontmatterValue extracts the YAML frontmatter from a marshaled record and
// returns it as a JSON-shaped value (map[string]any with float64 numbers) — the
// form the validator expects, matching how a real JSON document would parse.
func frontmatterValue(t *testing.T, marshaled []byte) any {
	t.Helper()
	s := strings.TrimPrefix(string(marshaled), "---\n")
	end := strings.Index(s, "\n---\n")
	if end < 0 {
		t.Fatalf("no closing frontmatter fence in:\n%s", marshaled)
	}
	var ymap any
	if err := yaml.Unmarshal([]byte(s[:end+1]), &ymap); err != nil {
		t.Fatalf("unmarshal frontmatter yaml: %v", err)
	}
	j, err := json.Marshal(ymap)
	if err != nil {
		t.Fatalf("frontmatter to json: %v", err)
	}
	var v any
	if err := json.Unmarshal(j, &v); err != nil {
		t.Fatalf("json to value: %v", err)
	}
	return v
}

func TestMarshal_CorpusOutputSatisfiesSchema(t *testing.T) {
	schema := loadRecordSchema(t)
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("empty corpus — nothing to validate")
	}
	for _, r := range recs {
		out, err := record.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", r.ID, err)
		}
		if err := schema.Validate(frontmatterValue(t, out)); err != nil {
			t.Errorf("corpus record %s marshals to schema-invalid frontmatter: %v\n--- marshaled ---\n%s", r.ID, err, out)
		}
	}
}

// A minimal freshly-quarantined record — the exact shape record_experience
// proposes, with every optional block nil — must marshal to schema-valid
// frontmatter. Without omitempty this emits `symptom: null` / `resolution: null`
// / `guard: null`, each rejected by the schema (those $refs are non-nullable
// objects). This is the record_experience write-path contract.
func TestMarshal_MinimalDraftSatisfiesSchema(t *testing.T) {
	schema := loadRecordSchema(t)
	r := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-9001",
		Kind:          "convention",
		Status:        "quarantined",
		Title:         "Prefer constructor injection over package globals",
		AppliesTo:     []record.AppliesTo{{Ecosystem: "Go"}},
		Provenance: record.Provenance{
			Source:     record.Source{Author: "claude"},
			RecordedAt: "2026-06-17",
			Valid:      record.Validity{From: "2026-06-17"},
		},
		Body: "Use constructors; do not reach for package-level mutable state.",
		Path: "experience/2026/9001-prefer-constructor-injection.md",
	}
	if err := record.Validate(r); err != nil {
		t.Fatalf("fixture is not a valid record: %v", err)
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := schema.Validate(frontmatterValue(t, out)); err != nil {
		t.Errorf("minimal draft marshals to schema-invalid frontmatter: %v\n--- marshaled ---\n%s", err, out)
	}
}

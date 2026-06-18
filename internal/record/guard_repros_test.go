// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func guardWithRepros(front map[string]any, repros []map[string]any) {
	front["guard"].(map[string]any)["repros"] = repros
}

func TestParseGuardReprosPositiveAndNegative(t *testing.T) {
	front := fm()
	guardWithRepros(front, []map[string]any{
		{"path": "experience/repro/0042-positive.sh", "kind": "positive", "label": "fix holds"},
		{"path": "experience/repro/0042-negative.sh", "kind": "negative", "label": "dead-end stays failing"},
	})

	rec, err := record.Parse(fmPath, render(t, front))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rec.Guard == nil || len(rec.Guard.Repros) != 2 {
		t.Fatalf("guard.repros = %+v", rec.Guard)
	}
	pos, neg := rec.Guard.Repros[0], rec.Guard.Repros[1]
	if pos.Path != "experience/repro/0042-positive.sh" || pos.Kind != "positive" || pos.Label != "fix holds" {
		t.Errorf("positive repro = %+v", pos)
	}
	if neg.Path != "experience/repro/0042-negative.sh" || neg.Kind != "negative" || neg.Label != "dead-end stays failing" {
		t.Errorf("negative repro = %+v", neg)
	}

	out, err := record.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := record.Parse(fmPath, out)
	if err != nil {
		t.Fatalf("re-Parse marshaled: %v", err)
	}
	if len(back.Guard.Repros) != 2 ||
		back.Guard.Repros[0] != pos || back.Guard.Repros[1] != neg {
		t.Errorf("round-trip repros = %+v", back.Guard.Repros)
	}
}

func TestParseLegacyGuardReproOnlyStillValid(t *testing.T) {
	rec, err := record.ParseFile(repoRoot, "experience/2026/0001-fts5-match-raw-user-input.md")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if rec.Guard == nil || rec.Guard.Repro == nil {
		t.Fatalf("legacy record must keep guard.repro, got %+v", rec.Guard)
	}
	if *rec.Guard.Repro != "experience/repro/0001-fts5-raw-match.sh" {
		t.Errorf("repro = %q", *rec.Guard.Repro)
	}
	if len(rec.Guard.Repros) != 0 {
		t.Errorf("legacy record must have no repros, got %+v", rec.Guard.Repros)
	}
}

func TestParseRejectsInvalidGuardRepros(t *testing.T) {
	cases := []struct {
		name    string
		repros  []map[string]any
		wantErr string
	}{
		{
			name:    "missing kind",
			repros:  []map[string]any{{"path": "experience/repro/0042-x.sh"}},
			wantErr: "kind",
		},
		{
			name:    "invalid kind",
			repros:  []map[string]any{{"path": "experience/repro/0042-x.sh", "kind": "maybe"}},
			wantErr: "kind",
		},
		{
			name:    "missing path",
			repros:  []map[string]any{{"kind": "positive"}},
			wantErr: "path",
		},
		{
			name: "duplicate path",
			repros: []map[string]any{
				{"path": "experience/repro/0042-a.sh", "kind": "positive"},
				{"path": "experience/repro/0042-a.sh", "kind": "negative"},
			},
			wantErr: "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			front := fm()
			guardWithRepros(front, tc.repros)
			_, err := record.Parse(fmPath, render(t, front))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseSkipsMissingReprosPathSingleFile(t *testing.T) {
	front := fm()
	guardWithRepros(front, []map[string]any{
		{"path": "experience/repro/0042-nope.sh", "kind": "positive"},
	})
	if _, err := record.Parse(fmPath, render(t, front)); err != nil {
		t.Fatalf("single-file validation must not check repros path existence: %v", err)
	}
}

func TestLoadCorpusRejectsMissingReprosPath(t *testing.T) {
	front := fm()
	guardWithRepros(front, []map[string]any{
		{"path": "experience/repro/0042-nope.sh", "kind": "positive"},
	})
	root := writeCorpus(t, map[string][]byte{fmPath: render(t, front)})
	if _, err := record.LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "repros") {
		t.Errorf("want missing repros path error, got %v", err)
	}
}

func TestSchema_GuardReprosShape(t *testing.T) {
	schema := loadRecordSchema(t)

	valid := map[string]any{
		"schema_version": 1,
		"id":             "exp-9002",
		"kind":           "trap",
		"status":         "quarantined",
		"title":          "A perfectly plausible trap title",
		"symptom": map[string]any{
			"summary": "something observable went wrong",
		},
		"resolution": map[string]any{
			"root_cause": "two factors",
			"fix":        "the change that worked",
		},
		"guard": map[string]any{
			"repro":         "experience/repro/0001.sh",
			"guarding_test": nil,
			"repros": []any{
				map[string]any{"path": "experience/repro/pos.sh", "kind": "positive", "label": "holds"},
				map[string]any{"path": "experience/repro/neg.sh", "kind": "negative"},
			},
		},
		"provenance": map[string]any{
			"source":      map[string]any{"author": "horia"},
			"recorded_at": "2026-06-12",
			"valid":       map[string]any{"from": "2026-06-12"},
		},
	}
	if err := schema.Validate(valid); err != nil {
		t.Errorf("valid guard.repros shape rejected: %v", err)
	}

	invalid := map[string]any{
		"schema_version": 1,
		"id":             "exp-9003",
		"kind":           "trap",
		"status":         "quarantined",
		"title":          "A perfectly plausible trap title",
		"symptom": map[string]any{
			"summary": "something observable went wrong",
		},
		"resolution": map[string]any{
			"root_cause": "two factors",
			"fix":        "the change that worked",
		},
		"guard": map[string]any{
			"repros": []any{
				map[string]any{"path": "experience/repro/x.sh", "kind": "positive", "extra": true},
			},
		},
		"provenance": map[string]any{
			"source":      map[string]any{"author": "horia"},
			"recorded_at": "2026-06-12",
			"valid":       map[string]any{"from": "2026-06-12"},
		},
	}
	if err := schema.Validate(invalid); err == nil {
		t.Error("guard.repros item with unknown property must be rejected")
	}
}

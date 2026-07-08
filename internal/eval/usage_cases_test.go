// SPDX-License-Identifier: AGPL-3.0-only

package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsageCases(t *testing.T) {
	tmpDir := t.TempDir()

	// Helper to write JSON and load it
	writeAndLoad := func(t *testing.T, cases []UsageCase) ([]UsageCase, error) {
		t.Helper()
		data, err := json.Marshal(cases)
		if err != nil {
			t.Fatalf("failed to marshal cases: %v", err)
		}
		path := filepath.Join(tmpDir, "cases.json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
		return LoadUsageCases(path)
	}

	t.Run("valid case with 2 cases, one with empty Used", func(t *testing.T) {
		cases := []UsageCase{
			{
				Name:       "case-one",
				Transcript: "user and agent talk here",
				Served:     []string{"exp-0001", "exp-0002"},
				Used:       []string{"exp-0001"},
			},
			{
				Name:       "case-two",
				Transcript: "other transcript",
				Served:     []string{"exp-0003"},
				Used:       []string{}, // or nil
			},
		}
		loaded, err := writeAndLoad(t, cases)
		if err != nil {
			t.Fatalf("unexpected error loading valid cases: %v", err)
		}
		if len(loaded) != 2 {
			t.Fatalf("loaded len = %d, want 2", len(loaded))
		}
		if loaded[0].Name != "case-one" || loaded[1].Name != "case-two" {
			t.Errorf("loaded names mismatch: got %q and %q", loaded[0].Name, loaded[1].Name)
		}
	})

	t.Run("missing file error", func(t *testing.T) {
		_, err := LoadUsageCases(filepath.Join(tmpDir, "nonexistent.json"))
		if err == nil {
			t.Error("expected error for missing file, got nil")
		}
	})

	t.Run("empty array error", func(t *testing.T) {
		_, err := writeAndLoad(t, []UsageCase{})
		if err == nil {
			t.Error("expected error for empty array, got nil")
		} else if !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected error message to contain 'empty', got: %v", err)
		}
	})

	t.Run("missing Name error", func(t *testing.T) {
		cases := []UsageCase{
			{
				Name:       "",
				Transcript: "some transcript",
				Served:     []string{"exp-0001"},
			},
		}
		_, err := writeAndLoad(t, cases)
		if err == nil {
			t.Error("expected error for missing Name, got nil")
		}
	})

	t.Run("missing Transcript error", func(t *testing.T) {
		cases := []UsageCase{
			{
				Name:       "no-transcript",
				Transcript: "",
				Served:     []string{"exp-0001"},
			},
		}
		_, err := writeAndLoad(t, cases)
		if err == nil {
			t.Error("expected error for missing Transcript, got nil")
		} else if !strings.Contains(err.Error(), "no-transcript") {
			t.Errorf("expected error to name the offending case, got: %v", err)
		}
	})

	t.Run("empty Served error", func(t *testing.T) {
		cases := []UsageCase{
			{
				Name:       "empty-served",
				Transcript: "transcript",
				Served:     []string{},
			},
		}
		_, err := writeAndLoad(t, cases)
		if err == nil {
			t.Error("expected error for empty Served, got nil")
		} else if !strings.Contains(err.Error(), "empty-served") {
			t.Errorf("expected error to name the offending case, got: %v", err)
		}
	})

	t.Run("Used not subset of Served error", func(t *testing.T) {
		cases := []UsageCase{
			{
				Name:       "subset-violation",
				Transcript: "transcript",
				Served:     []string{"exp-0001"},
				Used:       []string{"exp-0001", "exp-0002"},
			},
		}
		_, err := writeAndLoad(t, cases)
		if err == nil {
			t.Error("expected error for Used not subset of Served, got nil")
		} else if !strings.Contains(err.Error(), "subset-violation") {
			t.Errorf("expected error to name the offending case, got: %v", err)
		} else if !strings.Contains(err.Error(), "exp-0002") {
			t.Errorf("expected error to mention the offending used id 'exp-0002', got: %v", err)
		}
	})
}

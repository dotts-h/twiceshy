// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dotts-h/twiceshy/internal/promote"
)

// The run manifest is the machine-readable artifact the morning review and the
// daily audit (#0044) read instead of scraping stdout — it must be valid JSON
// with one action per record transition (#0036, ADR-0013 §B2).
func TestRunManifest_WriteJSON(t *testing.T) {
	m := promote.RunManifest{
		RunID:  "run-20260619T120000Z",
		Stage:  "promote",
		Counts: map[string]int{"promoted": 1, "held": 1, "ineligible": 1},
		Actions: []promote.RecordAction{
			{ID: "exp-0100", Outcome: "promoted", FromStatus: "quarantined", ToStatus: "validated",
				JudgeModel: "gemini-2.5-pro", JudgeDecision: "approve", ReproducedUnder: []string{"go1.25"}},
			{ID: "exp-0101", Outcome: "held", FromStatus: "quarantined", ToStatus: "quarantined", Reason: "judge declined"},
			{ID: "exp-0102", Outcome: "ineligible", FromStatus: "validated", ToStatus: "validated", Reason: "not execution-provable"},
		},
	}

	var buf bytes.Buffer
	if err := m.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got promote.RunManifest
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("manifest is not valid JSON: %v\n%s", err, buf.String())
	}
	if got.RunID != m.RunID || got.Stage != "promote" {
		t.Fatalf("run scope lost in round-trip: %+v", got)
	}
	if len(got.Actions) != 3 {
		t.Fatalf("want 3 actions (one per record), got %d", len(got.Actions))
	}
	if got.Actions[0].ToStatus != "validated" || got.Actions[0].JudgeModel != "gemini-2.5-pro" {
		t.Fatalf("promoted action lost its transition/provenance: %+v", got.Actions[0])
	}
	if got.Counts["promoted"] != 1 {
		t.Fatalf("counts lost: %v", got.Counts)
	}
}

// An empty run still emits a JSON array, never null — the consumer can iterate
// unconditionally.
func TestRunManifest_EmptyActionsIsArray(t *testing.T) {
	m := promote.RunManifest{RunID: "r", Stage: "adapt", Counts: map[string]int{}, Actions: []promote.RecordAction{}}
	var buf bytes.Buffer
	if err := m.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"actions": []`)) {
		t.Fatalf("empty actions must serialize as [], got: %s", buf.String())
	}
}

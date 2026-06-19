// SPDX-License-Identifier: AGPL-3.0-only

package promote

import (
	"encoding/json"
	"io"
)

// RecordAction is one record's transition in a promote/adapt run: what it was,
// what it became, and the execution-backed evidence + judge verdict that
// justified the change. Held/ineligible/orphan records record no transition
// (FromStatus == ToStatus) but still appear, so the manifest lists every record
// the run considered — the daily audit (#0044) reads this, not stdout prose.
type RecordAction struct {
	ID              string   `json:"id"`
	Outcome         string   `json:"outcome"` // promoted|held|ineligible|demoted|disputed|orphan
	FromStatus      string   `json:"from_status"`
	ToStatus        string   `json:"to_status"`
	JudgeModel      string   `json:"judge_model,omitempty"`
	JudgeDecision   string   `json:"judge_decision,omitempty"`
	ReproducedUnder []string `json:"reproduced_under,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

// RunManifest is the machine-readable outcome of one loop-mutating run
// (ADR-0013 §B2). The nightly driver (#0043) commits it as run-<id>.json; the
// morning review and the daily Opus audit (#0044) consume it without scraping
// stdout. Stage is "promote" or "adapt"; Counts mirrors the run's stats.
type RunManifest struct {
	RunID string `json:"run_id"`
	Stage string `json:"stage"`
	// Anomaly is true when the run tripped the anomaly guardrail and halted
	// before persisting further (#0037, ADR-0013 §D1) — the daily audit reads
	// this to react to a compromised-judge spike without scraping logs.
	Anomaly bool           `json:"anomaly"`
	Counts  map[string]int `json:"counts"`
	Actions []RecordAction `json:"actions"`
}

// WriteJSON emits the manifest as indented JSON. Actions is always serialized as
// an array (never null) so a consumer can iterate unconditionally — callers pass
// a non-nil (possibly empty) slice.
func (m RunManifest) WriteJSON(w io.Writer) error {
	if m.Actions == nil {
		m.Actions = []RecordAction{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

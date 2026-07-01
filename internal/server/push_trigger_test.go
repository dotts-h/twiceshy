// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// recordPushDecision must record the caller's trigger on the gate-decision line
// (#0116): the served-rate split needs prompt-vs-error to be recoverable from the
// log. "" and "prompt" are semantically identical (ADR-0028 decision 4), so both
// normalize to "prompt" for the log rather than leaving the field empty.
func TestRecordPushDecision_TriggerNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel}
	h.recordPushDecision("q-empty", index.PushDecision{}, "", "")
	h.recordPushDecision("q-prompt", index.PushDecision{}, "", "prompt")
	h.recordPushDecision("q-error", index.PushDecision{}, "", "error")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	got := readLines(t, path)
	if len(got) != 3 {
		t.Fatalf("want 3 decisions, got %d", len(got))
	}
	if got[0].Trigger != "prompt" {
		t.Errorf(`trigger="" must normalize to "prompt", got %q`, got[0].Trigger)
	}
	if got[1].Trigger != "prompt" {
		t.Errorf(`trigger="prompt" must stay "prompt", got %q`, got[1].Trigger)
	}
	if got[2].Trigger != "error" {
		t.Errorf(`trigger="error" must record as-is, got %q`, got[2].Trigger)
	}
}

// The search channel never sets a trigger — no `trigger` key on the line at all,
// same as before #0116.
func TestRecordSearchDecision_NoTriggerKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel}
	h.recordSearchDecision("q1", []index.Hit{{ID: "exp-0001", Score: 1}}, "")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	line := readRawLine(t, path)
	if strings.Contains(line, `"trigger"`) {
		t.Fatalf("search channel must never carry a trigger key: %s", line)
	}
}

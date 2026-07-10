// SPDX-License-Identifier: AGPL-3.0-only

package measurement_test

import (
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/measurement"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

func TestGenerateBaselineTreatmentAndConfidenceSummaries(t *testing.T) {
	session := "0123456789abcdef0123456789abcdef"
	decisions := []telemetry.Decision{
		{Time: "2026-07-01T01:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q1"},
		{Time: "2026-07-01T02:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q1"},
		{Time: "2026-07-01T03:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q1"},
		{Time: "2026-07-08T01:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q2", Served: []telemetry.ServedHit{{ID: "exp-0001"}}, Count: 1},
		{Time: "2026-07-08T02:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q2", Served: []telemetry.ServedHit{{ID: "exp-0001"}}, Count: 1},
		{Time: "2026-07-08T03:00:00Z", Channel: "push", Trigger: "error", Session: session, QueryHash: "q3"},
	}
	used, no := true, false
	outcomes := []measurement.Outcome{
		{Time: "2026-07-08T04:00:00Z", Session: session, RecordID: "exp-0001", Used: &used, Confirmed: true},
		{Time: "2026-07-08T05:00:00Z", Session: session, RecordID: "exp-0001", Used: &no, Incorrect: true},
	}
	cfg := measurement.Config{
		Baseline:  measurement.Window{Start: mustTime(t, "2026-07-01T00:00:00Z"), End: mustTime(t, "2026-07-02T00:00:00Z")},
		Treatment: measurement.Window{Start: mustTime(t, "2026-07-08T00:00:00Z"), End: mustTime(t, "2026-07-09T00:00:00Z")},
		Cohorts:   map[string]string{session: "team-a"},
	}
	rep, err := measurement.Generate(cfg, decisions, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Baseline.Metrics.Decisions != 3 || rep.Baseline.Metrics.RepeatedErrors != 2 || rep.Baseline.Metrics.Exposures != 0 {
		t.Errorf("baseline = %+v", rep.Baseline.Metrics)
	}
	m := rep.Treatment.Metrics
	if m.Decisions != 3 || m.ExposedDecisions != 2 || m.Exposures != 2 || m.Judged != 2 || m.Used != 1 || m.Confirmed != 1 || m.Incorrect != 1 || m.RepeatedErrors != 1 {
		t.Errorf("treatment = %+v", m)
	}
	if m.HitRate.Value != 2.0/3 || m.UsedRate.Value != .5 || m.HelpfulRate.Value != .5 || m.IncorrectRate.Value != .5 || m.OutcomeCoverage.Value != 1 {
		t.Errorf("rates = %+v", m)
	}
	if m.UsedRate.Low <= 0 || m.UsedRate.High >= 1 {
		t.Errorf("Wilson interval not confidence-aware: %+v", m.UsedRate)
	}
	if len(rep.Teams) != 2 || rep.Teams[0].Team != "team-a" || rep.Teams[0].Arm != "baseline" || rep.Teams[1].Arm != "treatment" {
		t.Errorf("team summaries order/content = %+v", rep.Teams)
	}
	if len(rep.Records) != 1 || rep.Records[0].RecordID != "exp-0001" || rep.Records[0].Metrics.Exposures != 2 {
		t.Errorf("record summaries = %+v", rep.Records)
	}
}

func TestGenerateRejectsOverlappingWindows(t *testing.T) {
	_, err := measurement.Generate(measurement.Config{
		Baseline:  measurement.Window{Start: mustTime(t, "2026-07-01T00:00:00Z"), End: mustTime(t, "2026-07-03T00:00:00Z")},
		Treatment: measurement.Window{Start: mustTime(t, "2026-07-02T00:00:00Z"), End: mustTime(t, "2026-07-04T00:00:00Z")},
	}, nil, nil)
	if err == nil {
		t.Fatal("overlapping windows must fail")
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

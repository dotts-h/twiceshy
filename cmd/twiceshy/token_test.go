// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
)

func TestFormatTokenListLines(t *testing.T) {
	revoked := "2026-07-06T12:00:00Z"
	lines := formatTokenListLines([]index.TokenInfo{
		{ID: "tok_ab12cd34", Label: "alice", DailyQuota: 1000, RatePerMin: 60, CallsToday: 42},
		{ID: "tok_deadbeef", Label: "bob", DailyQuota: 0, RatePerMin: 0, CallsToday: 0, RevokedAt: &revoked},
	})
	if len(lines) != 2 {
		t.Fatalf("len = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[0], "tok_ab12cd34") || !strings.Contains(lines[0], "active") || !strings.Contains(lines[0], "calls_today=42") {
		t.Fatalf("active line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "revoked") {
		t.Fatalf("revoked line = %q", lines[1])
	}
	if strings.Contains(strings.Join(lines, "\n"), "secret") {
		t.Fatal("list output must never mention secrets")
	}
}

func TestTeamPlanCLIIsDisabledByDefault(t *testing.T) {
	var out bytes.Buffer
	db := filepath.Join(t.TempDir(), "ix.db")
	err := runToken(context.Background(), []string{"report", "-index", db}, &out, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_TEAM_PLANS") {
		t.Fatalf("disabled team-plan report error = %v", err)
	}
	err = runToken(context.Background(), []string{"issue", "-index", db, "-plan", "team", "-organization", "org", "-workspace", "ws"}, &out, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_TEAM_PLANS") {
		t.Fatalf("disabled team-plan issue error = %v", err)
	}
	if _, statErr := os.Stat(db); !os.IsNotExist(statErr) {
		t.Fatalf("disabled feature must not create a registry, stat err=%v", statErr)
	}
}

func TestTeamPlanCLIReportAndAssignment(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ix.db")
	enabled := func(key string) string {
		if key == "TWICESHY_TEAM_PLANS" {
			return "1"
		}
		return ""
	}
	var issued bytes.Buffer
	if err := runToken(context.Background(), []string{"issue", "-index", db, "-label", "platform", "-plan", "team", "-organization", "org_acme", "-workspace", "ws_platform"}, &issued, enabled); err != nil {
		t.Fatalf("planned token issue: %v", err)
	}
	var report bytes.Buffer
	if err := runToken(context.Background(), []string{"report", "-index", db}, &report, enabled); err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, want := range []string{"org_acme", "ws_platform", "plan=team", "quota=20000", "rate=600"} {
		if !strings.Contains(report.String(), want) {
			t.Errorf("report %q missing %q", report.String(), want)
		}
	}
}

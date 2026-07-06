// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
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

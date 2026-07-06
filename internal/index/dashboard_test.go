// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

func dashboardRec(id, status string) *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "trap", Status: status,
		Title: "a record for dashboard stats tests, long enough title " + id,
		Path:  "experience/2026/" + id[4:] + "-x.md",
		Provenance: record.Provenance{
			Source:     record.Source{Author: "test"},
			RecordedAt: "2026-06-19",
			Valid:      record.Validity{From: "2026-06-19"},
		},
	}
}

func dashboardIndex(t *testing.T, recs ...*record.Record) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return ix
}

func TestRecordStatusCounts(t *testing.T) {
	ix := dashboardIndex(t,
		dashboardRec("exp-0100", "validated"),
		dashboardRec("exp-0101", "validated"),
		dashboardRec("exp-0102", "quarantined"),
		dashboardRec("exp-0103", "stale"),
	)
	got, err := ix.RecordStatusCounts(context.Background())
	if err != nil {
		t.Fatalf("RecordStatusCounts: %v", err)
	}
	if got.Validated != 2 {
		t.Errorf("validated = %d, want 2", got.Validated)
	}
	if got.Quarantined != 1 {
		t.Errorf("quarantined = %d, want 1", got.Quarantined)
	}
	if got.Total != 4 {
		t.Errorf("total = %d, want 4 (every status counted)", got.Total)
	}
}

func TestRecordStatusCountsEmpty(t *testing.T) {
	ix := dashboardIndex(t)
	got, err := ix.RecordStatusCounts(context.Background())
	if err != nil {
		t.Fatalf("RecordStatusCounts: %v", err)
	}
	if got.Validated != 0 || got.Quarantined != 0 || got.Total != 0 {
		t.Fatalf("empty corpus counts = %+v, want all zero", got)
	}
}

func TestTenantStatsCallsAndWindow(t *testing.T) {
	ix := dashboardIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, id, err := ix.IssueToken("alice", 1000, 60, now)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if _, err := ix.CountTokenCall(id, now); err != nil {
		t.Fatalf("CountTokenCall: %v", err)
	}

	// Today: two search_experience calls.
	for i := 0; i < 2; i++ {
		if err := ix.CountTenantCall(id, "search_experience", now); err != nil {
			t.Fatalf("CountTenantCall: %v", err)
		}
	}
	// 5 days ago: inside the 7d window.
	if err := ix.CountTenantCall(id, "get_experience", now.AddDate(0, 0, -5)); err != nil {
		t.Fatalf("CountTenantCall (5d ago): %v", err)
	}
	// 10 days ago: outside the 7d window.
	if err := ix.CountTenantCall(id, "get_experience", now.AddDate(0, 0, -10)); err != nil {
		t.Fatalf("CountTenantCall (10d ago): %v", err)
	}

	stats, err := ix.TenantStats(context.Background(), now)
	if err != nil {
		t.Fatalf("TenantStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("TenantStats len = %d, want 1", len(stats))
	}
	s := stats[0]
	if s.ID != id || s.Label != "alice" || s.Revoked || s.DailyQuota != 1000 {
		t.Fatalf("TenantStats = %+v", s)
	}
	if s.CallsToday != 1 {
		t.Fatalf("calls_today = %d, want 1 (CountTokenCall bump only)", s.CallsToday)
	}
	if s.Calls7d != 3 {
		t.Fatalf("calls_7d = %d, want 3 (2 today + 1 five days ago; the 10-day-old call must be excluded)", s.Calls7d)
	}
	if s.TopTools["search_experience"] != 2 {
		t.Fatalf("top_tools[search_experience] = %d, want 2", s.TopTools["search_experience"])
	}
	if s.TopTools["get_experience"] != 1 {
		t.Fatalf("top_tools[get_experience] = %d, want 1 (only the in-window call)", s.TopTools["get_experience"])
	}
}

func TestTenantStatsIncludesRevoked(t *testing.T) {
	ix := dashboardIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	_, id, err := ix.IssueToken("bob", 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.RevokeToken(id, now); err != nil {
		t.Fatal(err)
	}
	stats, err := ix.TenantStats(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || !stats[0].Revoked {
		t.Fatalf("TenantStats = %+v, want one revoked tenant", stats)
	}
}

func TestTopRecordsOrderedByUsage(t *testing.T) {
	ix := dashboardIndex(t,
		dashboardRec("exp-0100", "validated"),
		dashboardRec("exp-0101", "validated"),
		dashboardRec("exp-0102", "validated"),
	)
	ctx := context.Background()
	if err := ix.RecordHits(ctx, []string{"exp-0100", "exp-0100", "exp-0101"}, "2026-06-19"); err != nil {
		t.Fatal(err)
	}
	if err := ix.RecordPushes(ctx, []string{"exp-0101", "exp-0101"}); err != nil {
		t.Fatal(err)
	}
	// exp-0100: retrieved=2, pushed=0 -> 2
	// exp-0101: retrieved=1, pushed=2 -> 3
	// exp-0102: no usage row at all -> must be excluded, not zero-padded in.

	top, err := ix.TopRecords(ctx, 10)
	if err != nil {
		t.Fatalf("TopRecords: %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("TopRecords len = %d, want 2 (only records with usage)", len(top))
	}
	if top[0].ID != "exp-0101" || top[0].Retrieved != 1 || top[0].Pushed != 2 {
		t.Fatalf("top[0] = %+v, want exp-0101 retrieved=1 pushed=2 (highest total first)", top[0])
	}
	if top[1].ID != "exp-0100" || top[1].Retrieved != 2 || top[1].Pushed != 0 {
		t.Fatalf("top[1] = %+v, want exp-0100 retrieved=2 pushed=0", top[1])
	}
	if top[0].Title == "" {
		t.Error("TopRecords must carry the record title")
	}
}

func TestTopRecordsRespectsLimit(t *testing.T) {
	var recs []*record.Record
	for i := 0; i < 5; i++ {
		id := dashboardRecID(100 + i)
		recs = append(recs, dashboardRec(id, "validated"))
	}
	ix := dashboardIndex(t, recs...)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		id := dashboardRecID(100 + i)
		if err := ix.RecordHits(ctx, []string{id}, "2026-06-19"); err != nil {
			t.Fatal(err)
		}
	}
	top, err := ix.TopRecords(ctx, 2)
	if err != nil {
		t.Fatalf("TopRecords: %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("TopRecords len = %d, want 2 (respects the limit)", len(top))
	}
}

func dashboardRecID(n int) string {
	return fmt.Sprintf("exp-%04d", n)
}

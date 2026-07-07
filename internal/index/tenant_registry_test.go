// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestRebuildPreservesTenantRegistry(t *testing.T) {
	rec := &record.Record{
		SchemaVersion: 1, ID: "exp-0100", Kind: "trap", Status: "validated",
		Title: "a record for rebuild preservation tests, long enough title",
		Path:  "experience/2026/0100-x.md",
		Provenance: record.Provenance{
			Source:     record.Source{Author: "test"},
			RecordedAt: "2026-06-19",
			Valid:      record.Validity{From: "2026-06-19"},
		},
	}
	recs := []*record.Record{rec}

	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	ctx := context.Background()
	if err := ix.Rebuild(ctx, recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	// Seed every tenant-registry table: a token, a quota debit, a telemetry
	// bump, and a contribution debit.
	fullToken, tokenID, err := ix.IssueToken("test-tenant", 100, 60, now)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	_, allowed, err := ix.CountTokenCall(tokenID, now)
	if err != nil {
		t.Fatalf("CountTokenCall: %v", err)
	}
	if !allowed {
		t.Fatal("CountTokenCall not allowed")
	}

	if err := ix.CountTenantCall(tokenID, "search_experience", now); err != nil {
		t.Fatalf("CountTenantCall: %v", err)
	}

	_, allowed, err = ix.CountContributionCall(tokenID, "record_experience", 10, now)
	if err != nil {
		t.Fatalf("CountContributionCall: %v", err)
	}
	if !allowed {
		t.Fatal("CountContributionCall not allowed")
	}

	// A full rebuild over the same corpus — the operation ADR-0034 pins as
	// registry-safe.
	if err := ix.Rebuild(ctx, recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Every table must have survived: same token authenticates, every counter
	// kept its value and keeps counting.
	info, err := ix.AuthenticateToken(fullToken, now)
	if err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	if info.ID != tokenID {
		t.Errorf("AuthenticateToken returned ID %q, want %q", info.ID, tokenID)
	}
	if info.CallsToday != 1 {
		t.Errorf("CallsToday = %d, want 1", info.CallsToday)
	}

	tenantCalls, err := ix.TenantToolCallsToday(tokenID, "search_experience", now)
	if err != nil {
		t.Fatalf("TenantToolCallsToday: %v", err)
	}
	if tenantCalls != 1 {
		t.Errorf("TenantToolCallsToday = %d, want 1", tenantCalls)
	}

	contribCalls, allowed, err := ix.CountContributionCall(tokenID, "record_experience", 10, now)
	if err != nil {
		t.Fatalf("CountContributionCall after rebuild: %v", err)
	}
	if !allowed {
		t.Error("CountContributionCall after rebuild not allowed")
	}
	if contribCalls != 2 {
		t.Errorf("CountContributionCall calls = %d, want 2", contribCalls)
	}

	tokens, err := ix.ListTokens(now)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	found := false
	for _, tok := range tokens {
		if tok.ID == tokenID {
			found = true
			if tok.CallsToday != 1 {
				t.Errorf("ListTokens CallsToday = %d, want 1", tok.CallsToday)
			}
		}
	}
	if !found {
		t.Errorf("token %q was not found in ListTokens list %+v", tokenID, tokens)
	}
}

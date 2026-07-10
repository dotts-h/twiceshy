// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/entitlement"
	"github.com/dotts-h/twiceshy/internal/index"
	_ "modernc.org/sqlite"
)

func TestIssuePlannedTokenAssociatesWorkspaceAndDerivedQuota(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	full, id, err := ix.IssuePlannedToken("platform", "org_acme", "ws_platform", entitlement.Team, now)
	if err != nil {
		t.Fatalf("IssuePlannedToken: %v", err)
	}
	info, err := ix.AuthenticateToken(full, now)
	if err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	if info.ID != id || info.OrganizationID != "org_acme" || info.WorkspaceID != "ws_platform" || info.Plan != entitlement.Team {
		t.Fatalf("token info = %+v", info)
	}
	if info.DailyQuota != 20000 || info.RatePerMin != 600 {
		t.Fatalf("derived quotas = %d/%d", info.DailyQuota, info.RatePerMin)
	}
}

func TestOpenMigratesLegacyTenantSchemaWithoutRewritingTokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("0123456789abcdef", 2)
	hash := sha256.Sum256([]byte(secret))
	_, err = raw.Exec(`CREATE TABLE tokens (id TEXT PRIMARY KEY, secret_hash BLOB NOT NULL, label TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, revoked_at TEXT, daily_quota INTEGER NOT NULL DEFAULT 1000, rate_per_min INTEGER NOT NULL DEFAULT 60);
		CREATE TABLE token_usage (token_id TEXT NOT NULL, day TEXT NOT NULL, calls INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(token_id, day));
		INSERT INTO tokens VALUES ('tok_0123abcd', ?, 'legacy', '2026-07-10T12:00:00Z', NULL, 77, 11);`, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	ix, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("Open legacy db: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	info, err := ix.AuthenticateToken("tok_0123abcd_"+secret, time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("legacy token after migration: %v", err)
	}
	if info.Label != "legacy" || info.DailyQuota != 77 || info.Plan != "" {
		t.Fatalf("legacy token rewritten: %+v", info)
	}
	if err := ix.AssignTokenPlan(info.ID, "org_migrated", "ws_migrated", entitlement.Community, time.Now()); err != nil {
		t.Fatalf("new schema unavailable: %v", err)
	}
}

func TestAssignPlanMigratesLegacyTokenAndRebuildPreservesIt(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	full, id, err := ix.IssueToken("legacy", 1000, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.AssignTokenPlan(id, "org_acme", "ws_legacy", entitlement.Pro, now); err != nil {
		t.Fatalf("AssignTokenPlan: %v", err)
	}
	if err := ix.Rebuild(context.Background(), nil, "test/repo"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	info, err := ix.AuthenticateToken(full, now)
	if err != nil {
		t.Fatal(err)
	}
	if info.Plan != entitlement.Pro || info.DailyQuota != 5000 || info.WorkspaceID != "ws_legacy" {
		t.Fatalf("migrated token after rebuild = %+v", info)
	}
	report, err := ix.PlanReport(context.Background())
	if err != nil || len(report) != 1 || report[0].ID != id {
		t.Fatalf("PlanReport = %+v, %v", report, err)
	}
}

func TestLegacyTokenRemainsUnassigned(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	now := time.Now()
	full, _, err := ix.IssueToken("alpha", 77, 11, now)
	if err != nil {
		t.Fatal(err)
	}
	info, err := ix.AuthenticateToken(full, now)
	if err != nil {
		t.Fatal(err)
	}
	if info.Plan != "" || info.OrganizationID != "" || info.WorkspaceID != "" || info.DailyQuota != 77 || info.RatePerMin != 11 {
		t.Fatalf("legacy behavior changed: %+v", info)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
)

func tokenIndex(t *testing.T) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	return ix
}

func TestIssueAuthenticateRoundtrip(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	full, id, err := ix.IssueToken("alice", 1000, 60, now)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if id == "" || full == "" {
		t.Fatal("IssueToken must return non-empty id and full token")
	}
	if full[:len(id)] != id {
		t.Fatalf("full token %q must start with id %q", full, id)
	}

	info, err := ix.AuthenticateToken(full, now)
	if err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	if info.ID != id || info.Label != "alice" || info.DailyQuota != 1000 || info.RatePerMin != 60 {
		t.Fatalf("AuthenticateToken = %+v, want id=%s label=alice quota=1000 rate=60", info, id)
	}
	if info.RevokedAt != nil {
		t.Fatalf("new token must not be revoked, got %v", info.RevokedAt)
	}
}

func TestAuthenticateWrongSecretFails(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	full, id, err := ix.IssueToken("", 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	bad := full[:len(full)-1] + "x"
	if bad == full {
		t.Fatal("bad token must differ from full")
	}
	_, err = ix.AuthenticateToken(bad, now)
	if !errors.Is(err, index.ErrTokenUnknown) {
		t.Fatalf("wrong secret: got %v, want ErrTokenUnknown", err)
	}
	// id prefix alone must not authenticate.
	_, err = ix.AuthenticateToken(id, now)
	if !errors.Is(err, index.ErrTokenUnknown) {
		t.Fatalf("id-only: got %v, want ErrTokenUnknown", err)
	}
}

func TestAuthenticateRevokedFails(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	full, id, err := ix.IssueToken("bob", 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.RevokeToken(id, now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	_, err = ix.AuthenticateToken(full, now.Add(time.Minute))
	if !errors.Is(err, index.ErrTokenRevoked) {
		t.Fatalf("revoked token: got %v, want ErrTokenRevoked", err)
	}
}

func TestAuthenticateUnknownIDFails(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, err := ix.AuthenticateToken("tok_deadbeef_"+stringsRepeat("a", 32), now)
	if !errors.Is(err, index.ErrTokenUnknown) {
		t.Fatalf("unknown id: got %v, want ErrTokenUnknown", err)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}

func TestCountTokenCallIncrementsAtomically(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, id, err := ix.IssueToken("", 0, 0, now)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, perG = 8, 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				if _, _, err := ix.CountTokenCall(id, now); err != nil {
					t.Errorf("CountTokenCall: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	list, err := ix.ListTokens(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].CallsToday != goroutines*perG {
		t.Fatalf("calls today = %d, want %d", list[0].CallsToday, goroutines*perG)
	}
}

// TestCountTokenCallCapsAtQuota is #0131 finding 2: the old CountTokenCall
// incremented unconditionally and let tenantAuth reject only AFTER the bump
// (count-then-check), so an over-quota tenant's stored calls counter grew
// without bound — every rejected call still cost a row update. The atomic
// conditional UPDATE must stop incrementing once daily_quota is reached: quota
// N admits exactly N calls/day, and the stored total never exceeds N no matter
// how many more calls arrive.
func TestCountTokenCallCapsAtQuota(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, id, err := ix.IssueToken("", 3, 0, now)
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 10; i++ {
		calls, allowed, err := ix.CountTokenCall(id, now)
		if err != nil {
			t.Fatalf("call %d: CountTokenCall: %v", i, err)
		}
		wantAllowed := i <= 3
		if allowed != wantAllowed {
			t.Fatalf("call %d: allowed = %v, want %v", i, allowed, wantAllowed)
		}
		if calls > 3 {
			t.Fatalf("call %d: stored calls = %d, must never exceed quota 3", i, calls)
		}
	}

	list, err := ix.ListTokens(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].CallsToday != 3 {
		t.Fatalf("calls today = %d, want 3 (capped at quota, not 10)", list[0].CallsToday)
	}
}

// TestCountTokenCallQuotaCapRaceSafe drives concurrent calls at the quota
// boundary: the check-then-increment must be one atomic SQLite statement, not
// a Go-side read-then-write, or concurrent goroutines can all pass a stale
// check and push the stored total past the quota. Guards under `go test -race`.
func TestCountTokenCallQuotaCapRaceSafe(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	const quota = 5
	_, id, err := ix.IssueToken("", quota, 0, now)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, perG = 10, 10
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				if _, _, err := ix.CountTokenCall(id, now); err != nil {
					t.Errorf("CountTokenCall: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	list, err := ix.ListTokens(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].CallsToday != quota {
		t.Fatalf("calls today = %d, want exactly %d (never over the quota under race)", list[0].CallsToday, quota)
	}
}

func TestCountTokenCallDayRolloverUTC(t *testing.T) {
	ix := tokenIndex(t)
	day1 := time.Date(2026, 7, 6, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 7, 0, 1, 0, 0, time.UTC)

	_, id, err := ix.IssueToken("", 0, 0, day1)
	if err != nil {
		t.Fatal(err)
	}
	c1, _, err := ix.CountTokenCall(id, day1)
	if err != nil || c1 != 1 {
		t.Fatalf("day1 count = %d err=%v, want 1", c1, err)
	}
	c2, _, err := ix.CountTokenCall(id, day2)
	if err != nil || c2 != 1 {
		t.Fatalf("day2 count = %d err=%v, want 1 (UTC day rollover)", c2, err)
	}

	tokens, err := ix.ListTokens(day2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].CallsToday != 1 {
		t.Fatalf("ListTokens day2 = %+v, want calls_today=1", tokens)
	}
}

func TestRevokeTokenUnknownFails(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	err := ix.RevokeToken("tok_nosuch00", now)
	if !errors.Is(err, index.ErrTokenUnknown) {
		t.Fatalf("RevokeToken unknown: got %v, want ErrTokenUnknown", err)
	}
}

// TestCountTenantCallUpsertsPerTool guards the upsert math via TenantStats: two
// calls for one tool must increment together, a different tool for the same
// tenant must keep its own separate counter.
func TestCountTenantCallUpsertsPerTool(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	_, id, err := ix.IssueToken("carol", 1000, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ix.CountTenantCall(id, "search_experience", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := ix.CountTenantCall(id, "push", now); err != nil {
		t.Fatal(err)
	}

	stats, err := ix.TenantStats(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("TenantStats len = %d, want 1", len(stats))
	}
	got := stats[0]
	if got.TopTools["search_experience"] != 2 {
		t.Fatalf("search_experience calls = %d, want 2", got.TopTools["search_experience"])
	}
	if got.TopTools["push"] != 1 {
		t.Fatalf("push calls = %d, want 1 (per-tool separation)", got.TopTools["push"])
	}
}

func TestCountTenantCallConcurrentIncrements(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	_, id, err := ix.IssueToken("dana", 1000, 60, now)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, perG = 8, 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				if err := ix.CountTenantCall(id, "search_experience", now); err != nil {
					t.Errorf("CountTenantCall: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	stats, err := ix.TenantStats(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].TopTools["search_experience"] != goroutines*perG {
		t.Fatalf("TenantStats = %+v, want search_experience=%d", stats, goroutines*perG)
	}
}

func TestListTokensIncludesTodayCalls(t *testing.T) {
	ix := tokenIndex(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, id, err := ix.IssueToken("carol", 500, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, _, err := ix.CountTokenCall(id, now); err != nil {
			t.Fatal(err)
		}
	}

	list, err := ix.ListTokens(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("ListTokens len = %d, want 1", len(list))
	}
	tok := list[0]
	if tok.ID != id || tok.Label != "carol" || tok.CallsToday != 3 || tok.DailyQuota != 500 || tok.RatePerMin != 30 {
		t.Fatalf("ListTokens = %+v, want id=%s label=carol calls=3", tok, id)
	}
}

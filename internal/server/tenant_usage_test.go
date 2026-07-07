// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
)

// fakeTenantCallRecorder is a stub tenantCallRecorder (#0126): records every
// call it sees and can be primed to fail, so tests can assert the "log and
// continue, never fail the request" contract without a real index.
type fakeTenantCallRecorder struct {
	mu    sync.Mutex
	calls []struct{ tenant, tool string }
	err   error
}

func (f *fakeTenantCallRecorder) CountTenantCall(tokenID, tool string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct{ tenant, tool string }{tokenID, tool})
	return f.err
}

func (f *fakeTenantCallRecorder) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestRecordTenantCallSkipsEmptyTenant: an unauthenticated / pre-auth context
// (TenantFromContext == "") must never reach the store — recording a "" tenant
// would be meaningless telemetry.
func TestRecordTenantCallSkipsEmptyTenant(t *testing.T) {
	f := &fakeTenantCallRecorder{}
	h := &handlers{logger: quietLogger(), tenantCalls: f}
	h.recordTenantCall(context.Background(), "search_experience")
	if f.len() != 0 {
		t.Fatalf("got %d calls, want 0 (empty tenant must be skipped)", f.len())
	}
}

// TestRecordTenantCallRecordsForOperatorAndTenant: both "operator" and a tok_
// tenant id are valid recordable tenants.
func TestRecordTenantCallRecordsForOperatorAndTenant(t *testing.T) {
	f := &fakeTenantCallRecorder{}
	h := &handlers{logger: quietLogger(), tenantCalls: f}

	h.recordTenantCall(withTenant(context.Background(), "operator"), "get_experience")
	h.recordTenantCall(withTenant(context.Background(), "tok_abcd1234"), "search_experience")

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(f.calls))
	}
	if f.calls[0].tenant != "operator" || f.calls[0].tool != "get_experience" {
		t.Errorf("call 0 = %+v", f.calls[0])
	}
	if f.calls[1].tenant != "tok_abcd1234" || f.calls[1].tool != "search_experience" {
		t.Errorf("call 1 = %+v", f.calls[1])
	}
}

// TestRecordTenantCallLogsAndContinuesOnError guards the hard rule: a failing
// tenant-usage write is logged, never propagated or panicked on.
func TestRecordTenantCallLogsAndContinuesOnError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	f := &fakeTenantCallRecorder{err: errors.New("db is closed")}
	h := &handlers{logger: logger, tenantCalls: f}

	h.recordTenantCall(withTenant(context.Background(), "operator"), "search_experience")

	if f.len() != 1 {
		t.Fatalf("store was not called: got %d, want 1 (the failing write must still be attempted)", f.len())
	}
	if !strings.Contains(buf.String(), "tenant usage record failed") {
		t.Fatalf("a failed tenant-usage write must be logged at Warn; logs:\n%s", buf.String())
	}
}

// TestRecordTenantCallNilRecorderSafe: a handlers with no tenantCalls wired
// (the zero value used by unit tests that construct &handlers{} directly)
// must be a silent no-op, not a nil-pointer panic.
func TestRecordTenantCallNilRecorderSafe(t *testing.T) {
	h := &handlers{logger: quietLogger()}
	h.recordTenantCall(withTenant(context.Background(), "operator"), "search_experience")
}

// fakeContributionQuota is a stub contributionQuota (ADR-0032): every call is
// recorded and it can be primed to always error, so fail-closed tests don't
// need a real index.
type fakeContributionQuota struct {
	mu    sync.Mutex
	calls []struct {
		tokenID, tool string
		limit         int
	}
	err error
}

func (f *fakeContributionQuota) CountContributionCall(tokenID, tool string, limit int, _ time.Time) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		tokenID, tool string
		limit         int
	}{tokenID, tool, limit})
	if f.err != nil {
		return 0, false, f.err
	}
	return 0, true, nil
}

func (f *fakeContributionQuota) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// countingContributionQuota is a stub contributionQuota that actually enforces
// limit per tokenID+tool, so tests can drive it through a real boundary
// without a live index — used to prove enforcement is independent of the
// telemetry path (issue 0135's repro).
type countingContributionQuota struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *countingContributionQuota) CountContributionCall(tokenID, tool string, limit int, _ time.Time) (int, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	key := tokenID + "|" + tool
	if limit > 0 && c.counts[key] >= limit {
		return c.counts[key], false, nil
	}
	c.counts[key]++
	return c.counts[key], true, nil
}

// TestCheckContributionQuotaFailClosedOnStoreError is ADR-0032's core
// guarantee: a failing contribQuota store REJECTS the alpha write, the
// opposite default from read-path telemetry (#0126's best-effort contract).
func TestCheckContributionQuotaFailClosedOnStoreError(t *testing.T) {
	store := &fakeContributionQuota{err: errors.New("database is locked")}
	h := &handlers{logger: quietLogger(), contribQuota: store}
	ctx := withTenant(context.Background(), "tok_abcd1234")

	if err := h.checkContributionQuota(ctx, "record_experience", 10); err == nil {
		t.Fatal("checkContributionQuota with a failing store must reject the call, got nil error")
	}
	if n := store.len(); n != 1 {
		t.Fatalf("store was not called: got %d, want 1", n)
	}
}

// TestCheckContributionQuotaFailClosedOnNilSeam is ADR-0032's other fail-closed
// half: a handlers constructed without the contribQuota seam wired (e.g. a
// misconfigured New, or a unit test that forgot it) must REJECT alpha writes,
// never silently admit them.
func TestCheckContributionQuotaFailClosedOnNilSeam(t *testing.T) {
	h := &handlers{logger: quietLogger()} // contribQuota left nil
	ctx := withTenant(context.Background(), "tok_abcd1234")

	if err := h.checkContributionQuota(ctx, "record_experience", 10); err == nil {
		t.Fatal("checkContributionQuota with a nil contribQuota seam must reject the call, got nil error")
	}
}

// TestCheckContributionQuotaEnforcesIndependentOfTelemetryFailure is issue
// 0135's repro: with the tenant_usage telemetry recorder failing on every
// call (its best-effort contract swallows the error), the contribution quota
// must still admit exactly `limit` calls and reject the next one — the
// regression the old tenant_usage-backed design had.
func TestCheckContributionQuotaEnforcesIndependentOfTelemetryFailure(t *testing.T) {
	tenantCalls := &fakeTenantCallRecorder{err: errors.New("database is locked")}
	contrib := &countingContributionQuota{}
	h := &handlers{logger: quietLogger(), tenantCalls: tenantCalls, contribQuota: contrib}
	ctx := withTenant(context.Background(), "tok_abcd1234")

	for i := 1; i <= 10; i++ {
		h.recordTenantCall(ctx, "record_experience") // best-effort telemetry: fails and is swallowed
		if err := h.checkContributionQuota(ctx, "record_experience", 10); err != nil {
			t.Fatalf("call %d/10 must be admitted despite telemetry failing: %v", i, err)
		}
	}
	h.recordTenantCall(ctx, "record_experience")
	if err := h.checkContributionQuota(ctx, "record_experience", 10); err == nil {
		t.Fatal("the 11th call must be rejected (limit 10) even though telemetry never successfully recorded one")
	}
	if tenantCalls.len() != 11 {
		t.Fatalf("telemetry recorder saw %d attempts, want 11 (telemetry is attempted independent of quota outcome)", tenantCalls.len())
	}
}

// TestCheckContributionQuotaOperatorUnaffected guards that the operator tenant
// is exempt from the contribution quota unconditionally — even with a nil seam
// or a failing store, the checks that would fail-closed for an alpha tenant.
func TestCheckContributionQuotaOperatorUnaffected(t *testing.T) {
	ctx := withTenant(context.Background(), "operator")

	nilSeam := &handlers{logger: quietLogger()}
	if err := nilSeam.checkContributionQuota(ctx, "record_experience", 1); err != nil {
		t.Fatalf("operator must be exempt even with a nil contribQuota seam: %v", err)
	}

	failing := &handlers{logger: quietLogger(), contribQuota: &fakeContributionQuota{err: errors.New("boom")}}
	if err := failing.checkContributionQuota(ctx, "record_experience", 1); err != nil {
		t.Fatalf("operator must be exempt even when the store errors: %v", err)
	}
}

// TestPushHTTPRecordsTenantCallToolPush guards the schema's documented "push"
// tool value (#0126): the POST /push edge, not just MCP tools/call, bumps the
// calling tenant's counter under tool="push".
func TestPushHTTPRecordsTenantCallToolPush(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })

	f := &fakeTenantCallRecorder{}
	h := &handlers{ix: ix, logger: quietLogger(), tenantCalls: f}
	h.usage = newUsageRecorder(ix, quietLogger(), time.Now)

	body := strings.NewReader(`{"query":"anything"}`)
	req := httptest.NewRequest(http.MethodPost, "/push", body)
	req = req.WithContext(withTenant(req.Context(), "tok_push0001"))
	rec := httptest.NewRecorder()
	h.pushHTTP(rec, req)

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 || f.calls[0].tenant != "tok_push0001" || f.calls[0].tool != "push" {
		t.Fatalf("recorded calls = %+v, want one push call for tok_push0001", f.calls)
	}
}

// TestRetroHTTPRecordsTenantCallToolRetro guards the schema's documented
// "retro" tool value (#0126).
func TestRetroHTTPRecordsTenantCallToolRetro(t *testing.T) {
	f := &fakeTenantCallRecorder{}
	h := &handlers{logger: quietLogger(), tenantCalls: f, retroQueue: t.TempDir()}

	body := strings.NewReader(`{"transcript":"a session transcript long enough to pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/retro", body)
	req = req.WithContext(withTenant(req.Context(), "tok_retro001"))
	rec := httptest.NewRecorder()
	h.retroHTTP(rec, req)

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 || f.calls[0].tenant != "tok_retro001" || f.calls[0].tool != "retro" {
		t.Fatalf("recorded calls = %+v, want one retro call for tok_retro001", f.calls)
	}
}

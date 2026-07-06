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

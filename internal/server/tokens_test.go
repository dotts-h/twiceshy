// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
)

const tenantOpToken = "s3cret-operator-token"

type stubTokenStore struct {
	mu      sync.Mutex
	auth    func(full string, now time.Time) (index.TokenInfo, error)
	count   func(id string, now time.Time) (int, bool, error)
	countN  map[string]int
	quotas  map[string]int // id -> daily quota for the default CountTokenCall's cap check; 0/absent = unlimited
	authFn  map[string]index.TokenInfo
	revoked map[string]bool
}

func (s *stubTokenStore) AuthenticateToken(full string, now time.Time) (index.TokenInfo, error) {
	if s.auth != nil {
		return s.auth(full, now)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.authFn[full]
	if !ok {
		return index.TokenInfo{}, index.ErrTokenUnknown
	}
	if s.revoked[info.ID] {
		return index.TokenInfo{}, index.ErrTokenRevoked
	}
	return info, nil
}

// CountTokenCall mirrors the production cap semantics (index.Index.CountTokenCall,
// #0131): quota <= 0 (absent from quotas) is unlimited; otherwise it admits
// exactly quota calls/day and leaves the stored count unchanged afterward.
func (s *stubTokenStore) CountTokenCall(id string, now time.Time) (int, bool, error) {
	if s.count != nil {
		return s.count(id, now)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.countN == nil {
		s.countN = make(map[string]int)
	}
	quota := s.quotas[id]
	if quota > 0 && s.countN[id] >= quota {
		return s.countN[id], false, nil
	}
	s.countN[id]++
	return s.countN[id], true, nil
}

// tenantProbeHandler builds the same three-middleware order as server.New
// (#0131): tenantAuth (auth + per-token rate), then the global limiter, then
// withDailyQuota — so a test exercising this handler sees production ordering,
// including that a global 429 must run before (and so never burns) the quota
// debit.
func tenantProbeHandler(t *testing.T, store TokenStore) http.Handler {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tenant":"` + tenant + `"}`))
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter := newTokenBucket(1000, 1000)
	hardened := withRateLimit(logger, limiter, withDailyQuota(logger, store, inner))
	return tenantAuth(logger, tenantOpToken, store, hardened)
}

func authGet(t *testing.T, h http.Handler, bearer string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://example/probe", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestTenantAuthOperatorTokenStillWorks(t *testing.T) {
	h := tenantProbeHandler(t, nil)
	resp := authGet(t, h, tenantOpToken)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("operator token: status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"tenant":"operator"`) {
		t.Fatalf("body = %q, want tenant operator", body)
	}
}

func TestTenantAuthUnknownToken401(t *testing.T) {
	store := &stubTokenStore{}
	h := tenantProbeHandler(t, store)
	resp := authGet(t, h, "tok_deadbeef_"+strings.Repeat("a", 32))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown token: status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, "revoked") || strings.Contains(bodyStr, "unknown") {
		t.Fatal("401 body must not leak token reason")
	}
}

func TestTenantAuthRevokedToken401(t *testing.T) {
	full := "tok_ab12cd34_" + strings.Repeat("b", 32)
	store := &stubTokenStore{
		authFn: map[string]index.TokenInfo{
			full: {ID: "tok_ab12cd34", Label: "x", DailyQuota: 100, RatePerMin: 60},
		},
		revoked: map[string]bool{"tok_ab12cd34": true},
	}
	h := tenantProbeHandler(t, store)
	resp := authGet(t, h, full)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token: status = %d, want 401", resp.StatusCode)
	}
}

func TestTenantAuthOverRate429(t *testing.T) {
	full := "tok_rate0001_" + strings.Repeat("c", 32)
	store := &stubTokenStore{
		authFn: map[string]index.TokenInfo{
			full: {ID: "tok_rate0001", DailyQuota: 0, RatePerMin: 60},
		},
	}
	h := tenantProbeHandler(t, store)

	var ok, limited int
	for i := 0; i < 20; i++ {
		resp := authGet(t, h, full)
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("429 must carry Retry-After")
			}
			limited++
		}
	}
	if ok == 0 || limited == 0 {
		t.Fatalf("expected mix of 200 and 429, got ok=%d limited=%d", ok, limited)
	}
}

func TestTenantAuthOverQuota429(t *testing.T) {
	full := "tok_quota001_" + strings.Repeat("d", 32)
	store := &stubTokenStore{
		authFn: map[string]index.TokenInfo{
			full: {ID: "tok_quota001", DailyQuota: 2, RatePerMin: 1000},
		},
		countN: make(map[string]int),
		quotas: map[string]int{"tok_quota001": 2},
	}
	h := tenantProbeHandler(t, store)

	for i := 0; i < 3; i++ {
		resp := authGet(t, h, full)
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		_ = resp.Body.Close()
		if i < 2 {
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("call %d: status = %d, want 200", i+1, resp.StatusCode)
			}
		} else {
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("call %d: status = %d, want 429", i+1, resp.StatusCode)
			}
			if !strings.Contains(bodyStr, "quota_exhausted") {
				t.Fatalf("quota 429 body = %q, want quota_exhausted", bodyStr)
			}
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("quota 429 must carry Retry-After")
			}
		}
	}
}

// TestGlobalRateLimit429DoesNotBurnQuota is #0131 finding 1: the middleware
// chain used to debit a tok_ tenant's daily quota INSIDE tenantAuth, which ran
// before (outside) the shared global rate limiter — so a caller rejected by
// the global bucket had already had one of its own daily calls counted against
// it, for a request that never even reached the handler. The fix moves the
// quota debit (withDailyQuota) to run AFTER the global limiter, so a global
// 429 must leave the tenant's usage counter untouched.
func TestGlobalRateLimit429DoesNotBurnQuota(t *testing.T) {
	full := "tok_globl001_" + strings.Repeat("g", 32)
	store := &stubTokenStore{
		authFn: map[string]index.TokenInfo{
			full: {ID: "tok_globl001", DailyQuota: 1000, RatePerMin: 1000},
		},
		countN: make(map[string]int),
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A global bucket with zero tokens and zero refill always rejects.
	exhausted := newTokenBucket(0, 0)
	chain := tenantAuth(logger, tenantOpToken, store,
		withRateLimit(logger, exhausted,
			withDailyQuota(logger, store, inner)))

	resp := authGet(t, chain, full)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 from the exhausted global limiter", resp.StatusCode)
	}
	if got := store.countN["tok_globl001"]; got != 0 {
		t.Fatalf("global 429 must not burn quota: token_usage calls = %d, want 0", got)
	}

	// Confirm the quota is still fully available: swap in an always-allow
	// global bucket and the same request must now succeed and debit exactly once.
	chain = tenantAuth(logger, tenantOpToken, store,
		withRateLimit(logger, newTokenBucket(1000, 1000),
			withDailyQuota(logger, store, inner)))
	resp = authGet(t, chain, full)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 once the global limiter allows", resp.StatusCode)
	}
	if got := store.countN["tok_globl001"]; got != 1 {
		t.Fatalf("token_usage calls = %d, want 1 (debited exactly once, on the admitted request)", got)
	}
}

func TestTenantAuthTokTokenSetsContext(t *testing.T) {
	full := "tok_ctx00001_" + strings.Repeat("e", 32)
	store := &stubTokenStore{
		authFn: map[string]index.TokenInfo{
			full: {ID: "tok_ctx00001", DailyQuota: 0, RatePerMin: 1000},
		},
	}
	h := tenantProbeHandler(t, store)
	resp := authGet(t, h, full)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"tenant":"tok_ctx00001"`) {
		t.Fatalf("body = %q, want tenant tok_ctx00001", body)
	}
}

func TestSecondsUntilUTCMidnight(t *testing.T) {
	now := time.Date(2026, 7, 6, 23, 30, 0, 0, time.UTC)
	got := secondsUntilUTCMidnight(now)
	if got != 30*60 {
		t.Fatalf("secondsUntilUTCMidnight = %d, want %d", got, 30*60)
	}
}

// TestRejectedRequestStillAccessLogged guards the middleware order (review
// finding, PR #511): withRequestLog wraps tenantAuth, so a 401 emits the
// "http request" access-log line — on a public endpoint the rejected traffic
// is exactly what monitoring must count. The tenant attr rides the holder that
// tenantAuth fills on success (empty on a reject).
func TestRejectedRequestStillAccessLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withRequestLog(logger, tenantAuth(logger, tenantOpToken, nil, inner))

	// Rejected: no bearer at all.
	resp := authGet(t, h, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	logs := buf.String()
	if !strings.Contains(logs, `"msg":"http request"`) || !strings.Contains(logs, `"status":401`) {
		t.Fatalf("401 must produce the access-log line, got: %s", logs)
	}

	// Accepted: operator token; the access log carries the tenant.
	buf.Reset()
	resp = authGet(t, h, tenantOpToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	logs = buf.String()
	if !strings.Contains(logs, `"tenant":"operator"`) {
		t.Fatalf("access log must carry the tenant on success, got: %s", logs)
	}
}

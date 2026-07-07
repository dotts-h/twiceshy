// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
)

const defaultTokenRatePerMin = 60

// TokenStore authenticates tenant tokens and counts per-day usage.
type TokenStore interface {
	AuthenticateToken(full string, now time.Time) (index.TokenInfo, error)
	// CountTokenCall atomically debits one call against id's daily quota,
	// returning the resulting (possibly unchanged) total and whether this
	// call was admitted (#0131 finding 2: the check-then-increment must be
	// atomic at the store, not a Go-side compare after an unconditional bump).
	CountTokenCall(id string, now time.Time) (calls int, allowed bool, err error)
}

type tenantKey struct{}

// TenantFromContext returns the authenticated tenant id ("operator" or a tok_ id).
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		return v
	}
	return ""
}

// tenantHolder carries the tenant id upstream to the access logger: withRequestLog
// runs outside tenantAuth (so rejects are logged), but a context value set by the
// inner middleware is invisible to the outer one — the holder bridges that. Same
// request goroutine writes then reads, so a plain field suffices.
type tenantHolder struct{ v string }

func (t *tenantHolder) get() string {
	if t == nil {
		return ""
	}
	return t.v
}

type tenantHolderKey struct{}

func withTenantHolder(ctx context.Context, h *tenantHolder) context.Context {
	return context.WithValue(ctx, tenantHolderKey{}, h)
}

func withTenant(ctx context.Context, tenant string) context.Context {
	if h, ok := ctx.Value(tenantHolderKey{}).(*tenantHolder); ok {
		h.v = tenant
	}
	return context.WithValue(ctx, tenantKey{}, tenant)
}

type tokenRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	now     func() time.Time
}

func newTokenRateLimiter() *tokenRateLimiter {
	return &tokenRateLimiter{
		buckets: make(map[string]*tokenBucket),
		now:     time.Now,
	}
}

func (l *tokenRateLimiter) allow(id string, ratePerMin int) bool {
	if ratePerMin <= 0 {
		ratePerMin = defaultTokenRatePerMin
	}
	perSec := float64(ratePerMin) / 60.0
	burst := float64(ratePerMin) / 6.0
	if burst < 1 {
		burst = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[id]
	if !ok {
		b = newTokenBucket(perSec, burst)
		b.now = l.now
		l.buckets[id] = b
	}
	return b.allow()
}

func tenantAuth(logger *slog.Logger, operatorToken string, store TokenStore, next http.Handler) http.Handler {
	limits := newTokenRateLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		got := r.Header.Get("Authorization")
		if got == "" || len(got) <= len(prefix) || !strings.EqualFold(got[:len(prefix)], prefix) {
			logger.Warn("auth rejected",
				slog.String("reason", bearerRejectReason(got, operatorToken, prefix)),
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="twiceshy"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		value := got[len(prefix):]
		now := limits.now()

		if subtle.ConstantTimeCompare([]byte(value), []byte(operatorToken)) == 1 {
			next.ServeHTTP(w, r.WithContext(withTenant(r.Context(), "operator")))
			return
		}

		if store == nil || !strings.HasPrefix(value, "tok_") {
			logger.Warn("auth rejected",
				slog.String("reason", "bad_token"),
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="twiceshy"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		info, err := store.AuthenticateToken(value, now)
		if err != nil {
			reason := "bad_token"
			if errors.Is(err, index.ErrTokenRevoked) {
				reason = "token_revoked"
			} else if errors.Is(err, index.ErrTokenUnknown) {
				reason = "token_unknown"
			}
			logger.Warn("auth rejected",
				slog.String("reason", reason),
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="twiceshy"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !limits.allow(info.ID, info.RatePerMin) {
			logger.Warn("token rate limit exceeded",
				slog.String("tenant", info.ID),
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// The daily quota debit does NOT happen here (#0131 finding 1): it used to,
		// which ran ahead of the shared global rate limiter in the chain, so a
		// caller rejected by that limiter had already burned one of its own daily
		// calls for a request that never reached a handler. withDailyQuota debits
		// it downstream of the global limiter instead (see server.go's wiring).
		next.ServeHTTP(w, r.WithContext(withTenant(r.Context(), info.ID)))
	})
}

// withDailyQuota enforces each tok_ tenant's daily call quota (#0131). It must
// run AFTER the global rate limiter in the chain, so a global 429 never debits
// a tenant's quota — tenantAuth used to debit it itself, ahead of the global
// limiter. The "operator" tenant (and any request with no tenant in context,
// which should not reach here unauthenticated) has no quota and passes through.
func withDailyQuota(logger *slog.Logger, store TokenStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if store == nil || tenant == "" || tenant == "operator" {
			next.ServeHTTP(w, r)
			return
		}

		now := time.Now()
		calls, allowed, err := store.CountTokenCall(tenant, now)
		if err != nil {
			logger.Error("token usage count failed",
				slog.String("tenant", tenant),
				slog.String("err", err.Error()),
			)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !allowed {
			logger.Warn("token daily quota exhausted",
				slog.String("tenant", tenant),
				slog.Int("calls", calls),
			)
			w.Header().Set("Retry-After", strconv.Itoa(secondsUntilUTCMidnight(now)))
			http.Error(w, "quota_exhausted", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func secondsUntilUTCMidnight(now time.Time) int {
	utc := now.UTC()
	next := time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 0, 0, 0, time.UTC)
	sec := int(next.Sub(utc).Seconds())
	if sec < 1 {
		return 1
	}
	return sec
}

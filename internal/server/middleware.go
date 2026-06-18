// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Hardening limits for the HTTP edge (#0013, SECURITY_ANALYSIS.md Facet 3).
const (
	// maxRequestBytes bounds a raw request body at the transport, before the MCP
	// layer decodes it — larger than maxRecordBodyBytes (64 KiB) plus protocol
	// envelope, small enough that one call can't buffer unbounded memory.
	maxRequestBytes = 256 << 10
	// requestTimeout bounds the whole request so a slow/expensive handler (FTS
	// query, ingest) can't tie up a connection indefinitely.
	requestTimeout = 30 * time.Second
	// Default global rate: steady tokensPerSec with a burst. A single-tenant
	// deploy has one bearer token, so a global bucket is the right granularity;
	// per-tenant limiting is Tier B (#0010).
	defaultRatePerSec = 20
	defaultBurst      = 40
)

// withMaxBytes caps the request body at the transport. Reading past the cap
// yields an *http.MaxBytesError, which fails the downstream decode cleanly
// instead of buffering the whole body first.
func withMaxBytes(n int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, n)
		}
		next.ServeHTTP(w, r)
	})
}

// withTimeout derives a per-request deadline on the request context.
func withTimeout(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenBucket is a minimal stdlib token-bucket rate limiter (no third-party
// dep). now is injectable so tests are deterministic.
type tokenBucket struct {
	mu        sync.Mutex
	tokens    float64
	max       float64
	perSecond float64
	last      time.Time
	now       func() time.Time
}

func newTokenBucket(perSecond, burst float64) *tokenBucket {
	return &tokenBucket{
		tokens:    burst,
		max:       burst,
		perSecond: perSecond,
		now:       time.Now,
	}
}

// allow refills by elapsed time and consumes one token; false when empty.
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.now()
	if b.last.IsZero() {
		b.last = t
	}
	b.tokens += t.Sub(b.last).Seconds() * b.perSecond
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.last = t
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// withRateLimit rejects requests over the bucket's rate with 429 + Retry-After.
func withRateLimit(b *tokenBucket, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

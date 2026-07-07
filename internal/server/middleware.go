// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
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
func withRateLimit(logger *slog.Logger, b *tokenBucket, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.allow() {
			logger.Warn("rate limit exceeded",
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// responseRecorder captures the status code while delegating Flush and Unwrap
// so MCP streamable HTTP (SSE) keeps working.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// withRequestLog emits one access-log line per request after the handler returns.
// It runs OUTSIDE tenantAuth so rejected requests (401/429) are logged too — on a
// public endpoint that is exactly the traffic to count. The tenant id is written
// by the inner tenantAuth into a holder seeded here, because a context value set
// downstream is invisible upstream.
func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := newRequestID()
		rec := &responseRecorder{ResponseWriter: w}
		r = r.WithContext(withReqState(r.Context()))
		next.ServeHTTP(rec, r)

		sessionID := r.Header.Get("Mcp-Session-Id")
		if sessionID == "" {
			sessionID = rec.Header().Get("Mcp-Session-Id")
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		var tenant string
		if s := stateFromContext(r.Context()); s != nil {
			tenant = s.tenant
		}

		logger.Info("http request",
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int64("http_duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("mcp_session_id", sessionID),
			slog.String("tenant", tenant),
		)
	})
}

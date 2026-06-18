// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWithMaxBytes_RejectsOversizedBody(t *testing.T) {
	const limit = 1 << 10 // 1 KiB
	var readErr error
	h := withMaxBytes(limit, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))

	// Over the limit: read fails with *http.MaxBytesError (not a full buffer).
	big := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", limit+1)))
	h.ServeHTTP(httptest.NewRecorder(), big)
	var mbe *http.MaxBytesError
	if !errors.As(readErr, &mbe) {
		t.Fatalf("oversized body: want *http.MaxBytesError, got %v", readErr)
	}

	// Under the limit still reads cleanly.
	ok := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", limit-1)))
	h.ServeHTTP(httptest.NewRecorder(), ok)
	if readErr != nil {
		t.Fatalf("under-limit body must read cleanly, got %v", readErr)
	}
}

func TestWithTimeout_DeadlineReachesHandler(t *testing.T) {
	var deadlineSet bool
	var cancelled error
	h := withTimeout(20*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, deadlineSet = r.Context().Deadline()
		select {
		case <-r.Context().Done():
			cancelled = r.Context().Err()
		case <-time.After(time.Second):
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !deadlineSet {
		t.Error("handler did not see a request deadline")
	}
	if !errors.Is(cancelled, context.DeadlineExceeded) {
		t.Errorf("slow handler should observe DeadlineExceeded, got %v", cancelled)
	}
}

func TestTokenBucket_AllowsBurstThenRefills(t *testing.T) {
	now := time.Unix(0, 0)
	b := newTokenBucket(10, 3) // 10/s, burst 3
	b.now = func() time.Time { return now }

	// Burst of 3 allowed, 4th denied (no time elapsed).
	for i := 0; i < 3; i++ {
		if !b.allow() {
			t.Fatalf("burst token %d should be allowed", i+1)
		}
	}
	if b.allow() {
		t.Fatal("4th immediate request must be denied")
	}
	// After 100ms at 10/s, one token refills.
	now = now.Add(100 * time.Millisecond)
	if !b.allow() {
		t.Fatal("a token should have refilled after 100ms")
	}
	if b.allow() {
		t.Fatal("only one token should have refilled")
	}
}

func TestWithRateLimit_Returns429WhenExhausted(t *testing.T) {
	now := time.Unix(0, 0)
	b := newTokenBucket(1, 1) // 1/s, burst 1
	b.now = func() time.Time { return now }
	h := withRateLimit(slog.Default(), b, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rec1.Code)
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("429 response must carry Retry-After")
	}
	// After a second elapses, allowed again.
	now = now.Add(time.Second)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec3.Code != http.StatusOK {
		t.Fatalf("after refill: got %d, want 200", rec3.Code)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
)

func TestChainContract(t *testing.T) {
	// Setup a temporary SQLite index to satisfy server.New requirements
	ix, err := index.Open(filepath.Join(t.TempDir(), "contract.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })

	t.Run("unauthenticated request bypasses global rate limit", func(t *testing.T) {
		store := &stubTokenStore{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		chain, err := buildChain(authedStages(logger, newTokenBucket(10, 1), "operator-token", store), innerHandler)
		if err != nil {
			t.Fatalf("buildChain: %v", err)
		}

		// (1) Request without auth header -> 401
		req1 := httptest.NewRequest(http.MethodGet, "/push", nil)
		w1 := httptest.NewRecorder()
		chain.ServeHTTP(w1, req1)
		if w1.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w1.Code)
		}

		// (2) Followed by authed request -> must NOT get 429
		req2 := httptest.NewRequest(http.MethodGet, "/push", nil)
		req2.Header.Set("Authorization", "Bearer operator-token")
		w2 := httptest.NewRecorder()
		chain.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w2.Code)
		}

		// (3) A subsequent authed request should get 429 since the burst of 1 is now consumed
		req3 := httptest.NewRequest(http.MethodGet, "/push", nil)
		req3.Header.Set("Authorization", "Bearer operator-token")
		w3 := httptest.NewRecorder()
		chain.ServeHTTP(w3, req3)
		if w3.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w3.Code)
		}
	})

	t.Run("exhausting global bucket does not debit tenant quota", func(t *testing.T) {
		store := &stubTokenStore{
			authFn: map[string]index.TokenInfo{
				"tok_tenant": {
					ID:         "tenant-id",
					RatePerMin: 1000,
				},
			},
			quotas: map[string]int{
				"tenant-id": 1000,
			},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		chain, err := buildChain(authedStages(logger, newTokenBucket(10, 1), "operator-token", store), innerHandler)
		if err != nil {
			t.Fatalf("buildChain: %v", err)
		}

		// First, exhaust the global rate limiter with an operator request
		req1 := httptest.NewRequest(http.MethodGet, "/push", nil)
		req1.Header.Set("Authorization", "Bearer operator-token")
		w1 := httptest.NewRecorder()
		chain.ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w1.Code)
		}

		// Next, request using tok_tenant token. It should get 429.
		req2 := httptest.NewRequest(http.MethodGet, "/push", nil)
		req2.Header.Set("Authorization", "Bearer tok_tenant")
		w2 := httptest.NewRecorder()
		chain.ServeHTTP(w2, req2)
		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w2.Code)
		}

		// CountTokenCall must NOT be debited (stored call count remains 0)
		store.mu.Lock()
		calls := store.countN["tenant-id"]
		store.mu.Unlock()
		if calls != 0 {
			t.Errorf("expected 0 quota debits, got %d", calls)
		}
	})

	t.Run("successful tok_ request debits CountTokenCall exactly once", func(t *testing.T) {
		store := &stubTokenStore{
			authFn: map[string]index.TokenInfo{
				"tok_tenant": {
					ID:         "tenant-id",
					RatePerMin: 1000,
				},
			},
			quotas: map[string]int{
				"tenant-id": 1000,
			},
		}
		srv, err := New(Config{
			Index:      ix,
			Token:      "operator-token",
			TokenStore: store,
			Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/push", nil)
		req.Header.Set("Authorization", "Bearer tok_tenant")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", w.Code)
		}

		store.mu.Lock()
		calls := store.countN["tenant-id"]
		store.mu.Unlock()
		if calls != 1 {
			t.Errorf("expected exactly 1 quota debit, got %d", calls)
		}
	})

	t.Run("signup: 4KiB behavior is unchanged", func(t *testing.T) {
		store := &stubTokenStore{}
		srv, err := New(Config{
			Index:         ix,
			Token:         "operator-token",
			TokenStore:    store,
			SignupEnabled: true,
			TokenIssuer:   ix,
			Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		// Test invalid JSON returns 400 invalid_json
		req1 := httptest.NewRequest(http.MethodPost, "/signup", bytes.NewReader([]byte("{invalid-json")))
		w1 := httptest.NewRecorder()
		srv.ServeHTTP(w1, req1)
		if w1.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w1.Code)
		}
		if !strings.Contains(w1.Body.String(), "invalid_json") {
			t.Errorf("expected error invalid_json, got body: %s", w1.Body.String())
		}

		// Test terms not accepted returns 400 terms_not_accepted
		bodyBytes, _ := json.Marshal(SignupArgs{Email: "test@example.com", AcceptTerms: false})
		req2 := httptest.NewRequest(http.MethodPost, "/signup", bytes.NewReader(bodyBytes))
		w2 := httptest.NewRecorder()
		srv.ServeHTTP(w2, req2)
		if w2.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w2.Code)
		}
		if !strings.Contains(w2.Body.String(), "terms_not_accepted") {
			t.Errorf("expected error terms_not_accepted, got body: %s", w2.Body.String())
		}
	})

	t.Run("misordered declaration fails construction", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		store := &stubTokenStore{}
		limiter := newTokenBucket(10, 1)
		stages := authedStages(logger, limiter, "operator-token", store)

		// Find the index of daily-quota and global-rate-limit
		var quotaIdx, rateLimitIdx int
		for i, stg := range stages {
			switch stg.name {
			case "daily-quota":
				quotaIdx = i
			case "global-rate-limit":
				rateLimitIdx = i
			}
		}

		// Swap them
		stages[quotaIdx], stages[rateLimitIdx] = stages[rateLimitIdx], stages[quotaIdx]

		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		_, err := buildChain(stages, innerHandler)
		if err == nil {
			t.Fatal("expected buildChain to fail with misordered stages, but got nil error")
		}
		if !strings.Contains(err.Error(), "must run after stage") {
			t.Errorf("expected error to mention 'must run after stage', got: %v", err)
		}
	})
}

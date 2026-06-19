// SPDX-License-Identifier: AGPL-3.0-only

package notify_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/notify"
)

// An unset heartbeat URL must be a silent no-op — no panic, no request (#0045).
func TestHeartbeat_EmptyURLIsNoop(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notify.Heartbeat(context.Background(), "", nil)
	if hits != 0 {
		t.Fatalf("empty url must not hit the server, got %d requests", hits)
	}
}

func TestHeartbeat_PingsConfiguredURL(t *testing.T) {
	var (
		mu        sync.Mutex
		hits      int
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		gotMethod = r.Method
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notify.Heartbeat(context.Background(), srv.URL, nil)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("want exactly 1 GET, got %d requests", hits)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("want GET, got %s", gotMethod)
	}
}

// Heartbeat must never break the loop it watches: a dead endpoint is logged, not propagated.
func TestHeartbeat_FailureIsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close()

	notify.Heartbeat(context.Background(), srv.URL, nil)
}

// SPDX-License-Identifier: AGPL-3.0-only

package notify_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A reachable monitor returning a non-2xx status must be swallowed (never
// propagated) AND logged at Warn. This exercises Heartbeat's live non-2xx branch
// (notify.go), distinct from the connection-refused transport-error branch above,
// by wiring a real logger to a buffer. The non-2xx log line emits only "status"
// (no "event" key — notify.go), so we assert on the message and status only.
func TestHeartbeat_Non2xxIsLoggedNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close() // NOT closed — the server is live and answers 500.

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	notify.Heartbeat(context.Background(), srv.URL, logger)

	got := buf.String()
	for _, want := range []string{
		"level=WARN",
		"heartbeat returned non-2xx",
		"status=500",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("non-2xx heartbeat must log %q at Warn, got log: %q", want, got)
		}
	}
}

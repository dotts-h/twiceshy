// SPDX-License-Identifier: AGPL-3.0-only

package notify_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/notify"
)

// An unset alert URL must be a silent no-op (the default deploy posture) — no
// panic, no request (#0038, ADR-0013 §B3).
func TestNew_EmptyURLIsNoop(t *testing.T) {
	a := notify.New("", nil)
	// Must not panic or block; nothing to assert beyond "it returns and runs".
	a.Alert(context.Background(), "anomaly", "should go nowhere")
}

func TestHTTPNotifier_PostsToChannel(t *testing.T) {
	var (
		mu                           sync.Mutex
		gotBody, gotTitle, gotMethod string
		hits                         int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits++
		gotBody, gotTitle, gotMethod = string(b), r.Header.Get("Title"), r.Method
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := notify.New(srv.URL, nil)
	a.Alert(context.Background(), "anomaly", "promote halted: 26 promotions exceed the threshold")

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("want exactly 1 POST, got %d", hits)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("want POST, got %s", gotMethod)
	}
	if !strings.Contains(gotBody, "26 promotions") {
		t.Fatalf("alert body lost the message: %q", gotBody)
	}
	if !strings.Contains(gotTitle, "anomaly") {
		t.Fatalf("alert title must name the event, got %q", gotTitle)
	}
}

// Alerting must never break the loop it watches: a server error / unreachable
// target is logged, not propagated.
func TestHTTPNotifier_FailureIsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	srv.Close() // closed → connection refused

	a := notify.New(srv.URL, nil)
	// Must return without panicking despite the dead endpoint.
	a.Alert(context.Background(), "emergency_stop", "paused")
}

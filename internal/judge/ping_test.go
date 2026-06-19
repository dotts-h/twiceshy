// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

func newJudge(t *testing.T, endpoint string) *judge.ModelJudge {
	t.Helper()
	j, err := judge.NewModelJudge(judge.Config{Endpoint: endpoint, Model: "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("NewModelJudge: %v", err)
	}
	return j
}

// Ping passes when the endpoint answers (any status = up) — the preflight only
// checks reachability, not judging ability (#0040, ADR-0013 §A3).
func TestPing_UpWhenEndpointAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed) // shim only accepts POST — still "up"
	}))
	defer srv.Close()

	if err := newJudge(t, srv.URL).Ping(context.Background()); err != nil {
		t.Fatalf("a reachable endpoint must ping OK: %v", err)
	}
}

// A transport error (closed listener → connection refused) is "down".
func TestPing_DownWhenUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // now refusing connections

	err := newJudge(t, srv.URL).Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("an unreachable endpoint must report down; got %v", err)
	}
}

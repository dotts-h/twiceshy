// SPDX-License-Identifier: AGPL-3.0-only

package retro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewModelAnalyzer_RequiresEndpointAndModel(t *testing.T) {
	if _, err := NewModelAnalyzer(ModelConfig{Model: "gpt-oss:20b"}); err == nil {
		t.Error("want error for empty endpoint")
	}
	if _, err := NewModelAnalyzer(ModelConfig{Endpoint: "http://x"}); err == nil {
		t.Error("want error for empty model")
	}
}

func TestModelAnalyzer_ExtractsAndFramesTranscript(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"candidates":[{"kind":"trap","title":"fts5 MATCH treats input as query syntax","summary":"dotted token errors","error_signatures":["fts5: syntax error"],"ecosystem":"Go","package":"modernc.org/sqlite","root_cause":"raw query is parsed as FTS5 syntax","fix":"quote the query","body":"narrative"}]}`)
	}))
	defer srv.Close()

	a, err := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
	if err != nil {
		t.Fatalf("NewModelAnalyzer: %v", err)
	}
	got, err := a.Analyze(context.Background(), "agent hit fts5: syntax error on modernc.org/sqlite")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if got[0].Kind != "trap" || got[0].Package != "modernc.org/sqlite" {
		t.Errorf("candidate mis-parsed: %+v", got[0])
	}

	// The request must carry the model id and the transcript framed as DATA.
	var req wireRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("request body not wireRequest JSON: %v", err)
	}
	if req.Model != "gpt-oss:20b" {
		t.Errorf("request model = %q, want gpt-oss:20b", req.Model)
	}
	if !strings.Contains(req.Prompt, transcriptBegin) || !strings.Contains(req.Prompt, "agent hit fts5") {
		t.Errorf("request prompt did not frame the transcript:\n%s", req.Prompt)
	}
}

func TestModelAnalyzer_ErrorsAreNotSilentlyEmpty(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http_500": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"garbled":  func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "not json at all") },
		"empty":    func(w http.ResponseWriter, _ *http.Request) {},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			a, _ := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
			if _, err := a.Analyze(context.Background(), "x"); err == nil {
				t.Error("want error, got nil (a bad response must never read as 'no traps')")
			}
		})
	}
}

// A context cancelled mid-request must surface as an error, never as empty
// candidates — the caller leaves the transcript queued for retry (analyzer.go:
// "never treats the error as 'no traps'"). Exercises the transport-error branch
// (model.go: a.client.Do error), the same invariant as an unreachable endpoint.
func TestModelAnalyzer_CancelledContextErrors(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client's context is cancelled (or the test releases us),
		// so the cancellation — not a fast response — decides the outcome.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	a, _ := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel shortly after Analyze starts the request, while the handler blocks.
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	got, err := a.Analyze(ctx, "x")
	if err == nil {
		t.Fatalf("cancelled request returned nil error and %d candidates; a cancelled/unreachable endpoint must never read as 'no traps'", len(got))
	}
	if got != nil {
		t.Errorf("cancelled request returned candidates %+v, want nil", got)
	}
}

// A non-2xx HTTP response from the endpoint is a content failure scoped to THIS
// transcript — the endpoint was reachable — so it must surface as ErrUnprocessable
// (poison-pill detection), not as a generic transient error.
func TestModelAnalyzer_NonOKStatusIsErrUnprocessable(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		code := code
		t.Run(fmt.Sprintf("HTTP_%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_, _ = io.WriteString(w, "model refused")
			}))
			defer srv.Close()
			a, _ := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
			_, err := a.Analyze(context.Background(), "x")
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !errors.Is(err, ErrUnprocessable) {
				t.Errorf("non-2xx HTTP response must be ErrUnprocessable; got %v", err)
			}
		})
	}
}

// A connection failure (no HTTP response received at all) is a transient outage
// unrelated to the transcript content — it must NOT be ErrUnprocessable so the
// caller keeps the transcript queued for retry.
func TestModelAnalyzer_ConnectionFailureIsNotErrUnprocessable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // close immediately so the connection is refused
	a, _ := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
	_, err := a.Analyze(context.Background(), "x")
	if err == nil {
		t.Fatal("want error on connection failure, got nil")
	}
	if errors.Is(err, ErrUnprocessable) {
		t.Errorf("connection failure must NOT be ErrUnprocessable (transient, not content-specific); got %v", err)
	}
}

func TestModelAnalyzer_CapsMaxTraps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var cs []string
		for i := 0; i < 20; i++ {
			cs = append(cs, fmt.Sprintf(`{"kind":"trap","title":"durable trap headline number %02d","body":"b"}`, i))
		}
		_, _ = io.WriteString(w, `{"candidates":[`+strings.Join(cs, ",")+`]}`)
	}))
	defer srv.Close()

	a, _ := NewModelAnalyzer(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", MaxTraps: 3, Client: srv.Client()})
	got, err := a.Analyze(context.Background(), "x")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d candidates, want 3 (capped by MaxTraps)", len(got))
	}
}

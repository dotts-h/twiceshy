// SPDX-License-Identifier: AGPL-3.0-only

package retro

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewModelUsageJudge_RequiresEndpointAndModel(t *testing.T) {
	if _, err := NewModelUsageJudge(ModelConfig{Model: "gpt-oss:20b"}); err == nil {
		t.Error("want error for empty endpoint")
	}
	if _, err := NewModelUsageJudge(ModelConfig{Endpoint: "http://x"}); err == nil {
		t.Error("want error for empty model")
	}
}

func TestModelUsageJudge_ParsesVerdictsAndFramesTranscript(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"verdicts":[{"id":"exp-0149","used":true},{"id":"exp-0150","used":false}]}`)
	}))
	defer srv.Close()

	j, err := NewModelUsageJudge(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
	if err != nil {
		t.Fatalf("NewModelUsageJudge: %v", err)
	}
	got, err := j.JudgeUsage(context.Background(), "agent used exp-0149 for fts5 fix")
	if err != nil {
		t.Fatalf("JudgeUsage: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d verdicts, want 2", len(got))
	}
	if got[0].ID != "exp-0149" || !got[0].Used {
		t.Errorf("verdict[0] mis-parsed: %+v", got[0])
	}
	if got[1].ID != "exp-0150" || got[1].Used {
		t.Errorf("verdict[1] mis-parsed: %+v", got[1])
	}

	// The request must carry the model id and the transcript framed as DATA.
	var req wireRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("request body not wireRequest JSON: %v", err)
	}
	if req.Model != "gpt-oss:20b" {
		t.Errorf("request model = %q, want gpt-oss:20b", req.Model)
	}
	if !strings.Contains(req.Prompt, transcriptBegin) || !strings.Contains(req.Prompt, "agent used exp-0149") {
		t.Errorf("request prompt did not frame the transcript:\n%s", req.Prompt)
	}
}

func TestModelUsageJudge_ErrorsAreNotSilentlyEmpty(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http_500": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"garbled":  func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "not json at all") },
		"empty":    func(w http.ResponseWriter, _ *http.Request) {},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			j, _ := NewModelUsageJudge(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
			if _, err := j.JudgeUsage(context.Background(), "x"); err == nil {
				t.Error("want error, got nil (a bad response must never read as 'all ignored')")
			}
		})
	}
}

// A context cancelled mid-request must surface as an error — the caller leaves
// the transcript for retry, never treats it as "all ignored" (mirrors
// TestModelAnalyzer_CancelledContextErrors).
func TestModelUsageJudge_CancelledContextErrors(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	j, _ := NewModelUsageJudge(ModelConfig{Endpoint: srv.URL, Model: "gpt-oss:20b", Client: srv.Client()})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	got, err := j.JudgeUsage(ctx, "x")
	if err == nil {
		t.Fatalf("cancelled request returned nil error and %d verdicts; must never read as 'all ignored'", len(got))
	}
	if got != nil {
		t.Errorf("cancelled request returned verdicts %+v, want nil", got)
	}
}

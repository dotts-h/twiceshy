// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// chatResponse renders a minimal OpenAI-compatible chat-completions body.
func chatResponse(content string, totalTokens int) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
		"usage": map[string]any{"completion_tokens": totalTokens - 10, "total_tokens": totalTokens},
	})
	return string(b)
}

// The ModelRunner drives a real off-pool model over an OpenAI-compatible endpoint.
// This pins the seam contract the live numbers depend on: the card is made available
// as experience ONLY in the memory-on arm, the task prompt is always present, and the
// completion + token usage map onto Result. Driven through the HTTP seam (httptest),
// not the model — same discipline as internal/retro/model.go's tests.
func TestModelRunner_CardInjectedOnlyInOnArm(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatResponse("const ref = useRef<number|null>(null)", 52))
	}))
	defer srv.Close()

	runner, err := NewModelRunner(RunnerConfig{Endpoint: srv.URL, Model: "test-model"})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}

	const card = "PASS-AN-EXPLICIT-INITIAL-VALUE-TO-USEREF"
	off, err := runner.Run(context.Background(), "create a mutable number ref", "")
	if err != nil {
		t.Fatalf("off-arm Run: %v", err)
	}
	on, err := runner.Run(context.Background(), "create a mutable number ref", card)
	if err != nil {
		t.Fatalf("on-arm Run: %v", err)
	}

	if len(bodies) != 2 {
		t.Fatalf("want 2 upstream requests, got %d", len(bodies))
	}
	if strings.Contains(bodies[0], card) {
		t.Error("off-arm (memory-off) request must NOT carry the card")
	}
	if !strings.Contains(bodies[1], card) {
		t.Error("on-arm (memory-on) request MUST carry the card text")
	}
	for i, b := range bodies {
		if !strings.Contains(b, "create a mutable number ref") {
			t.Errorf("arm %d request is missing the task prompt", i)
		}
	}
	if off.Output != "const ref = useRef<number|null>(null)" {
		t.Errorf("Output = %q, want the completion content", off.Output)
	}
	if on.Output != off.Output {
		t.Errorf("on-arm Output = %q, want the same canned completion %q", on.Output, off.Output)
	}
	if off.Tokens != 52 {
		t.Errorf("Tokens = %d, want total_tokens 52 (the cost metric)", off.Tokens)
	}
	if off.Steps != 1 {
		t.Errorf("Steps = %d, want 1 (a one-shot completion)", off.Steps)
	}
}

// A non-2xx upstream is an error, not a silent empty Result that would corrupt the
// avoidance numbers (a failed attempt must abort the run, per agenteval.Run).
func TestModelRunner_UpstreamErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	runner, err := NewModelRunner(RunnerConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}
	if _, err := runner.Run(context.Background(), "p", ""); err == nil {
		t.Error("a 500 from upstream must surface as an error, not a zero Result")
	}
}

// An empty endpoint is a configuration error caught at construction, so a misconfigured
// eval fails fast rather than silently producing zeros.
func TestNewModelRunner_RequiresEndpoint(t *testing.T) {
	if _, err := NewModelRunner(RunnerConfig{Model: "m"}); err == nil {
		t.Error("NewModelRunner must require an endpoint")
	}
}

func TestModelRunner_RetryOnTransportError_Success(t *testing.T) {
	oldBackoff := transportRetryBackoff
	transportRetryBackoff = 0
	defer func() { transportRetryBackoff = oldBackoff }()

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&reqCount, 1)
		if count == 1 {
			// First request hangs/times out
			time.Sleep(100 * time.Millisecond)
			return
		}
		// Second request succeeds
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("const ref = useRef<number|null>(null)", 52)))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 30 * time.Millisecond}
	runner, err := NewModelRunner(RunnerConfig{
		Endpoint: srv.URL,
		Model:    "m",
		Client:   client,
	})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}

	res, err := runner.Run(context.Background(), "p", "")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if atomic.LoadInt32(&reqCount) != 2 {
		t.Errorf("expected exactly 2 requests, got %d", reqCount)
	}
	if res.Output != "const ref = useRef<number|null>(null)" {
		t.Errorf("unexpected output: %s", res.Output)
	}
}

func TestModelRunner_RetryOnTransportError_Failure(t *testing.T) {
	oldBackoff := transportRetryBackoff
	transportRetryBackoff = 0
	defer func() { transportRetryBackoff = oldBackoff }()

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		// Both requests hang
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 30 * time.Millisecond}
	runner, err := NewModelRunner(RunnerConfig{
		Endpoint: srv.URL,
		Model:    "m",
		Client:   client,
	})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}

	_, err = runner.Run(context.Background(), "p", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if atomic.LoadInt32(&reqCount) != 2 {
		t.Errorf("expected exactly 2 requests, got %d", reqCount)
	}
}

func TestModelRunner_NoRetryOnHTTP500(t *testing.T) {
	oldBackoff := transportRetryBackoff
	transportRetryBackoff = 0
	defer func() { transportRetryBackoff = oldBackoff }()

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	runner, err := NewModelRunner(RunnerConfig{
		Endpoint: srv.URL,
		Model:    "m",
	})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}

	_, err = runner.Run(context.Background(), "p", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if atomic.LoadInt32(&reqCount) != 1 {
		t.Errorf("expected exactly 1 request (no retry on 500), got %d", reqCount)
	}
}

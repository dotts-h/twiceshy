// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// chatCompletionStub serves a canned OpenAI-compatible chat-completions response
// whose assistant message is content, mirroring runner_test.go's chatResponse.
func chatCompletionStub(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func draftRec() *record.Record {
	return &record.Record{
		ID:         "exp-0001",
		Title:      "FTS5 MATCH parses barewords as query syntax",
		Kind:       "trap",
		Status:     "validated",
		Symptom:    &record.Symptom{Summary: "dots/dashes in a bareword break MATCH"},
		Resolution: &record.Resolution{RootCause: "MATCH is not a literal string", Fix: "tokenize and quote each token"},
	}
}

func TestModelTaskDrafter_DraftsFromModelOutput(t *testing.T) {
	srv := chatCompletionStub(t, `{"prompt":"Build a search query from user input","verify":"gobuild","deps":[]}`, http.StatusOK)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	tc, err := d.DraftTask(context.Background(), draftRec())
	if err != nil {
		t.Fatalf("DraftTask: %v", err)
	}
	if tc.TrapID != "exp-0001" {
		t.Errorf("TrapID = %q, want exp-0001", tc.TrapID)
	}
	if tc.Prompt != "Build a search query from user input" {
		t.Errorf("Prompt = %q", tc.Prompt)
	}
	if tc.VerifyID != "gobuild" {
		t.Errorf("VerifyID = %q, want gobuild", tc.VerifyID)
	}
}

func TestModelTaskDrafter_TscRequiresDeps(t *testing.T) {
	srv := chatCompletionStub(t, `{"prompt":"Style a component","verify":"tsc","deps":[]}`, http.StatusOK)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	if _, err := d.DraftTask(context.Background(), draftRec()); !errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("tsc with empty deps must be ErrTaskUnsupported, got %v", err)
	}
}

func TestModelTaskDrafter_UnknownVerifyIsUnsupported(t *testing.T) {
	srv := chatCompletionStub(t, `{"prompt":"do a thing","verify":"pytest","deps":[]}`, http.StatusOK)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	if _, err := d.DraftTask(context.Background(), draftRec()); !errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("an unknown verify class must be ErrTaskUnsupported, got %v", err)
	}
}

func TestModelTaskDrafter_EmptyPromptIsUnsupported(t *testing.T) {
	srv := chatCompletionStub(t, `{"prompt":"","verify":"gobuild","deps":[]}`, http.StatusOK)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	if _, err := d.DraftTask(context.Background(), draftRec()); !errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("an empty prompt must be ErrTaskUnsupported, got %v", err)
	}
}

func TestModelTaskDrafter_UnparseableOutputIsUnsupported(t *testing.T) {
	srv := chatCompletionStub(t, "I cannot help with that.", http.StatusOK)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	if _, err := d.DraftTask(context.Background(), draftRec()); !errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("unparseable model output must be ErrTaskUnsupported, got %v", err)
	}
}

func TestModelTaskDrafter_TransportErrorIsHardFailure(t *testing.T) {
	srv := chatCompletionStub(t, "", http.StatusInternalServerError)
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	// Unlike an unusable draft, an upstream transport failure is a REAL error (the
	// endpoint being down), not a per-record skip — it must not be ErrTaskUnsupported
	// so Prospect aborts instead of silently skipping every record.
	if _, err := d.DraftTask(context.Background(), draftRec()); err == nil || errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("a transport failure must be a hard error, not ErrTaskUnsupported; got %v", err)
	}
}

func TestNewModelTaskDrafter_RequiresEndpoint(t *testing.T) {
	if _, err := NewModelTaskDrafter(DrafterConfig{Model: "m"}); err == nil {
		t.Error("NewModelTaskDrafter must require an endpoint")
	}
}

func TestModelTaskDrafter_Name(t *testing.T) {
	d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: "http://example.invalid", Model: "qwen"})
	if err != nil {
		t.Fatalf("NewModelTaskDrafter: %v", err)
	}
	if d.Name() != "model-task-drafter(qwen)" {
		t.Errorf("Name() = %q", d.Name())
	}
}

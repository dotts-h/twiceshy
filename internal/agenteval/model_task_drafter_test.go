// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	srv := chatCompletionStub(t, `{"prompt":"Build a search query from user input","verify":"gobuild","deps":[],"control":"package main\n\nfunc main() {}\n"}`, http.StatusOK)
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

// TestModelTaskDrafter_ControlField table-drives the new control-snippet
// contract: DraftTask must thread a non-empty "control" field from the model's
// JSON into TaskCase.Control, and must treat an empty or missing control the
// same as an empty prompt -- ErrTaskUnsupported, never a silently-empty
// control reaching the verifier. Two distinct, non-trivial control values are
// asserted on the success path so the test cannot be satisfied by a drafter
// that always returns one constant string.
func TestModelTaskDrafter_ControlField(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantErr     bool
		wantControl string
	}{
		{
			name:        "gobuild control threads through verbatim",
			content:     `{"prompt":"Write a single self-contained package main program that reverses a string","verify":"gobuild","deps":[],"control":"package main\n\nfunc reverse(s string) string {\n\trunes := []rune(s)\n\tfor i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {\n\t\trunes[i], runes[j] = runes[j], runes[i]\n\t}\n\treturn string(runes)\n}\n\nfunc main() {}\n"}`,
			wantControl: "package main\n\nfunc reverse(s string) string {\n\trunes := []rune(s)\n\tfor i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {\n\t\trunes[i], runes[j] = runes[j], runes[i]\n\t}\n\treturn string(runes)\n}\n\nfunc main() {}\n",
		},
		{
			name:        "tsc control threads through with a different value",
			content:     `{"prompt":"Write a single .ts module exporting a reverse(s: string) function","verify":"tsc","deps":["typescript"],"control":"export function reverse(s: string): string {\n  return s.split('').reverse().join('');\n}\n"}`,
			wantControl: "export function reverse(s: string): string {\n  return s.split('').reverse().join('');\n}\n",
		},
		{
			name:    "empty control string is unsupported",
			content: `{"prompt":"Write a function","verify":"gobuild","deps":[],"control":""}`,
			wantErr: true,
		},
		{
			name:    "missing control field is unsupported",
			content: `{"prompt":"Write a function","verify":"gobuild","deps":[]}`,
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			srv := chatCompletionStub(t, tt.content, http.StatusOK)
			d, err := NewModelTaskDrafter(DrafterConfig{Endpoint: srv.URL, Model: "m"})
			if err != nil {
				t.Fatalf("NewModelTaskDrafter: %v", err)
			}
			tc, err := d.DraftTask(context.Background(), draftRec())
			if tt.wantErr {
				if !errors.Is(err, ErrTaskUnsupported) {
					t.Fatalf("an empty/missing control must be ErrTaskUnsupported, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DraftTask: %v", err)
			}
			if tc.Control != tt.wantControl {
				t.Errorf("Control = %q, want %q", tc.Control, tt.wantControl)
			}
		})
	}
}

// TestProspectDrafterSystemV1_RequiresControlAndVerifiableShapes asserts the
// system prompt was tightened to (a) require a "control" answer alongside the
// prompt, (b) constrain gobuild tasks to a single self-contained stdlib-only
// "package main" file and tsc tasks to a single .ts/.tsx module, and (c)
// explicitly forbid workflow/config/YAML-shaped answers, which can't be
// verified by a plain compiler invocation.
func TestProspectDrafterSystemV1_RequiresControlAndVerifiableShapes(t *testing.T) {
	lower := strings.ToLower(prospectDrafterSystemV1)

	required := []string{"control", "package main", "workflow", "config", "yaml"}
	for _, s := range required {
		if !strings.Contains(lower, s) {
			t.Errorf("prospectDrafterSystemV1 must mention %q, got:\n%s", s, prospectDrafterSystemV1)
		}
	}

	if !strings.Contains(prospectDrafterSystemV1, ".ts") && !strings.Contains(prospectDrafterSystemV1, ".tsx") {
		t.Errorf("prospectDrafterSystemV1 must require a .ts/.tsx module for verify=tsc, got:\n%s", prospectDrafterSystemV1)
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

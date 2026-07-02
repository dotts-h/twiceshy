// SPDX-License-Identifier: AGPL-3.0-only

// model_task_drafter.go implements ModelTaskDrafter, the off-pool model that
// drafts prospector tasks (#0113). It mirrors ModelRunner's OpenAI-compatible
// chat-completions HTTP shape (runner.go) rather than reusing it directly: the
// request/response types are shared, but the message shape (system-prompt-only,
// no card) and the strict-JSON parsing (mirroring
// internal/drafter/model.go's modelDrafterSystemV1 pattern) are distinct.
package agenteval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

const (
	drafterHTTPTimeout  = 60 * time.Second
	drafterMaxRespBytes = 1 << 20 // cap a misbehaving endpoint's body
)

// DrafterConfig configures a ModelTaskDrafter.
type DrafterConfig struct {
	// Endpoint is the base URL of the OpenAI-compatible chat-completions API.
	// Required.
	Endpoint string
	// Model is the model identifier forwarded in the request body.
	Model string
	// APIKey is inserted as the Bearer token when non-empty.
	APIKey string
	// Client is an optional HTTP client; nil uses a timeout-bounded default.
	Client *http.Client
}

// ModelTaskDrafter drafts prospector TaskCases by asking an off-pool model, over
// an OpenAI-compatible chat-completions endpoint, for a natural coding task that
// would walk an unwarned coder into the record's trap. It satisfies TaskDrafter.
// The broker verifier PROVES the drafted task's output, so a hallucinated or
// unusable draft is auto-rejected by ErrTaskUnsupported, never trusted — the
// drafter drafts, it never judges (same standing rule as internal/drafter).
type ModelTaskDrafter struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewModelTaskDrafter builds a ModelTaskDrafter from cfg. Returns an error when
// cfg.Endpoint is empty so a misconfigured run fails fast.
func NewModelTaskDrafter(cfg DrafterConfig) (*ModelTaskDrafter, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("agenteval: drafter endpoint required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: drafterHTTPTimeout}
	}
	return &ModelTaskDrafter{
		endpoint: endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   client,
	}, nil
}

// Name implements TaskDrafter; the model id makes a model-drafted task auditably
// distinct in reports.
func (d *ModelTaskDrafter) Name() string { return "model-task-drafter(" + d.model + ")" }

// draftedTaskJSON is the strict shape the model must emit.
type draftedTaskJSON struct {
	Prompt  string   `json:"prompt"`
	Verify  string   `json:"verify"`
	Deps    []string `json:"deps"`
	Control string   `json:"control"`
}

// prospectDrafterVerifyClasses are the only verify values the model may emit;
// anything else is unusable (ErrTaskUnsupported), never silently accepted.
var prospectDrafterVerifyClasses = map[string]bool{"tsc": true, "gobuild": true}

// DraftTask asks the model for a task and validates it. It returns
// ErrTaskUnsupported (a skip, not an abort) when the response is unparseable, the
// prompt is empty, the verify class is unknown, or verify is "tsc" with no deps —
// a tsc job with nothing installed cannot type-check anything. A transport/HTTP
// failure is a real error, surfaced unwrapped (the endpoint being down should
// abort the run, not silently skip every record).
func (d *ModelTaskDrafter) DraftTask(ctx context.Context, rec *record.Record) (TaskCase, error) {
	content, err := d.complete(ctx, prospectDrafterSystemV1, buildProspectDraftPrompt(rec))
	if err != nil {
		return TaskCase{}, fmt.Errorf("agenteval: drafter call for %s: %w", rec.ID, err)
	}
	dt, err := parseDraftedTask(content)
	if err != nil {
		return TaskCase{}, fmt.Errorf("agenteval: drafted task for %s unusable (%v): %w", rec.ID, err, ErrTaskUnsupported)
	}
	return TaskCase{
		TrapID:   rec.ID,
		Prompt:   dt.Prompt,
		VerifyID: dt.Verify,
		Deps:     dt.Deps,
		Control:  dt.Control,
	}, nil
}

// complete POSTs an OpenAI-compatible chat-completions request (system + user
// message, no card — the drafter never sees the fix as "experience") and returns
// the assistant message content. Mirrors ModelRunner.Run's transport (runner.go),
// reusing its request/response types since both live in this package.
func (d *ModelTaskDrafter) complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: d.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	url := d.endpoint + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", d.model, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, drafterMaxRespBytes))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	var cr chatCompletionResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("response contained no choices")
	}
	return cr.Choices[0].Message.Content, nil
}

// parseDraftedTask extracts the JSON object from raw (tolerating prose/markdown
// fences around it, like internal/drafter/model.go's extractJSONObject) and
// validates it: prompt must be non-empty, verify must be a known class,
// verify=="tsc" requires at least one dep, and control must be non-empty — a
// missing/blank control makes the draft unusable, same as an empty prompt.
func parseDraftedTask(raw string) (draftedTaskJSON, error) {
	js := extractDraftedJSONObject(raw)
	if js == "" {
		return draftedTaskJSON{}, errors.New("no JSON object in model output")
	}
	var dt draftedTaskJSON
	if err := json.Unmarshal([]byte(js), &dt); err != nil {
		return draftedTaskJSON{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if strings.TrimSpace(dt.Prompt) == "" {
		return draftedTaskJSON{}, errors.New("empty prompt")
	}
	if !prospectDrafterVerifyClasses[dt.Verify] {
		return draftedTaskJSON{}, fmt.Errorf("unknown verify class %q", dt.Verify)
	}
	if dt.Verify == "tsc" && len(dt.Deps) == 0 {
		return draftedTaskJSON{}, errors.New("verify tsc requires deps")
	}
	if strings.TrimSpace(dt.Control) == "" {
		return draftedTaskJSON{}, errors.New("empty control")
	}
	return dt, nil
}

// extractDraftedJSONObject returns the substring from the first '{' to the last
// '}', or "" if absent — tolerant of a model that wraps JSON in prose or ```json
// fences (same pattern as internal/drafter/model.go's extractJSONObject).
func extractDraftedJSONObject(s string) string {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}

// buildProspectDraftPrompt renders the record's structured fact for the drafter —
// title, symptom summary, and applies_to — but deliberately OMITS resolution
// (root_cause/fix): the model must never see the fix while drafting the task, or
// the drafted prompt would leak it (the leak guard in Prospect is the backstop,
// not the primary control).
func buildProspectDraftPrompt(rec *record.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Record %s\n", rec.ID)
	if rec.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", rec.Title)
	}
	if rec.Symptom != nil && rec.Symptom.Summary != "" {
		fmt.Fprintf(&b, "Symptom: %s\n", rec.Symptom.Summary)
	}
	for _, a := range rec.AppliesTo {
		fmt.Fprintf(&b, "Applies to: ecosystem=%s package=%s\n", a.Ecosystem, a.Package)
	}
	return b.String()
}

// prospectDrafterSystemV1 instructs the model to emit ONLY the moving parts of a
// prospector task, mirroring internal/drafter/model.go's modelDrafterSystemV1
// strict-JSON pattern.
const prospectDrafterSystemV1 = `You draft a coding task that would walk an unwarned coder into a known trap, as STRICT JSON.
Given a record's title, symptom, and ecosystem/package, output ONLY a JSON object:
{
  "prompt": "<a natural, self-contained coding request an unwarned coder would answer by hitting the trap>",
  "verify": "tsc" | "gobuild",
  "deps": ["<npm package with a major pin, e.g. typescript, @types/react@19>"],
  "control": "<a correct, trap-avoiding answer to the same task — the code an experienced coder would write>"
}
Rules:
- The prompt MUST NOT mention the trap, any error text, or the escape/fix — it is a plain task, nothing more.
- Choose verify=gobuild for a Go trap, verify=tsc for a TypeScript/JS type-level trap.
- deps must name every npm package (with a major version pin) the task's code needs; required whenever verify is tsc.
- control is REQUIRED and must be non-empty: a correct, trap-avoiding solution to prompt, verifiable by the chosen
  verify class.
- Both prompt and control must describe/produce a single, self-contained, verifiable source file:
  - verify=gobuild: a single stdlib-only "package main" Go file — no external deps, no multi-file layout.
  - verify=tsc: a single .ts or .tsx module.
- NEVER ask for, or answer with, a workflow, pipeline, config, or YAML file — CI workflows, docker-compose,
  package.json, or any other config-shaped answer cannot be verified by a plain compiler invocation and must not be
  drafted.
- Output JSON only — no prose, no markdown fences.`

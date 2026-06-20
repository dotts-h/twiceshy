// SPDX-License-Identifier: AGPL-3.0-only

package drafter

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
	modelDraftTimeout = 2 * time.Minute
	modelMaxRespBytes = 1 << 20 // cap a misbehaving endpoint's body
)

// ModelDrafter drafts a candidate Go-deprecation repro by asking an off-pool
// local model (e.g. qwen2.5-coder on VM 101) for the moving parts — the
// deprecated-symbol trap, the replacement fix, the staticcheck code, and any
// third-party require — and assembling them with the SAME proven script
// scaffolding the deterministic drafter uses (emitGoDeprecationRepro). The model
// only writes the Go code it is good at; it never writes the fragile offline
// scripts. The broker gate then PROVES the result, so a hallucinated draft is
// auto-rejected, never trusted — that execution gate is what makes a cheap drafter
// safe (ADR-0011 §8). Standing rule: the local model DRAFTS, it never judges.
type ModelDrafter struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewModelDrafter builds a model drafter that POSTs to an Ollama-compatible
// /api/chat endpoint, off the Anthropic pool ([[llm-offload-stack]]).
func NewModelDrafter(endpoint, model string) *ModelDrafter {
	return &ModelDrafter{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		model:    model,
		client:   &http.Client{Timeout: modelDraftTimeout},
	}
}

// Name implements Drafter; the model id is included so a model-drafted repro is
// auditably distinct on its attached label (it carries more poison risk than a
// deterministic-template repro).
func (d *ModelDrafter) Name() string { return "model-drafter(" + d.model + ")" }

// Draft asks the model for a deprecation repro and emits it. Out-of-class records
// (no Go package) and unusable model output return ErrUnsupported so the pipeline
// SKIPS them and the run continues — the gate, not this drafter, is the trust
// anchor. A transport/HTTP failure surfaces as a real error.
func (d *ModelDrafter) Draft(ctx context.Context, root string, rec *record.Record) (string, error) {
	// MVP class: Go deprecation records the deterministic catalog could not cover.
	// Non-Go records (Python, etc.) are out of scope — never spend a model call.
	if goPackage(rec) == "" {
		return "", fmt.Errorf("not a Go record: %w", ErrUnsupported)
	}
	// Advisory/vuln records (GHSA/CVE/GO ids) are NOT deprecations: they validate by
	// version-range via the diverse panel (ADR-0016), not an execution repro (#0026
	// scope boundary — "OSV vulns are facts, not behaviours, a poor repro fit"). They
	// carry a Go package too, so without this guard the model drafter would burn a
	// call per advisory; skip them up front.
	if record.IsAdvisoryClass(rec) {
		return "", fmt.Errorf("advisory-class record, not a deprecation: %w", ErrUnsupported)
	}
	content, err := d.complete(ctx, modelDrafterSystemV1, buildModelDraftPrompt(rec))
	if err != nil {
		// The model drafter is an OPTIONAL fallback (VM 101 is parked by default). If
		// it is unavailable, DECLINE so the corpus walk continues and the
		// deterministic class still drafts later records — do not abort the batch. A
		// genuine gate (broker) crash, by contrast, still aborts upstream: "this
		// drafter is unavailable" must not look like "the trusted gate crashed".
		return "", fmt.Errorf("model drafter %s unavailable (%v): %w", d.model, err, ErrUnsupported)
	}
	tmpl, err := parseModelDraft(content)
	if err != nil {
		// The model produced nothing usable. Decline (skip) rather than hard-fail:
		// the deterministic class already had its turn and the walk should continue.
		return "", fmt.Errorf("model draft for %s unusable (%v): %w", rec.ID, err, ErrUnsupported)
	}
	return emitGoDeprecationRepro(root, rec, tmpl)
}

// complete POSTs an Ollama /api/chat request (forced JSON, temperature 0 for
// reproducibility) and returns the assistant message content.
func (d *ModelDrafter) complete(ctx context.Context, system, user string) (string, error) {
	reqBody, err := json.Marshal(map[string]any{
		"model": d.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"stream":  false,
		"format":  "json",
		"options": map[string]any{"temperature": 0},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model endpoint returned %d", resp.StatusCode)
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, modelMaxRespBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding model response: %w", err)
	}
	return out.Message.Content, nil
}

// modelDraftJSON is the strict shape the model must emit (forced via format=json).
type modelDraftJSON struct {
	Check       string `json:"check"`
	Trap        string `json:"trap"`
	Fix         string `json:"fix"`
	FixRequires []struct {
		Path    string `json:"path"`
		Version string `json:"version"`
	} `json:"fix_requires"`
}

// parseModelDraft extracts the JSON object from the model output (tolerating any
// prose/markdown fence around it), validates the required fields, and returns a
// goDeprecation ready for emitGoDeprecationRepro. A missing field is "unusable",
// not a panic source — the caller maps that to a skip.
func parseModelDraft(raw string) (goDeprecation, error) {
	js := extractJSONObject(raw)
	if js == "" {
		return goDeprecation{}, errors.New("no JSON object in model output")
	}
	var m modelDraftJSON
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return goDeprecation{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if m.Check == "" || m.Trap == "" || m.Fix == "" {
		return goDeprecation{}, errors.New("missing required check/trap/fix")
	}
	td := goDeprecation{check: m.Check, trap: m.Trap, fix: m.Fix}
	for _, r := range m.FixRequires {
		if r.Path == "" || r.Version == "" {
			return goDeprecation{}, fmt.Errorf("incomplete fix_require %+v", r)
		}
		// A real module path has a dot (a domain) in its first element; a bare stdlib
		// name like "io" must never become a require — it would fail `go mod tidy` in
		// prepare and burn a broker run. Reject up front so the draft is skipped.
		if !strings.Contains(r.Path, ".") {
			return goDeprecation{}, fmt.Errorf("fix_require %q is not a module path", r.Path)
		}
		td.fixReqs = append(td.fixReqs, goRequire{path: r.Path, version: r.Version})
	}
	return td, nil
}

// extractJSONObject returns the substring from the first '{' to the last '}', or
// "" if absent — tolerant of a model that wraps JSON in prose or ```json fences.
func extractJSONObject(s string) string {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}

// modelDrafterSystemV1 instructs the local model to emit ONLY the moving parts of
// a staticcheck deprecation repro; the proven scripts are added by the drafter.
const modelDrafterSystemV1 = `You generate a MINIMAL Go staticcheck deprecation repro as STRICT JSON.
Given a record describing a deprecated Go symbol and its replacement, output ONLY a JSON object:
{
  "check": "<the staticcheck code the deprecated usage raises, e.g. SA1019>",
  "trap": "<full source of a tiny 'package main' with func main that USES the deprecated symbol>",
  "fix":  "<full source of a tiny 'package main' with func main that uses the REPLACEMENT instead>",
  "fix_requires": [{"path":"<module path>","version":"<vX.Y.Z>"}]
}
Rules:
- trap and fix must each be a complete, compilable 'package main' file with a main function.
- The trap MUST trigger the staticcheck code; the fix MUST be clean (no deprecation, compiles).
- Prefer the stdlib replacement; include fix_requires ONLY when the fix needs a third-party module (omit or [] otherwise).
- Output JSON only — no prose, no markdown fences.`

// buildModelDraftPrompt renders the record's structured fact for the drafter.
func buildModelDraftPrompt(rec *record.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Record %s\n", rec.ID)
	if rec.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", rec.Title)
	}
	if pkg := goPackage(rec); pkg != "" {
		fmt.Fprintf(&b, "Go package: %s\n", pkg)
	}
	if rec.Symptom != nil {
		if rec.Symptom.Summary != "" {
			fmt.Fprintf(&b, "Symptom: %s\n", rec.Symptom.Summary)
		}
		for _, sig := range rec.Symptom.ErrorSignatures {
			fmt.Fprintf(&b, "Diagnostic: %s\n", sig)
		}
	}
	if rec.Resolution != nil {
		if rec.Resolution.RootCause != "" {
			fmt.Fprintf(&b, "Root cause: %s\n", rec.Resolution.RootCause)
		}
		if rec.Resolution.Fix != "" {
			fmt.Fprintf(&b, "Intended fix: %s\n", rec.Resolution.Fix)
		}
	}
	return b.String()
}

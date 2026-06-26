// SPDX-License-Identifier: AGPL-3.0-only

package retro

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
)

const (
	// analyzerHTTPTimeout bounds one transcript analysis — a single off-pool
	// round-trip over a long transcript, so it is generous (mirrors the judge).
	analyzerHTTPTimeout = 60 * time.Second
	// analyzerMaxRespBytes caps the response body — a misbehaving endpoint cannot
	// flood us, and the cap is far above any honest candidate list.
	analyzerMaxRespBytes = 1 << 20
	// defaultMaxTraps bounds candidates per transcript when the config leaves it 0.
	defaultMaxTraps = 8
)

// ModelConfig configures a ModelAnalyzer.
type ModelConfig struct {
	// Endpoint is the off-pool analysis endpoint ([[llm-offload-stack]]); the same
	// prompt→JSON shim the judge POSTs to (internal/judge) serves both — only the
	// prompt and the expected response shape differ.
	Endpoint string
	// Model is the analyzer model id (e.g. "gpt-oss:20b"). Unlike the judge, the
	// analyzer is a *drafter* (output is quarantined), so the local-family ban does
	// not apply — any model is allowed.
	Model string
	// System optionally overrides the system prompt, sent over the wire so the
	// prompt lives in version control, not the untracked shim.
	System string
	// MaxTraps caps candidates accepted per transcript (0 → defaultMaxTraps).
	MaxTraps int
	// Client is an optional HTTP client; nil uses a timeout-bounded default.
	Client *http.Client
}

// ModelAnalyzer is the thin HTTP edge to an off-pool model that extracts candidate
// records from a transcript. Any transport, status, empty, or parse failure
// returns an error — the caller leaves the transcript queued, never dropping it.
type ModelAnalyzer struct {
	endpoint string
	model    string
	system   string
	maxTraps int
	client   *http.Client
}

// NewModelAnalyzer builds an off-pool analyzer. Endpoint and model are required.
func NewModelAnalyzer(cfg ModelConfig) (*ModelAnalyzer, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("retro: analyzer endpoint required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("retro: analyzer model required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: analyzerHTTPTimeout}
	}
	maxTraps := cfg.MaxTraps
	if maxTraps <= 0 {
		maxTraps = defaultMaxTraps
	}
	return &ModelAnalyzer{endpoint: endpoint, model: cfg.Model, system: cfg.System, maxTraps: maxTraps, client: client}, nil
}

// wireRequest mirrors the judge shim contract: {model, prompt, system?}.
type wireRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
}

// wireCandidates is the strict JSON the endpoint must return. A response that
// cannot decode into this shape is an error — the transcript stays queued.
type wireCandidates struct {
	Candidates []wireCandidate `json:"candidates"`
}

type wireCandidate struct {
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	ErrorSignatures []string `json:"error_signatures"`
	Ecosystem       string   `json:"ecosystem"`
	Package         string   `json:"package"`
	RootCause       string   `json:"root_cause"`
	Fix             string   `json:"fix"`
	Body            string   `json:"body"`
}

// Analyze frames the transcript as DATA, POSTs the extraction prompt, and parses
// the strict-JSON candidate list, capped at MaxTraps.
func (a *ModelAnalyzer) Analyze(ctx context.Context, transcript string) ([]Candidate, error) {
	prompt := buildPrompt(frameTranscript(transcript), a.maxTraps)
	body, err := json.Marshal(wireRequest{Model: a.model, Prompt: prompt, System: a.system})
	if err != nil {
		return nil, fmt.Errorf("retro: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("retro: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("retro: call %s: %w", a.model, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("retro: status %d: %s: %w", resp.StatusCode, strings.TrimSpace(string(b)), ErrUnprocessable)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, analyzerMaxRespBytes))
	if err != nil {
		return nil, fmt.Errorf("retro: read response: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("retro: empty response: %w", ErrUnprocessable)
	}
	var wc wireCandidates
	if err := json.Unmarshal(raw, &wc); err != nil {
		return nil, fmt.Errorf("retro: decode candidates: %w", errors.Join(err, ErrUnprocessable))
	}

	out := make([]Candidate, 0, len(wc.Candidates))
	for _, c := range wc.Candidates {
		// wireCandidate and Candidate are identical in shape (tags aside): the wire
		// type carries the JSON contract, the domain type travels to the driver.
		out = append(out, Candidate(c))
		if len(out) >= a.maxTraps {
			break
		}
	}
	return out, nil
}

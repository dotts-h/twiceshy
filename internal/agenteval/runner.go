// SPDX-License-Identifier: AGPL-3.0-only

// runner.go implements ModelRunner, the thin HTTP edge to an OpenAI-compatible
// chat-completions endpoint. It satisfies AgentRunner: card=="" is the
// memory-off arm (no card injected); card!="" is the memory-on arm (the card
// is sent as a system message before the task prompt).
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
)

const (
	runnerHTTPTimeout   = 120 * time.Second
	runnerMaxRespBytes  = 1 << 20
	defaultSystemPrompt = "Output ONLY the code that solves the task. No prose, no explanation, no markdown commentary outside a single fenced code block."
)

// RunnerConfig configures a ModelRunner.
type RunnerConfig struct {
	// Endpoint is the base URL of the OpenAI-compatible chat-completions API.
	// Required.
	Endpoint string
	// Model is the model identifier forwarded in the request body.
	Model string
	// APIKey is inserted as the Bearer token when non-empty.
	APIKey string
	// SystemPrompt overrides the default system prompt. Zero value uses the
	// built-in prompt instructing the model to output code only.
	SystemPrompt string
	// Client is an optional HTTP client; nil uses a timeout-bounded default.
	Client *http.Client
}

// ModelRunner drives an off-pool model over an OpenAI-compatible
// chat-completions endpoint. It satisfies AgentRunner.
type ModelRunner struct {
	endpoint     string
	model        string
	apiKey       string
	systemPrompt string
	client       *http.Client
}

// NewModelRunner constructs a ModelRunner from cfg. Returns an error when
// cfg.Endpoint is empty so a misconfigured eval fails fast.
func NewModelRunner(cfg RunnerConfig) (*ModelRunner, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("agenteval: runner endpoint required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: runnerHTTPTimeout}
	}
	sp := cfg.SystemPrompt
	if sp == "" {
		sp = defaultSystemPrompt
	}
	return &ModelRunner{
		endpoint:     endpoint,
		model:        cfg.Model,
		apiKey:       cfg.APIKey,
		systemPrompt: sp,
		client:       client,
	}, nil
}

// chatMessage is one element of the messages array in a chat-completions request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the OpenAI-compatible request body.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// chatCompletionResponse is the OpenAI-compatible response body (subset we need).
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Run calls the chat-completions endpoint with prompt as the user message.
// When card is non-empty the card is injected as a system message immediately
// before the user prompt (memory-on arm). When card is empty it is omitted
// entirely (memory-off arm). Returns an error on any non-2xx response.
func (r *ModelRunner) Run(ctx context.Context, prompt, card string) (Result, error) {
	msgs := []chatMessage{
		{Role: "system", Content: r.systemPrompt},
	}
	if card != "" {
		msgs = append(msgs, chatMessage{
			Role:    "system",
			Content: "Relevant past experience:\n" + card,
		})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: prompt})

	body, err := json.Marshal(chatRequest{Model: r.model, Messages: msgs})
	if err != nil {
		return Result{}, fmt.Errorf("agenteval: marshal request: %w", err)
	}

	url := r.endpoint + "/v1/chat/completions"
	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("agenteval: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if r.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+r.apiKey)
		}
		return req, nil
	}

	resp, err := postWithOneRetry(ctx, r.client, buildReq)
	if err != nil {
		return Result{}, fmt.Errorf("agenteval: call %s: %w", r.model, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Result{}, fmt.Errorf("agenteval: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, runnerMaxRespBytes))
	if err != nil {
		return Result{}, fmt.Errorf("agenteval: read response: %w", err)
	}

	var cr chatCompletionResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return Result{}, fmt.Errorf("agenteval: decode response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return Result{}, errors.New("agenteval: response contained no choices")
	}

	return Result{
		Output: cr.Choices[0].Message.Content,
		Steps:  1,
		Tokens: cr.Usage.TotalTokens,
	}, nil
}

// transportRetryBackoff is the duration to wait before retrying a transport-level error.
// It is package-level and overridden to ~0 in tests.
var transportRetryBackoff = 2 * time.Second

// postWithOneRetry executes the given HTTP request. If a transport-level error occurs
// (i.e. client.Do returns an error), it retries the request exactly once after transportRetryBackoff.
// Non-transport errors (any HTTP response, even 5xx) are never retried.
// To ensure the body is re-sendable, we take a request builder function.
func postWithOneRetry(ctx context.Context, client *http.Client, buildReq func() (*http.Request, error)) (*http.Response, error) {
	req, err := buildReq()
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		// Any client.Do error is transport-level by definition (timeout, refused/
		// reset connection, DNS) — an HTTP response, however bad, never errors here.
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(transportRetryBackoff):
		}

		req, err = buildReq()
		if err != nil {
			return nil, err
		}
		return client.Do(req)
	}

	return resp, nil
}

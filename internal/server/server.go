// SPDX-License-Identifier: AGPL-3.0-only

// Package server exposes the MCP tools over streamable HTTP behind bearer
// auth: the pull channel (ADR-0001 §5) search_experience and get_experience,
// and the write path (Phase 3) record_experience — which dedups an
// agent-proposed draft and returns a quarantined record to PR, never a direct
// write. The push channel (Phase 2) still lives elsewhere.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dotts-h/twiceshy/internal/index"
)

// Version is stamped into the MCP server implementation info.
const Version = "0.1.0"

// Config wires the server.
type Config struct {
	// Index is the derived SQLite index to serve from.
	Index *index.Index
	// Token is the bearer token required on every request. Required:
	// there is no unauthenticated mode (CONVENTIONS.md, Security).
	Token string
	// Repo, when set, lets fingerprint matching also use app-scoped
	// fingerprints for that repository identifier.
	Repo string
}

// Tool descriptions are load-bearing: description text alone produces
// large swings in tool-use quality (research §3). They must tell the model
// when to call, what to pass, and that empty results are an answer.
const (
	searchDescription = "Search a curated store of hard-won, validated engineering experience: " +
		"traps, fixes, dead-ends and conventions recorded when someone last hit this exact problem. " +
		"Call this BEFORE debugging an unfamiliar error and BEFORE retrying an approach that just failed: " +
		"pass the verbatim error text or a short symptom description in `query`. " +
		"Optionally filter by `ecosystem` (e.g. \"Go\", \"PyPI\", \"MCP\") and `package`. " +
		"Returns at most 3 high-confidence matches with ids for get_experience. " +
		"An empty result means nothing recorded applies — that is an answer; do not force a near-miss to fit."

	getDescription = "Fetch the full markdown of one experience record by id (e.g. \"exp-0001\") " +
		"as returned by search_experience: symptom, root cause, the validated fix, " +
		"dead ends already tried and why they failed (do not retry those), and the guarding test. " +
		"Read it before acting on the lesson."
)

// New returns the HTTP handler serving the MCP pull channel.
func New(cfg Config) (http.Handler, error) {
	if cfg.Index == nil {
		return nil, errors.New("server: an index is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("server: a bearer token is required; there is no unauthenticated mode")
	}

	h := &handlers{ix: cfg.Index, repo: cfg.Repo}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "twiceshy",
		Title:   "twiceshy experience service",
		Version: Version,
	}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "search_experience", Description: searchDescription}, h.search)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_experience", Description: getDescription}, h.get)
	mcp.AddTool(srv, &mcp.Tool{Name: "record_experience", Description: recordDescription}, h.record)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil)

	// Middleware chain (outermost first): reject unauthenticated requests before
	// any work, then rate-limit, then bound the request's time and body size.
	limiter := newTokenBucket(defaultRatePerSec, defaultBurst)
	hardened := withRateLimit(limiter,
		withTimeout(requestTimeout,
			withMaxBytes(maxRequestBytes, mcpHandler)))
	return bearerAuth(cfg.Token, hardened), nil
}

type handlers struct {
	ix   *index.Index
	repo string
}

// SearchArgs is the search_experience input.
type SearchArgs struct {
	Query              string `json:"query" jsonschema:"verbatim error text or a short symptom description"`
	Ecosystem          string `json:"ecosystem,omitempty" jsonschema:"optional stack filter, e.g. Go, PyPI, npm, MCP"`
	Package            string `json:"package,omitempty" jsonschema:"optional package/module filter within the ecosystem"`
	K                  int    `json:"k,omitempty" jsonschema:"max results, 1-3 (hard cap 3)"`
	IncludeQuarantined bool   `json:"include_quarantined,omitempty" jsonschema:"also surface unreviewed quarantined records, labeled by status"`
}

// SearchHit is one search_experience result row.
type SearchHit struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind"`
	Status  string  `json:"status"`
	Title   string  `json:"title"`
	Summary string  `json:"summary,omitempty"`
	Score   float64 `json:"score"`
	Matched string  `json:"matched"`
}

// SearchResult is the search_experience output.
type SearchResult struct {
	Hits []SearchHit `json:"hits"`
}

// maxQueryBytes caps the query at the MCP edge. index.maxQueryTokens bounds the
// token *count*, but a single whitespace-free multi-MB token slips past it into
// a SHA-256 plus a multi-MB FTS5 MATCH term; an authenticated client shouldn't
// be able to turn one call into that much work.
const maxQueryBytes = 16 << 10

func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, SearchResult{}, errors.New("query must be non-empty")
	}
	if len(args.Query) > maxQueryBytes {
		return nil, SearchResult{}, fmt.Errorf("query too large: %d bytes (max %d)", len(args.Query), maxQueryBytes)
	}
	hits, err := h.ix.Retrieve(ctx, index.Query{
		Text:               args.Query,
		Repo:               h.repo,
		Ecosystem:          args.Ecosystem,
		Package:            args.Package,
		K:                  args.K,
		IncludeQuarantined: args.IncludeQuarantined,
	})
	if err != nil {
		return nil, SearchResult{}, fmt.Errorf("search failed: %w", err)
	}
	out := SearchResult{Hits: make([]SearchHit, 0, len(hits))}
	for _, hit := range hits {
		out.Hits = append(out.Hits, SearchHit{
			ID:      hit.ID,
			Kind:    hit.Kind,
			Status:  hit.Status,
			Title:   hit.Title,
			Summary: hit.Summary,
			Score:   hit.Score,
			Matched: hit.Matched,
		})
	}
	return nil, out, nil
}

// GetArgs is the get_experience input.
type GetArgs struct {
	ID string `json:"id" jsonschema:"record id as returned by search_experience, e.g. exp-0001"`
}

// GetResult is the get_experience output: the full record file.
type GetResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, GetResult, error) {
	stored, err := h.ix.Get(ctx, args.ID)
	if err != nil {
		return nil, GetResult{}, err
	}
	return nil, GetResult{
		ID:       stored.ID,
		Kind:     stored.Kind,
		Status:   stored.Status,
		Title:    stored.Title,
		Path:     stored.Path,
		Markdown: stored.Markdown,
	}, nil
}

// bearerAuth enforces a constant-time bearer-token check on every request.
// The token is never logged.
func bearerAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		got := r.Header.Get("Authorization")
		if len(got) <= len(prefix) ||
			!strings.EqualFold(got[:len(prefix)], prefix) ||
			subtle.ConstantTimeCompare([]byte(got[len(prefix):]), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="twiceshy"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

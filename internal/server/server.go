// SPDX-License-Identifier: AGPL-3.0-only

// Package server exposes the MCP pull channel over streamable HTTP and the
// push channel (ADR-0001 §5) as POST /push — both behind the same bearer
// auth and middleware chain. Pull: search_experience, get_experience, and
// the write path (Phase 3) record_experience. Push: embedding-free trap
// cards for hook additionalContext injection.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

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
	// Embedder, when set, enables dense (cosine) retrieval on the pull channel,
	// fused with fingerprint + BM25 via RRF (ADR-0009). nil = embedding-free
	// retrieval only. The hot/push path never uses it.
	Embedder index.Embedder
	// Logger emits structured server logs. nil defaults to JSON on stderr.
	Logger *slog.Logger
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

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}

	h := &handlers{ix: cfg.Index, repo: cfg.Repo, emb: cfg.Embedder, logger: logger}
	h.usage = newUsageRecorder(cfg.Index, logger, time.Now)
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "twiceshy",
		Title:   "twiceshy experience service",
		Version: Version,
	}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "search_experience", Description: searchDescription}, h.search)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_experience", Description: getDescription}, h.get)
	mcp.AddTool(srv, &mcp.Tool{Name: "record_experience", Description: recordDescription}, h.record)
	mcp.AddTool(srv, &mcp.Tool{Name: "report_outcome", Description: reportDescription}, h.reportOutcome)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()
	// Route all methods to pushHTTP so a non-POST gets a clean 405 from the
	// handler itself, rather than falling through to the MCP catch-all.
	mux.HandleFunc("/push", h.pushHTTP)
	mux.Handle("/", mcpHandler)

	// Middleware chain (outermost first): access log, then reject unauthenticated
	// requests before any work, then rate-limit, then bound time and body size.
	limiter := newTokenBucket(defaultRatePerSec, defaultBurst)
	hardened := withRateLimit(logger, limiter,
		withTimeout(requestTimeout,
			withMaxBytes(maxRequestBytes, mux)))
	return withRequestLog(logger, bearerAuth(logger, cfg.Token, hardened)), nil
}

type handlers struct {
	ix     *index.Index
	repo   string
	emb    index.Embedder // optional; enables pull-channel dense retrieval
	logger *slog.Logger
	usage  *usageRecorder // records retrieval usage off the latency budget (ADR-0013 §4)
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
	start := time.Now()
	const tool = "search_experience"

	if strings.TrimSpace(args.Query) == "" {
		err := errors.New("query must be non-empty")
		h.logToolError(tool, start, err)
		return nil, SearchResult{}, err
	}
	if len(args.Query) > maxQueryBytes {
		err := fmt.Errorf("query too large: %d bytes (max %d)", len(args.Query), maxQueryBytes)
		h.logToolError(tool, start, err)
		return nil, SearchResult{}, err
	}
	// Pull channel: dense (cosine) retrieval fused with fingerprint + BM25 when
	// an embedder is configured; falls back to the embedding-free path otherwise
	// (ADR-0009). RetrieveFused applies the relevance floor like Retrieve.
	hits, err := h.ix.RetrieveFused(ctx, index.Query{
		Text:               args.Query,
		Repo:               h.repo,
		Ecosystem:          args.Ecosystem,
		Package:            args.Package,
		K:                  args.K,
		IncludeQuarantined: args.IncludeQuarantined,
	}, h.emb)
	if err != nil {
		err = fmt.Errorf("search failed: %w", err)
		h.logToolError(tool, start, err)
		return nil, SearchResult{}, err
	}
	out := SearchResult{Hits: make([]SearchHit, 0, len(hits))}
	for _, hit := range hits {
		out.Hits = append(out.Hits, SearchHit{
			ID:      hit.ID,
			Kind:    hit.Kind,
			Status:  hit.Status,
			Title:   capText(sanitizeForTransport(hit.Title), maxSearchTitleBytes),
			Summary: capText(sanitizeForTransport(hit.Summary), maxSearchSummaryBytes),
			Score:   hit.Score,
			Matched: hit.Matched,
		})
	}
	enveloped := renderSearchResults(out.Hits)
	// Reinforcement signal (ADR-0013 §4): a served record's usage advances. Done
	// async so it never adds to the retrieval latency budget.
	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}
	h.usage.record(ids)
	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.Int("hits", len(out.Hits)),
	)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: enveloped}},
	}, out, nil
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
	start := time.Now()
	const tool = "get_experience"

	stored, err := h.ix.Get(ctx, args.ID)
	if err != nil {
		h.logToolError(tool, start, err,
			slog.String("id", args.ID),
			slog.Bool("found", false),
		)
		return nil, GetResult{}, err
	}
	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("id", args.ID),
		slog.Bool("found", true),
	)
	h.usage.record([]string{stored.ID})
	enveloped := renderGetExperience(stored.Status, stored.ID, stored.Markdown)
	result := GetResult{
		ID:       stored.ID,
		Kind:     stored.Kind,
		Status:   stored.Status,
		Title:    stored.Title,
		Path:     stored.Path,
		Markdown: enveloped,
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: enveloped}},
	}, result, nil
}

func (h *handlers) logToolError(tool string, start time.Time, err error, extra ...slog.Attr) {
	class := errorClass(err)
	attrs := []any{
		slog.String("tool", tool),
		slog.String("outcome", "error"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("error_class", class),
	}
	for _, a := range extra {
		attrs = append(attrs, a)
	}
	if clientError(class) {
		h.logger.Warn("tool call", attrs...)
	} else {
		h.logger.Error("tool call", attrs...)
	}
}

// bearerAuth enforces a constant-time bearer-token check on every request.
// The token is never logged.
func bearerAuth(logger *slog.Logger, token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		got := r.Header.Get("Authorization")
		reason := bearerRejectReason(got, token, prefix)
		if reason != "" {
			logger.Warn("auth rejected",
				slog.String("reason", reason),
				slog.String("remote_addr", r.RemoteAddr),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="twiceshy"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerRejectReason(got, token, prefix string) string {
	if got == "" {
		return "missing_bearer"
	}
	if len(got) <= len(prefix) || !strings.EqualFold(got[:len(prefix)], prefix) {
		return "wrong_scheme"
	}
	if subtle.ConstantTimeCompare([]byte(got[len(prefix):]), []byte(token)) != 1 {
		return "bad_token"
	}
	return ""
}

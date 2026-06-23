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
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// Version is stamped into the MCP server implementation info.
const Version = "0.1.0"

// Config wires the server.
type Config struct {
	// Index is the derived SQLite index to serve from.
	Index *index.Index
	// RecordCount is how many records were indexed at startup. /readyz reports
	// NOT-ready when it is 0 (an empty corpus = "serving nothing", the failure the
	// crash-loop outage produced) so an external monitor can page on it.
	RecordCount int
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
	// ReportQueue, when set, is the directory report_outcome enqueues outcome
	// reports into for the `intake-reports` CLI to materialize into experience/
	// (ADR-0013 §E1, #0042). Empty keeps the legacy behavior: report_outcome
	// returns the counter-record markdown for a human to PR, and writes nothing.
	ReportQueue string
	// RetroQueue, when set, is the directory POST /retro spools session transcripts
	// into for the `retro-intake` CLI to analyze off-pool into quarantined drafts
	// (ADR-0018, #0065). Empty disables the /retro endpoint (503): retro capture is
	// opt-in, like the report queue.
	RetroQueue string
	// IssueQueue, when set, is the directory report_issue enqueues agent-submitted
	// issues into for the `intake-issues` CLI to materialize into docs/issues/
	// (#0066). Empty keeps the fallback: report_issue returns a PR-ready docs/issues
	// markdown for a human to PR, and writes nothing.
	IssueQueue string
	// Corpus is the corpus root (the directory containing experience/) the index
	// was built from. The write path scans it to allocate record ids robustly
	// against a live index that has drifted behind the committed corpus (#0059).
	// Empty falls back to index-only allocation.
	Corpus string
	// Telemetry, when set, records per-query gate decisions for /push and
	// search_experience (#0067) — write-only, off the hot path, never read by
	// retrieval. nil disables it.
	Telemetry *telemetry.Recorder
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

// Server is the running pull+push handler. It is an http.Handler; SetRecordCount
// lets a hot-reload (#0060) update the readiness count the probes report after
// rebuilding the index in place, without reconstructing the server.
type Server struct {
	http.Handler
	h *handlers
}

// SetRecordCount updates the record count /healthz and /readyz report, after a
// hot-reload rebuilds the index. Concurrency-safe with in-flight probes.
func (s *Server) SetRecordCount(n int) { s.h.recordCount.Store(int64(n)) }

// New returns the Server handling the MCP pull channel and the push channel.
func New(cfg Config) (*Server, error) {
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

	h := &handlers{ix: cfg.Index, repo: cfg.Repo, emb: cfg.Embedder, logger: logger, reportQueue: cfg.ReportQueue, retroQueue: cfg.RetroQueue, issueQueue: cfg.IssueQueue, corpus: cfg.Corpus, telemetry: cfg.Telemetry}
	h.recordCount.Store(int64(cfg.RecordCount))
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
	mcp.AddTool(srv, &mcp.Tool{Name: "report_issue", Description: issueDescription}, h.reportIssue)
	mcp.AddTool(srv, &mcp.Tool{Name: "confirm_helpful", Description: confirmDescription}, h.confirmHelpful)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()
	// Route all methods to pushHTTP so a non-POST gets a clean 405 from the
	// handler itself, rather than falling through to the MCP catch-all.
	mux.HandleFunc("/push", h.pushHTTP)
	// Same rationale (exp-0006): /retro is registered unqualified, not "POST
	// /retro", so a non-POST returns 405 from retroHTTP instead of falling through.
	mux.HandleFunc("/retro", h.retroHTTP)
	mux.Handle("/", mcpHandler)

	// Middleware chain (outermost first): access log, then reject unauthenticated
	// requests before any work, then rate-limit, then bound time and body size.
	limiter := newTokenBucket(defaultRatePerSec, defaultBurst)
	hardened := withRateLimit(logger, limiter,
		withTimeout(requestTimeout,
			withMaxBytes(maxRequestBytes, mux)))
	authed := withRequestLog(logger, bearerAuth(logger, cfg.Token, hardened))

	// Health probes bypass auth + rate-limit so a container HEALTHCHECK and an
	// external uptime monitor can reach them unauthenticated: /healthz = liveness
	// (the process is up and serving), /readyz = readiness (the index is non-empty;
	// NOT-ready on an empty corpus = "serving nothing"). Their absence is what made
	// the 5h crash-loop outage invisible.
	outer := http.NewServeMux()
	outer.HandleFunc("/healthz", h.healthz)
	outer.HandleFunc("/readyz", h.readyz)
	outer.Handle("/", authed)
	return &Server{Handler: outer, h: h}, nil
}

// healthz is liveness: 200 as long as the process serves HTTP. Unauthenticated,
// no index work — a probe that pages only when the process is truly down.
func (h *handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ok","records":%d}`+"\n", h.recordCount.Load())
}

// readyz is readiness: 200 only when the index has records, else 503. An empty
// corpus means the server is up but serving nothing — the exact silent-failure
// the outage produced — so it must read as NOT-ready to an external monitor.
func (h *handlers) readyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := h.recordCount.Load()
	if n <= 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"status":"empty","records":0}`+"\n")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ready","records":%d}`+"\n", n)
}

type handlers struct {
	ix          *index.Index
	recordCount atomic.Int64 // current indexed record count; drives /healthz + /readyz, updated on hot-reload
	repo        string
	emb         index.Embedder // optional; enables pull-channel dense retrieval
	logger      *slog.Logger
	usage       *usageRecorder      // records retrieval usage off the latency budget (ADR-0013 §4)
	reportQueue string              // optional; report_outcome enqueues here for intake-reports (ADR-0013 §E1)
	retroQueue  string              // optional; POST /retro spools transcripts here for retro-intake (ADR-0018)
	issueQueue  string              // optional; report_issue enqueues here for intake-issues (#0066)
	corpus      string              // corpus root for robust id allocation against the source of truth (#0059)
	telemetry   *telemetry.Recorder // optional; per-query gate-decision log (#0067)

	idMu   sync.Mutex // serializes record-id allocation (#0089)
	lastID int        // high-water mark of ids handed out this process; 0 = none yet
}

// allocateNextID hands out a fresh exp-NNNN id. ingest.NextID is corpus/index-derived
// and stateless, so two record_experience calls in one session would both get the same
// id (TECH_DEBT M3 / #0089 — the field-report collision). The in-process high-water mark
// closes that for a single server; the draft is still propose-only and PR-reviewed.
func (h *handlers) allocateNextID(ctx context.Context) (string, error) {
	next, err := ingest.NextID(ctx, h.ix, h.corpus)
	if err != nil {
		return "", err
	}
	n, ok := record.Num(next)
	if !ok {
		return "", fmt.Errorf("allocateNextID: NextID returned malformed id %q", next)
	}
	h.idMu.Lock()
	defer h.idMu.Unlock()
	if n <= h.lastID {
		n = h.lastID + 1
	}
	h.lastID = n
	return record.FormatID(n), nil
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

func (h *handlers) search(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
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
	// The MCP session id correlates this retrieval with the session's captured
	// transcript so the retro helpfulness join can attribute served cards (#0069); it
	// is hashed (never stored raw) in recordSearchDecision.
	h.recordSearchDecision(args.Query, hits, sessionIDFromRequest(req))
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

// sessionIDFromRequest extracts the MCP session id from a tool request, or "" if there
// is none. GetSession returns the *ServerSession boxed in a Session interface, so a nil
// pointer is a non-nil interface (the typed-nil gotcha) — assert to the concrete type
// and nil-check THAT (the SDK's own idiom) before reading the id, or ss.ID() panics on
// the nil receiver.
func sessionIDFromRequest(req *mcp.CallToolRequest) string {
	if req == nil {
		return ""
	}
	if ss, ok := req.GetSession().(*mcp.ServerSession); ok && ss != nil {
		return ss.ID()
	}
	return ""
}

// recordSearchDecision logs this query's search decision (#0067): the served
// records and their scores, for measuring retrieval precision on real traffic, plus
// the SALTED hash of the MCP session id (#0069) so served cards can be attributed to
// the session's captured transcript. Best-effort and async; the raw query and the raw
// session id are hashed, never stored. An empty sessionID records no correlation key.
// nil recorder = no-op.
func (h *handlers) recordSearchDecision(query string, hits []index.Hit, sessionID string) {
	if h.telemetry == nil {
		return
	}
	served := make([]telemetry.ServedHit, len(hits))
	for i, hit := range hits {
		served[i] = telemetry.ServedHit{ID: hit.ID, Score: hit.Score}
	}
	session := ""
	if sessionID != "" {
		session = h.telemetry.Hash(sessionID)
	}
	h.telemetry.Record(telemetry.Decision{
		Channel:   "search",
		QueryHash: h.telemetry.Hash(query),
		Session:   session,
		Served:    served,
		Count:     len(hits),
	})
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

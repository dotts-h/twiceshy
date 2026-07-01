// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// PushArgs is the POST /push request body.
type PushArgs struct {
	Query     string `json:"query"`
	Ecosystem string `json:"ecosystem,omitempty"`
	Package   string `json:"package,omitempty"`
	// Session is the caller's session id (the UserPromptSubmit hook forwards the
	// Claude Code session id). Stamped as a salted hash on the gate-decision log so
	// pushed cards can be attributed to the session for the retro helpfulness join
	// (#0069) — push is the dominant channel, so without it nearly all served cards
	// are unattributable. Optional; empty records no correlation key.
	Session string `json:"session,omitempty"`
	// Trigger names what prompted this call: "" or "prompt" (the UserPromptSubmit
	// hook, a raw prompt) or "error" (the error-pull hook, #0087 — a verbatim
	// error/log line the client already singled out). It maps to
	// index.Query.ErrorTrigger, which relaxes the two-token corroboration rule
	// (#0108) back to the single-token gate for a verbatim error line. Any other
	// value is rejected (400) — a silently-ignored typo would silently reopen the
	// single-token gate for what is actually a raw prompt.
	Trigger string `json:"trigger,omitempty"`
}

// PushResult is the POST /push response: ready-to-inject additionalContext text.
// IDs lists the injected record ids so a client can close the feedback loop —
// call confirm_helpful (or report_outcome) on a pushed card that helped or
// misled. The push impression itself is recorded server-side as `pushed`.
type PushResult struct {
	Count   int      `json:"count"`
	Context string   `json:"context"`
	IDs     []string `json:"ids,omitempty"`
}

func (h *handlers) pushHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const route = "push"

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var args PushArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		if errors.Is(err, io.EOF) {
			http.Error(w, "request body required", http.StatusBadRequest)
			return
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(args.Query) == "" {
		http.Error(w, "query must be non-empty", http.StatusBadRequest)
		return
	}
	if len(args.Query) > maxQueryBytes {
		http.Error(w, fmt.Sprintf("query too large: %d bytes (max %d)", len(args.Query), maxQueryBytes), http.StatusBadRequest)
		return
	}
	switch args.Trigger {
	case "", "prompt", "error":
	default:
		http.Error(w, fmt.Sprintf("trigger must be \"\", \"prompt\", or \"error\", got %q", args.Trigger), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	// Push channel: embedding-free retrieval only (ADR-0001 §4). RetrievePushTraced
	// applies the discriminative-token gate so off-topic prompts inject nothing, and
	// never surfaces quarantined records; its trace feeds per-query telemetry (#0067).
	// trigger=="error" (the error-pull hook, #0087) relaxes the two-token
	// corroboration rule (#0108) back to the single-token gate for a verbatim
	// error line; "" and "prompt" are identical (strict prompt-triggered push).
	decision, err := h.ix.RetrievePushTraced(ctx, index.Query{
		Text:         args.Query,
		Repo:         h.repo,
		ErrorTrigger: args.Trigger == "error",
		Ecosystem:    args.Ecosystem,
		Package:      args.Package,
	})
	if err != nil {
		h.logger.Error("push failed",
			slog.String("route", route),
			slog.String("outcome", "error"),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("error", err.Error()),
		)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}

	cards := make([]string, 0, len(decision.Served))
	ids := make([]string, 0, len(decision.Served))
	for _, hit := range decision.Served {
		stored, err := h.ix.Get(ctx, hit.ID)
		if err != nil {
			// Corpus drift (a served id no longer in the records table): drop this
			// card and serve the rest — "empty is a valid answer". An unexpected DB
			// error is genuine infra failure, so it still 500s rather than masking it.
			if errors.Is(err, index.ErrNotFound) {
				h.logger.Warn("push get: served id missing, dropping card",
					slog.String("route", route),
					slog.String("id", hit.ID),
					slog.String("error", err.Error()),
				)
				continue
			}
			h.logger.Error("push get failed",
				slog.String("route", route),
				slog.String("id", hit.ID),
				slog.String("error", err.Error()),
			)
			http.Error(w, "record load failed", http.StatusInternalServerError)
			return
		}
		rec, err := record.Parse(stored.Path, []byte(stored.Markdown))
		if err != nil {
			// A single unparseable served record must not 500 the whole hot-path
			// injection; drop it and serve the others. The served set already passed
			// status:validated gating, so this is resilience, not a policy bypass.
			h.logger.Warn("push parse: dropping unparseable card",
				slog.String("route", route),
				slog.String("id", hit.ID),
				slog.String("error", err.Error()),
			)
			continue
		}
		cards = append(cards, RenderTrapCard(rec))
		ids = append(ids, hit.ID)
	}

	// Record the push impression off the latency budget (ADR-0013 §4), the same
	// seam pull uses for `retrieved` — closing the feedback loop (#0058): `pushed`
	// is the denominator a later confirm_helpful (numerator) is measured against.
	h.usage.recordPush(ids)
	h.recordPushDecision(args.Query, decision, args.Session, args.Trigger)

	out := PushResult{
		Count:   len(cards),
		Context: RenderPushContext(cards),
		IDs:     ids,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("push encode failed", slog.String("error", err.Error()))
		return
	}
	h.logger.Info("push",
		slog.String("route", route),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.Int("count", out.Count),
	)
}

// recordPushDecision logs this query's push gate decision (#0067): which path the
// gate took (fingerprint bypass / discriminative tokens) and what it served, attributed
// to sessionID by a SALTED hash (never the raw id) so the retro helpfulness join can
// confirm only cards served in that session (#0069). Best-effort and async; the raw
// query is hashed, never stored. An empty sessionID records no key. nil recorder = no-op.
// trigger is the caller's PushArgs.Trigger ("" | "prompt" | "error"); "" and "prompt"
// normalize to "prompt" on the log (#0116) — they are semantically identical
// (ADR-0028 decision 4), so the served-rate split by trigger has no empty bucket.
func (h *handlers) recordPushDecision(query string, d index.PushDecision, sessionID, trigger string) {
	if h.telemetry == nil {
		return
	}
	served := make([]telemetry.ServedHit, len(d.Served))
	for i, hit := range d.Served {
		served[i] = telemetry.ServedHit{ID: hit.ID, Score: hit.Score}
	}
	session := ""
	if sessionID != "" {
		session = h.telemetry.Hash(sessionID)
	}
	queryText := ""
	if h.queryText {
		queryText = truncateQueryText(query)
	}
	if trigger == "" {
		trigger = "prompt"
	}
	h.telemetry.Record(telemetry.Decision{
		Channel:           "push",
		QueryHash:         h.telemetry.Hash(query),
		QueryText:         queryText,
		Session:           session,
		Tokens:            d.Discriminative,
		FingerprintBypass: d.FingerprintBypass,
		Served:            served,
		Count:             len(d.Served),
		Trigger:           trigger,
	})
}

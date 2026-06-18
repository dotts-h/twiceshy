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
)

// PushArgs is the POST /push request body.
type PushArgs struct {
	Query     string `json:"query"`
	Ecosystem string `json:"ecosystem,omitempty"`
	Package   string `json:"package,omitempty"`
}

// PushResult is the POST /push response: ready-to-inject additionalContext text.
type PushResult struct {
	Count   int    `json:"count"`
	Context string `json:"context"`
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

	ctx := r.Context()
	// Push channel: embedding-free retrieval only (ADR-0001 §4). Never
	// IncludeQuarantined — quarantined records never enter the push channel.
	hits, err := h.ix.Retrieve(ctx, index.Query{
		Text:      args.Query,
		Repo:      h.repo,
		Ecosystem: args.Ecosystem,
		Package:   args.Package,
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

	cards := make([]string, 0, len(hits))
	for _, hit := range hits {
		stored, err := h.ix.Get(ctx, hit.ID)
		if err != nil {
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
			h.logger.Error("push parse failed",
				slog.String("route", route),
				slog.String("id", hit.ID),
				slog.String("error", err.Error()),
			)
			http.Error(w, "record parse failed", http.StatusInternalServerError)
			return
		}
		cards = append(cards, RenderTrapCard(rec))
	}

	out := PushResult{
		Count:   len(cards),
		Context: RenderPushContext(cards),
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

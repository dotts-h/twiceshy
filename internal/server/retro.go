// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/screen"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// RetroArgs is the POST /retro request body: a bounded session transcript the
// SessionEnd hook ships for off-pool analysis (#0065, ADR-0018).
type RetroArgs struct {
	Transcript string `json:"transcript"`
	Author     string `json:"author,omitempty"`
	Session    string `json:"session,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// RetroResult is the POST /retro response: the transcript was screened and queued
// for the retro-intake driver. Nothing is analyzed on the request path.
type RetroResult struct {
	Queued bool   `json:"queued"`
	Status string `json:"status"`
}

// retroHTTP screens a session transcript for secrets and spools it for the
// retro-intake driver (#0065, ADR-0018). The expensive off-pool analysis runs in
// the driver, never here: the edge stays thin and DoS-resistant. A secret in the
// transcript is refused outright — it must never land on disk. The route is
// registered unqualified (not "POST /retro") so a non-POST returns a clean 405
// from here rather than falling through to the MCP catch-all (exp-0006).
func (h *handlers) retroHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const route = "retro"

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.retroQueue == "" {
		http.Error(w, "retro capture is not enabled on this server", http.StatusServiceUnavailable)
		return
	}

	var args RetroArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			http.Error(w, "request body required", http.StatusBadRequest)
		case errors.As(err, &maxErr):
			http.Error(w, "transcript too large", http.StatusRequestEntityTooLarge)
		default:
			http.Error(w, "invalid json", http.StatusBadRequest)
		}
		return
	}
	if strings.TrimSpace(args.Transcript) == "" {
		http.Error(w, "transcript must be non-empty", http.StatusBadRequest)
		return
	}
	h.recordTenantCall(r.Context(), "retro")

	// Screen at the edge: a secret-bearing transcript is refused and never spooled
	// (fail-closed). harmful-code / pii findings are expected in a coding transcript
	// (shell snippets, private IPs) and do not block — the analyzer sees them framed
	// as DATA and intake screens the built record again (defense in depth).
	if screen.HasSecret(screen.Scan(args.Transcript)) {
		h.logger.Warn("retro refused: secret in transcript", slog.String("route", route))
		http.Error(w, "transcript contains a secret; refused", http.StatusUnprocessableEntity)
		return
	}

	author := strings.TrimSpace(args.Author)
	if author == "" {
		author = "claude"
	}
	path, err := spool.EnqueueTranscript(h.retroQueue, spool.Transcript{
		SessionID:  args.Session,
		Author:     author,
		Reason:     args.Reason,
		Transcript: args.Transcript,
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		h.logger.Error("retro enqueue failed",
			slog.String("route", route),
			slog.String("error", err.Error()),
		)
		http.Error(w, "could not queue transcript", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(RetroResult{Queued: true, Status: "queued"}); err != nil {
		h.logger.Error("retro encode failed", slog.String("error", err.Error()))
		return
	}
	h.logger.Info("retro",
		slog.String("route", route),
		slog.String("outcome", "queued"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("session", args.Session),
		slog.String("path", path),
	)
}

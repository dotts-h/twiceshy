// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// statzTopRecordsLimit bounds the "top_records" block (#0126) — a bounded
// dashboard summary, not a full export.
const statzTopRecordsLimit = 10

// statzResponse is the GET /statz body: an operator-only snapshot of corpus
// health, aggregate usage, and per-tenant activity (#0126).
type statzResponse struct {
	Records     statzRecords     `json:"records"`
	UsageTotals statzUsageTotals `json:"usage_totals"`
	Tenants     []statzTenant    `json:"tenants"`
	TopRecords  []statzTopRecord `json:"top_records"`
}

type statzRecords struct {
	Validated   int `json:"validated"`
	Quarantined int `json:"quarantined"`
	Total       int `json:"total"`
}

type statzUsageTotals struct {
	Pushed           int `json:"pushed"`
	Retrieved        int `json:"retrieved"`
	ConfirmedHelpful int `json:"confirmed_helpful"`
}

type statzTenant struct {
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Revoked    bool           `json:"revoked"`
	DailyQuota int            `json:"daily_quota"`
	CallsToday int            `json:"calls_today"`
	Calls7d    int            `json:"calls_7d"`
	TopTools   map[string]int `json:"top_tools"`
}

type statzTopRecord struct {
	ID        string `json:"id"`
	Retrieved int    `json:"retrieved"`
	Pushed    int    `json:"pushed"`
	Title     string `json:"title"`
}

// statzHTTP serves the operator-only dashboard stats (#0126). Registered
// unqualified on the authed mux (same pattern as /push, /retro): tenantAuth
// has already validated SOME bearer token by the time this runs, but the
// stats here are operator-only, so any tok_ tenant is rejected with 403 —
// distinct from tenantAuth's 401 (no/bad credentials at all).
func (h *handlers) statzHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if TenantFromContext(r.Context()) != "operator" {
		http.Error(w, "operator only", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	now := time.Now()

	counts, err := h.ix.RecordStatusCounts(ctx)
	if err != nil {
		h.logger.Error("statz: record status counts failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	allUsage, err := h.ix.AllUsage(ctx)
	if err != nil {
		h.logger.Error("statz: all usage failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tenants, err := h.ix.TenantStats(ctx, now)
	if err != nil {
		h.logger.Error("statz: tenant stats failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	top, err := h.ix.TopRecords(ctx, statzTopRecordsLimit)
	if err != nil {
		h.logger.Error("statz: top records failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := statzResponse{
		Records:     statzRecords{Validated: counts.Validated, Quarantined: counts.Quarantined, Total: counts.Total},
		UsageTotals: sumUsageTotals(allUsage),
		Tenants:     make([]statzTenant, 0, len(tenants)),
		TopRecords:  make([]statzTopRecord, 0, len(top)),
	}
	for _, t := range tenants {
		resp.Tenants = append(resp.Tenants, statzTenant{
			ID:         t.ID,
			Label:      t.Label,
			Revoked:    t.Revoked,
			DailyQuota: t.DailyQuota,
			CallsToday: t.CallsToday,
			Calls7d:    t.Calls7d,
			TopTools:   t.TopTools,
		})
	}
	for _, tr := range top {
		resp.TopRecords = append(resp.TopRecords, statzTopRecord{ID: tr.ID, Retrieved: tr.Retrieved, Pushed: tr.Pushed, Title: tr.Title})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("statz encode failed", slog.String("error", err.Error()))
	}
}

// sumUsageTotals folds AllUsage's per-record map into the "usage_totals" block.
func sumUsageTotals(all map[string]record.Usage) statzUsageTotals {
	var out statzUsageTotals
	for _, u := range all {
		out.Pushed += u.Pushed
		out.Retrieved += u.Retrieved
		out.ConfirmedHelpful += u.ConfirmedHelpful
	}
	return out
}

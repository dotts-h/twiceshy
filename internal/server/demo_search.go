// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	demoGlobalPerDay   = 500
	demoMaxPerIPPerDay = 20
	maxDemoQueryBytes  = 200
)

type demoLimiter struct {
	mu          sync.Mutex
	day         string
	ipCounts    map[string]int
	globalCount int
	now         func() time.Time
}

func newDemoLimiter(now func() time.Time) *demoLimiter {
	if now == nil {
		now = time.Now
	}
	return &demoLimiter{
		ipCounts: make(map[string]int),
		now:      now,
	}
}

func (l *demoLimiter) allow(ip string) (bool, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := l.now().UTC().Format("2006-01-02")
	if day != l.day {
		l.day = day
		l.ipCounts = make(map[string]int)
		l.globalCount = 0
	}
	if l.globalCount >= demoGlobalPerDay {
		return false, "global_limit_exceeded"
	}
	if l.ipCounts[ip] >= demoMaxPerIPPerDay {
		return false, "ip_limit_exceeded"
	}
	l.globalCount++
	l.ipCounts[ip]++
	return true, ""
}

type DemoSearchHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

type DemoSearchResult struct {
	Hits []DemoSearchHit `json:"hits"`
}

func (h *handlers) demoSearchHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.demoEnabled {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := resolveSignupClientIP(r, h.trustedProxies)
	if allowed, limitErr := h.demoLimiter.allow(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(secondsUntilUTCMidnight(time.Now())))
		writeDemoError(w, http.StatusTooManyRequests, limitErr)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeDemoError(w, http.StatusBadRequest, "empty_query")
		return
	}
	if len(q) > maxDemoQueryBytes {
		writeDemoError(w, http.StatusBadRequest, "query_too_long")
		return
	}

	hits, err := h.pullSearchHits(r.Context(), SearchArgs{
		Query:              q,
		K:                  3,
		IncludeQuarantined: false,
	})
	if err != nil {
		h.logger.Error("demo-search failed", slog.String("error", err.Error()))
		writeDemoError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	outHits := make([]DemoSearchHit, 0, len(hits))
	for _, hit := range hits {
		outHits = append(outHits, DemoSearchHit{
			ID:      hit.ID,
			Title:   capText(sanitizeForTransport(hit.Title), maxSearchTitleBytes),
			Kind:    hit.Kind,
			Summary: capText(sanitizeForTransport(hit.Summary), maxSearchSummaryBytes),
		})
	}

	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}
	h.usage.record(ids)
	h.recordSearchDecision(q, hits, "")
	if h.tenantCalls != nil {
		if err := h.tenantCalls.CountTenantCall("demo", "demo-search", time.Now()); err != nil {
			h.logger.Warn("demo-search tenant usage record failed", slog.String("error", err.Error()))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(DemoSearchResult{Hits: outHits})
}

func writeDemoError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", reason)
}

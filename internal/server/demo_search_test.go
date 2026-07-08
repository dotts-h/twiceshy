// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
)

func newDemoServer(t *testing.T, enabled bool, recs []*record.Record, trustedProxies []*net.IPNet) (*httptest.Server, *index.Index) {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })

	if len(recs) > 0 {
		if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
			t.Fatalf("Rebuild: %v", err)
		}
	}

	cfg := server.Config{
		Index:          ix,
		Token:          token,
		DemoEnabled:    enabled,
		TrustedProxies: trustedProxies,
	}
	h, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, ix
}

func getDemoSearch(t *testing.T, tsURL, query string) *http.Response {
	t.Helper()
	target := tsURL + "/demo-search"
	if query != "" {
		target += "?q=" + url.QueryEscape(query)
	}
	resp, err := http.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestDemoSearchDisabledReturns404(t *testing.T) {
	ts, _ := newDemoServer(t, false, nil, nil)
	resp := getDemoSearch(t, ts.URL, "anything")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when demo is disabled", resp.StatusCode)
	}
}

func TestDemoSearchValidation(t *testing.T) {
	ts, _ := newDemoServer(t, true, nil, nil)

	// Case 1: Empty query
	t.Run("empty query", func(t *testing.T) {
		resp := getDemoSearch(t, ts.URL, "")
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		var out map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != "empty_query" {
			t.Fatalf("error = %q, want empty_query", out["error"])
		}
	})

	// Case 2: Max 200 bytes limit
	t.Run("over-length query", func(t *testing.T) {
		largeQuery := strings.Repeat("a", 201)
		resp := getDemoSearch(t, ts.URL, largeQuery)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		var out map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != "query_too_long" {
			t.Fatalf("error = %q, want query_too_long", out["error"])
		}
	})
}

func TestDemoSearchHappyPathAndClamping(t *testing.T) {
	// Create dummy records so FTS BM25 idf of commonmatch is high
	var recs []*record.Record
	for i := 1; i <= 20; i++ {
		dummy := &record.Record{
			SchemaVersion: 1,
			ID:            record.FormatID(100 + i),
			Kind:          "workflow",
			Status:        "validated",
			Title:         "Dummy record title",
			Symptom: &record.Symptom{
				Summary: "Dummy summary.",
			},
			Body: "Dummy body without match.",
		}
		dummy.Provenance.Source.Author = "author"
		recs = append(recs, dummy)
	}

	// Create validated and quarantined test records matching "commonmatch"
	r1 := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0001",
		Kind:          "trap",
		Status:        "validated",
		Title:         "Avoid calling read file commonmatch",
		Symptom: &record.Symptom{
			Summary: "A error occurred with commonmatch.",
		},
		Body: "Full narrative here about commonmatch.",
	}
	r1.Provenance.Source.Author = "author1"
	recs = append(recs, r1)

	r2 := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0002",
		Kind:          "fix",
		Status:        "validated",
		Title:         "Fix check in code paths commonmatch",
		Symptom: &record.Symptom{
			Summary: "checks were bypassed commonmatch.",
		},
		Body: "Full narrative about commonmatch.",
	}
	r2.Provenance.Source.Author = "author2"
	recs = append(recs, r2)

	r3 := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0003",
		Kind:          "convention",
		Status:        "validated",
		Title:         "Convention on checking commonmatch",
		Symptom: &record.Symptom{
			Summary: "Standard checking rules commonmatch.",
		},
		Body: "Full narrative about checking commonmatch rules.",
	}
	r3.Provenance.Source.Author = "author3"
	recs = append(recs, r3)

	// More than 3 records to test clamping
	r4 := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0004",
		Kind:          "workflow",
		Status:        "validated",
		Title:         "Workflow for setting up commonmatch",
		Symptom: &record.Symptom{
			Summary: "Steps to configure commonmatch.",
		},
		Body: "Full narrative about commonmatch configuration workflows.",
	}
	r4.Provenance.Source.Author = "author4"
	recs = append(recs, r4)

	// Quarantined record matching same keywords
	rQuar := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0005",
		Kind:          "trap",
		Status:        "quarantined",
		Title:         "Avoid commonmatch issues on systems",
		Symptom: &record.Symptom{
			Summary: "quarantined errors commonmatch.",
		},
		Body: "Full narrative on system commonmatch.",
	}
	rQuar.Provenance.Source.Author = "author5"
	recs = append(recs, rQuar)

	ts, _ := newDemoServer(t, true, recs, nil)

	t.Run("happy path fields", func(t *testing.T) {
		resp := getDemoSearch(t, ts.URL, "commonmatch")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var out struct {
			Hits []map[string]any `json:"hits"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}

		// Clamped to 3 hits
		if len(out.Hits) != 3 {
			t.Fatalf("len(hits) = %d, want 3 (clamped)", len(out.Hits))
		}

		// Check fields returned in hits: id, title, kind, summary
		for _, hit := range out.Hits {
			if len(hit) != 4 {
				t.Fatalf("returned hit has %d fields, want exactly 4: %+v", len(hit), hit)
			}
			for _, field := range []string{"id", "title", "kind", "summary"} {
				if _, ok := hit[field]; !ok {
					t.Fatalf("hit missing field %q: %+v", field, hit)
				}
			}
		}

		// Ensure quarantined record (exp-0005) is not among hits
		for _, hit := range out.Hits {
			if hit["id"] == "exp-0005" {
				t.Fatal("quarantined record exp-0005 was surfaced")
			}
		}
	})

	t.Run("empty result handles honestly", func(t *testing.T) {
		resp := getDemoSearch(t, ts.URL, "nomatchquery")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var out struct {
			Hits []map[string]any `json:"hits"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}

		if out.Hits == nil || len(out.Hits) != 0 {
			t.Fatalf("expected empty hits array, got %+v", out.Hits)
		}
	})
}

func TestDemoSearchRateLimits(t *testing.T) {
	ts, _ := newDemoServer(t, true, nil, nil)

	// 20 requests/day per IP limit
	t.Run("per-ip limit 429", func(t *testing.T) {
		// Run 20 requests (IP is loopback/default)
		for i := 0; i < 20; i++ {
			resp := getDemoSearch(t, ts.URL, "test")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("request %d: status = %d, want 200", i+1, resp.StatusCode)
			}
		}

		// 21st request from same IP should get 429
		resp := getDemoSearch(t, ts.URL, "test")
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("21st request: status = %d, want 429", resp.StatusCode)
		}

		var out map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != "ip_limit_exceeded" {
			t.Fatalf("error = %q, want ip_limit_exceeded", out["error"])
		}
	})
}

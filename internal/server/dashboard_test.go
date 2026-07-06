// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

func newStatzServer(t *testing.T) (*httptest.Server, *index.Index) {
	t.Helper()
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token, TokenStore: ix, Repo: testRepo, RecordCount: len(recs)})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, ix
}

func getStatz(t *testing.T, ts *httptest.Server, bearer string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/statz", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func TestStatzRequiresAuth(t *testing.T) {
	ts, _ := newStatzServer(t)
	code, _ := getStatz(t, ts, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("no bearer: status = %d, want 401", code)
	}
}

func TestStatzRejectsTenantToken(t *testing.T) {
	ts, ix := newStatzServer(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	full, _, err := ix.IssueToken("alice", 1000, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	code, body := getStatz(t, ts, full)
	if code != http.StatusForbidden {
		t.Fatalf("tok_ tenant: status = %d, want 403, body=%s", code, body)
	}
}

func TestStatzOperatorTokenReturnsShape(t *testing.T) {
	ts, ix := newStatzServer(t)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	_, id, err := ix.IssueToken("bob", 500, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.CountTenantCall(id, "search_experience", now); err != nil {
		t.Fatal(err)
	}
	if err := ix.RecordHits(context.Background(), []string{"exp-0001"}, "2026-07-06"); err != nil {
		t.Fatal(err)
	}

	code, body := getStatz(t, ts, token)
	if code != http.StatusOK {
		t.Fatalf("operator token: status = %d, want 200, body=%s", code, body)
	}

	var got struct {
		Records struct {
			Validated   int `json:"validated"`
			Quarantined int `json:"quarantined"`
			Total       int `json:"total"`
		} `json:"records"`
		UsageTotals struct {
			Pushed           int `json:"pushed"`
			Retrieved        int `json:"retrieved"`
			ConfirmedHelpful int `json:"confirmed_helpful"`
		} `json:"usage_totals"`
		Tenants []struct {
			ID         string         `json:"id"`
			Label      string         `json:"label"`
			Revoked    bool           `json:"revoked"`
			DailyQuota int            `json:"daily_quota"`
			CallsToday int            `json:"calls_today"`
			Calls7d    int            `json:"calls_7d"`
			TopTools   map[string]int `json:"top_tools"`
		} `json:"tenants"`
		TopRecords []struct {
			ID        string `json:"id"`
			Retrieved int    `json:"retrieved"`
			Pushed    int    `json:"pushed"`
			Title     string `json:"title"`
		} `json:"top_records"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode /statz body: %v\nbody=%s", err, body)
	}

	if got.Records.Total == 0 || got.Records.Validated == 0 {
		t.Fatalf("records block empty: %+v", got.Records)
	}
	if got.UsageTotals.Retrieved != 1 {
		t.Fatalf("usage_totals.retrieved = %d, want 1", got.UsageTotals.Retrieved)
	}
	found := false
	for _, tn := range got.Tenants {
		if tn.ID == id {
			found = true
			if tn.Label != "bob" || tn.DailyQuota != 500 {
				t.Errorf("tenant row = %+v", tn)
			}
			if tn.Calls7d != 1 || tn.TopTools["search_experience"] != 1 {
				t.Errorf("tenant usage = %+v, want calls_7d=1 top_tools[search_experience]=1", tn)
			}
		}
	}
	if !found {
		t.Fatalf("tenants missing issued token %s: %+v", id, got.Tenants)
	}
	topFound := false
	for _, tr := range got.TopRecords {
		if tr.ID == "exp-0001" {
			topFound = true
			if tr.Title == "" {
				t.Error("top_records entry missing title")
			}
		}
	}
	if !topFound {
		t.Fatalf("top_records missing exp-0001: %+v", got.TopRecords)
	}
}

func TestStatzMethodNotAllowed(t *testing.T) {
	ts, _ := newStatzServer(t)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/statz", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /statz: status = %d, want 405", resp.StatusCode)
	}
}

// TestToolCallRecordsTenantUsage drives a real MCP tools/call through the full
// HTTP stack with a tok_ tenant token, then asserts tenant_usage advanced for
// that exact tenant and tool (#0126) — the same call path production traffic
// takes, not a direct handler invocation.
func TestToolCallRecordsTenantUsage(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	full, id, err := ix.IssueToken("mcp-tenant", 10000, 10000, now)
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token, TokenStore: ix, Repo: testRepo, RecordCount: len(recs)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "twiceshy-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: full}},
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect with tenant token: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": `FTS5: syntax error near "."`},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}

	stats, err := ix.TenantStats(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var got *index.TenantStat
	for i := range stats {
		if stats[i].ID == id {
			got = &stats[i]
		}
	}
	if got == nil {
		t.Fatalf("tenant %s missing from TenantStats: %+v", id, stats)
	}
	if got.TopTools["search_experience"] != 1 {
		t.Fatalf("search_experience calls = %d, want 1", got.TopTools["search_experience"])
	}
}

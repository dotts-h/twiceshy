// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
)

const logTestToken = "s3cret-test-token"

func newLoggedTestServer(t *testing.T, buf *bytes.Buffer) *httptest.Server {
	t.Helper()
	recs, err := record.LoadCorpus("../..")
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
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	h, err := server.New(server.Config{
		Index:  ix,
		Token:  logTestToken,
		Repo:   testRepo,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func TestStructuredLoggingEmitsSafeFields(t *testing.T) {
	var buf bytes.Buffer
	ts := newLoggedTestServer(t, &buf)
	session := connectWithToken(t, ts, logTestToken)

	// A unique sentinel in the query proves query text is never logged.
	const querySentinel = "zzq-leak-sentinel-7f3a"
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": querySentinel + ` FTS5: syntax error near "."`},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("auth failure status = %d, want 401", resp.StatusCode)
	}

	logs := buf.String()
	if strings.Contains(logs, logTestToken) {
		t.Fatalf("bearer token must never appear in logs")
	}
	if strings.Contains(logs, querySentinel) {
		t.Fatalf("caller query text must never appear in logs")
	}

	var sawSearchOK, sawAuthWarn bool
	for line := range strings.SplitSeq(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		if entry["tool"] == "search_experience" && entry["outcome"] == "ok" {
			if _, ok := entry["duration_ms"]; ok {
				sawSearchOK = true
			}
		}
		if entry["level"] == "WARN" && entry["reason"] != nil {
			sawAuthWarn = true
			if strings.Contains(line, logTestToken) {
				t.Fatal("auth warn line must not contain the bearer token")
			}
		}
	}
	if !sawSearchOK {
		t.Fatalf("expected tool=search_experience outcome=ok with duration_ms, got:\n%s", logs)
	}
	if !sawAuthWarn {
		t.Fatalf("expected auth failure warn with reason, got:\n%s", logs)
	}
}

func connectWithToken(t *testing.T, ts *httptest.Server, tok string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "twiceshy-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: tok}},
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

package server_test

import (
	"context"
	"encoding/json"
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

const token = "s3cret-test-token"

func newTestServer(t *testing.T) *httptest.Server {
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
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

func connect(t *testing.T, ts *httptest.Server) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "twiceshy-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestNewRequiresIndexAndToken(t *testing.T) {
	if _, err := server.New(server.Config{Index: nil, Token: "x"}); err == nil {
		t.Error("nil index must be rejected")
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	if _, err := server.New(server.Config{Index: ix, Token: ""}); err == nil {
		t.Error("empty bearer token must be rejected")
	}
}

func TestBearerAuthRejectsBadCredentials(t *testing.T) {
	ts := newTestServer(t)
	for name, header := range map[string]string{
		"no auth":      "",
		"wrong token":  "Bearer wrong",
		"not bearer":   "Basic " + token,
		"empty bearer": "Bearer ",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

// Guarding test for exp-0003: the pull channel is MCP over streamable
// HTTP — a real SDK client must complete the handshake against the
// handler and see both Phase 1 tools.
func TestServerSpeaksStreamableHTTP(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"search_experience", "get_experience"} {
		if !got[want] {
			t.Errorf("missing tool %q (have %v)", want, got)
		}
	}
}

func TestSearchExperienceTool(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

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
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exp-0001", "fingerprint"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("structured output missing %q: %s", want, out)
		}
	}
}

func TestSearchExperienceRejectsEmptyQuery(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": "   "},
	})
	if err == nil && !res.IsError {
		t.Error("blank query must be a tool error")
	}
}

func TestGetExperienceTool(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_experience",
		Arguments: map[string]any{"id": "exp-0001"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exp-0001", "The trap"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("structured output missing %q", want)
		}
	}
}

func TestGetExperienceUnknownIDIsToolError(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_experience",
		Arguments: map[string]any{"id": "exp-9999"},
	})
	if err == nil && !res.IsError {
		t.Error("unknown id must be a tool error, not a silent success")
	}
}

func toolText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

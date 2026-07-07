// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/server"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// newRetroServer builds a server with a retro queue configured (queueDir == ""
// leaves /retro disabled). The /retro path never touches the index, so an empty
// one is enough — no corpus load.
func newRetroServer(t *testing.T, queueDir string) *httptest.Server {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	h, err := server.New(server.Config{Index: ix, Token: token, RetroQueue: queueDir})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func postRetro(t *testing.T, tsURL, bearer string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, tsURL+"/retro", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestRetroUnauthorized(t *testing.T) {
	ts := newRetroServer(t, t.TempDir())
	resp := postRetro(t, ts.URL, "", map[string]any{"transcript": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// A non-POST must get a clean 405 from retroHTTP, not fall through to the MCP
// catch-all — the exact Go 1.22 method-routing trap (exp-0006).
func TestRetroNonPostReturns405(t *testing.T) {
	ts := newRetroServer(t, t.TempDir())
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/retro", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 (must not fall through to the MCP catch-all)", resp.StatusCode)
	}
}

func TestRetroDisabledReturns503(t *testing.T) {
	ts := newRetroServer(t, "") // no retro queue configured
	resp := postRetro(t, ts.URL, token, map[string]any{"transcript": "agent hit a trap"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when retro capture is disabled", resp.StatusCode)
	}
}

func TestRetroEmptyTranscriptReturns400(t *testing.T) {
	ts := newRetroServer(t, t.TempDir())
	resp := postRetro(t, ts.URL, token, map[string]any{"transcript": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty transcript", resp.StatusCode)
	}
}

func TestRetroQueuesCleanTranscript(t *testing.T) {
	queue := t.TempDir()
	ts := newRetroServer(t, queue)
	resp := postRetro(t, ts.URL, token, map[string]any{
		"transcript": "agent hit fts5: syntax error on a dotted token and fixed it by quoting",
		"author":     "claude",
		"session":    "sess-1",
		"reason":     "logout",
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	files, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("queue has %d files, want 1", len(files))
	}
	tr, err := spool.ReadTranscript(files[0])
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if !strings.Contains(tr.Transcript, "fts5: syntax error") {
		t.Errorf("spooled transcript = %q, missing body", tr.Transcript)
	}
	if tr.SessionID != "sess-1" || tr.Author != "claude" || tr.Reason != "logout" {
		t.Errorf("spooled metadata mismatch: %+v", tr)
	}
}

// TestRetroHTTPAlphaTenantForbidden is ADR-0031's /retro invariant (#0136):
// the alpha opens record_experience/report_outcome only — a tok_ tenant is
// refused 403 before any body read/screen/spool work, and nothing is
// spooled. Operator behavior (202) is unaffected — see TestRetroQueuesCleanTranscript.
func TestRetroHTTPAlphaTenantForbidden(t *testing.T) {
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	queue := t.TempDir()
	fullToken, _, err := ix.IssueToken("alpha-retro-forbidden-test", 100000, 100000, time.Now())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token, TokenStore: ix, RetroQueue: queue})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp := postRetro(t, ts.URL, fullToken, map[string]any{"transcript": "an alpha tenant transcript long enough to pass"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (retro capture is operator-only in the alpha)", resp.StatusCode)
	}
	if files, _ := spool.List(queue); len(files) != 0 {
		t.Errorf("an alpha tenant's retro call spooled %d files, want 0", len(files))
	}
}

func TestRetroRefusesSecretAndSpoolsNothing(t *testing.T) {
	queue := t.TempDir()
	ts := newRetroServer(t, queue)
	// Secret-shaped value assembled at run time, never a literal token in any
	// commit (CONVENTIONS: gitleaks scans the whole history).
	secret := "AKIA" + strings.Repeat("A", 16) // matches screen's aws-access-key rule
	resp := postRetro(t, ts.URL, token, map[string]any{
		"transcript": "agent accidentally printed " + secret + " to the log",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a secret-bearing transcript", resp.StatusCode)
	}
	if files, _ := spool.List(queue); len(files) != 0 {
		t.Errorf("a refused transcript spooled %d files, want 0 (a secret must never land on disk)", len(files))
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/server"
)

// newSignupServer builds a server with the real index as both TokenStore and
// TokenIssuer (its method set satisfies both), so a signed-up token can be
// authenticated against the same store that issued it. logger, when non-nil,
// lets a test probe the log buffer (e.g. for secret leakage).
func newSignupServer(t *testing.T, enabled bool, logger *slog.Logger) (*httptest.Server, *index.Index) {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	cfg := server.Config{Index: ix, Token: token, TokenStore: ix, TokenIssuer: ix, SignupEnabled: enabled, Logger: logger}
	h, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, ix
}

func postSignup(t *testing.T, tsURL string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(tsURL+"/signup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestSignupDisabledReturns404(t *testing.T) {
	ts, _ := newSignupServer(t, false, nil)
	resp := postSignup(t, ts.URL, map[string]any{"email": "a@b.com", "accept_terms": true})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when signup is disabled", resp.StatusCode)
	}
}

func TestSignupTermsNotAccepted400(t *testing.T) {
	ts, _ := newSignupServer(t, true, nil)
	resp := postSignup(t, ts.URL, map[string]any{"email": "a@b.com", "accept_terms": false})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "terms_not_accepted" {
		t.Fatalf("error = %q, want terms_not_accepted", body["error"])
	}
}

func TestSignupBadEmailsReturn400(t *testing.T) {
	cases := []struct {
		name  string
		email string
	}{
		{"empty", ""},
		{"too_short", "a@"},
		{"no_at", "not-an-email.com"},
		{"two_at", "a@b@c.com"},
		{"empty_local", "@example.com"},
		{"empty_domain", "a@"},
		{"no_dot_in_domain", "a@localhost"},
		{"too_long", strings.Repeat("a", 250) + "@b.co"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, _ := newSignupServer(t, true, nil)
			resp := postSignup(t, ts.URL, map[string]any{"email": tc.email, "accept_terms": true})
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("email %q: status = %d, want 400", tc.email, resp.StatusCode)
			}
		})
	}
}

func TestSignupHappyPathIssuesWorkingToken(t *testing.T) {
	ts, ix := newSignupServer(t, true, nil)
	resp := postSignup(t, ts.URL, map[string]any{"email": " User@Example.COM ", "accept_terms": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Token       string `json:"token"`
		QuotaPerDay int    `json:"quota_per_day"`
		RatePerMin  int    `json:"rate_per_min"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Token == "" {
		t.Fatal("token must not be empty")
	}
	if out.QuotaPerDay != 1000 {
		t.Errorf("quota_per_day = %d, want 1000", out.QuotaPerDay)
	}
	if out.RatePerMin != 60 {
		t.Errorf("rate_per_min = %d, want 60", out.RatePerMin)
	}

	// The issued token must actually authenticate against the store it came from.
	info, err := ix.AuthenticateToken(out.Token, time.Now())
	if err != nil {
		t.Fatalf("AuthenticateToken(issued token): %v", err)
	}
	if info.Label != "user@example.com" {
		t.Errorf("label = %q, want the trimmed/lowercased email", info.Label)
	}
	if info.DailyQuota != 1000 || info.RatePerMin != 60 {
		t.Errorf("stored quota/rate = %d/%d, want 1000/60", info.DailyQuota, info.RatePerMin)
	}
}

func TestSignupFourthSameIPReturns429(t *testing.T) {
	ts, _ := newSignupServer(t, true, nil)
	emails := []string{"one@example.com", "two@example.com", "three@example.com", "four@example.com"}
	var codes []int
	for _, e := range emails {
		resp := postSignup(t, ts.URL, map[string]any{"email": e, "accept_terms": true})
		codes = append(codes, resp.StatusCode)
	}
	for i := 0; i < 3; i++ {
		if codes[i] != http.StatusOK {
			t.Fatalf("signup %d: status = %d, want 200", i+1, codes[i])
		}
	}
	if codes[3] != http.StatusTooManyRequests {
		t.Fatalf("4th signup from the same IP: status = %d, want 429", codes[3])
	}
}

func TestSignupSecretNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	ts, _ := newSignupServer(t, true, logger)
	resp := postSignup(t, ts.URL, map[string]any{"email": "secret-probe@example.com", "accept_terms": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Token == "" {
		t.Fatal("token must not be empty")
	}
	if strings.Contains(buf.String(), out.Token) {
		t.Fatalf("logs must never contain the issued secret token, got: %s", buf.String())
	}
}

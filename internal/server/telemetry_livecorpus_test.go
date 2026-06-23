//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

func readDecisions(t *testing.T, path string) []telemetry.Decision {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open telemetry log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var out []telemetry.Decision
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var d telemetry.Decision
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			t.Fatalf("decode %q: %v", sc.Text(), err)
		}
		out = append(out, d)
	}
	return out
}

// TestTelemetryRecordsPushGateDecision tests the push handler records a per-query
// gate decision (#0067): the served ids + scores + the discriminative gate tokens
// for a hit, an empty decision for an off-topic query — and the raw query is hashed,
// never persisted. BM25 + the discriminative-df gate are corpus-relative (exp-0098),
// so this exercises the gate against the real corpus, the way push runs in production.
func TestTelemetryRecordsPushGateDecision(t *testing.T) {
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Skipf("live corpus unavailable at ../.. (decoupled to twiceshy-corpus, ADR-0021): %v", err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: logPath, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token, Repo: testRepo, Telemetry: tel})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	// A discriminative-token query serves a card; an off-topic prompt serves nothing.
	if _, out := postPush(t, ts.URL, token, map[string]string{"query": `FTS5: syntax error near "."`}); out.Count < 1 {
		t.Fatalf("discriminative query should serve at least one card, got %d", out.Count)
	}
	if _, out := postPush(t, ts.URL, token, map[string]string{"query": "write a haiku about cats"}); out.Count != 0 {
		t.Fatalf("off-topic query should serve nothing, got %d", out.Count)
	}

	if err := tel.Close(); err != nil { // flush the async writer
		t.Fatalf("Close: %v", err)
	}

	decs := readDecisions(t, logPath)
	if len(decs) != 2 {
		t.Fatalf("want 2 recorded decisions, got %d: %+v", len(decs), decs)
	}
	var served, empty *telemetry.Decision
	for i := range decs {
		if decs[i].Channel != "push" {
			t.Fatalf("channel = %q, want push", decs[i].Channel)
		}
		if decs[i].Count > 0 {
			served = &decs[i]
		} else {
			empty = &decs[i]
		}
	}
	if served == nil || empty == nil {
		t.Fatalf("expected one served + one gate-closed decision, got %+v", decs)
	}
	if len(served.Served) == 0 || served.Served[0].ID == "" || served.Count != len(served.Served) {
		t.Errorf("served decision must carry the served ids: %+v", served)
	}
	// FTS5: syntax error matches a stored fingerprint exactly, so the gate records
	// a fingerprint bypass (the deterministic path) rather than a token match.
	if !served.FingerprintBypass {
		t.Errorf("a fingerprint-exact query must record fp_bypass: %+v", served)
	}
	if served.QueryHash == "" || strings.Contains(served.QueryHash, "FTS5") {
		t.Errorf("the raw query must be hashed, not stored: %q", served.QueryHash)
	}
}

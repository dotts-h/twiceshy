// SPDX-License-Identifier: AGPL-3.0-only

package judge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

func TestJudgeTimeout_EnvOverride(t *testing.T) {
	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "7")
	if got := judgeTimeout(); got != 7*time.Second {
		t.Fatalf("env override: want 7s, got %v", got)
	}
	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "")
	if got := judgeTimeout(); got != judgeHTTPTimeout {
		t.Fatalf("empty falls back to default %v, got %v", judgeHTTPTimeout, got)
	}
	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "garbage")
	if got := judgeTimeout(); got != judgeHTTPTimeout {
		t.Fatalf("garbage falls back to default, got %v", got)
	}
	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "0")
	if got := judgeTimeout(); got != judgeHTTPTimeout {
		t.Fatalf("non-positive falls back to default, got %v", got)
	}
}

// A hung-but-connected upstream — accepts the TCP connection, then never responds —
// must cause Judge to FAIL FAST with an error within ~the configured timeout, not
// block forever. This is the freeze-class regression guard: the 2-week "corpus does
// nothing" was a wedged judge socket, and http.Client.Timeout must bound it.
func TestJudge_HungUpstreamTimesOutInsteadOfHanging(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block // accept the request, then never write a response
	}))
	// Unblock the handler BEFORE closing the server: srv.Close() waits for the
	// outstanding (hung) request, so the channel must be closed first.
	t.Cleanup(func() { close(block); srv.Close() })

	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "1") // 1s bound so the test is fast
	j, err := NewModelJudge(Config{Endpoint: srv.URL, Model: "gemini-2.5-pro", Advisory: true})
	if err != nil {
		t.Fatalf("NewModelJudge: %v", err)
	}
	req := Request{Record: &record.Record{
		Symptom:    &record.Symptom{ErrorSignatures: []string{"GHSA-aaaa-bbbb-cccc"}},
		AppliesTo:  []record.AppliesTo{{Ecosystem: "Go", Package: "example.com/x"}},
		Resolution: &record.Resolution{Fix: "x"},
	}}

	done := make(chan error, 1)
	start := time.Now()
	go func() { _, e := j.Judge(context.Background(), req); done <- e }()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("a hung upstream must yield an error (no-verdict), got nil")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("timeout fired too late (%v) — not bounded by TWICESHY_JUDGE_TIMEOUT", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Judge hung past the configured timeout — the freeze regression is back")
	}
}

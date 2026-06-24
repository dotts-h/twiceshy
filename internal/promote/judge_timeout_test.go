// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
)

// A panel member whose upstream hangs (accepts the connection, never responds) must
// NOT wedge the promote run: the judge call times out, the unanimous panel reports
// no-verdict, and Promote returns a fail-safe HELD outcome bounded by the timeout —
// so a batch loop continues to the next record instead of blocking until the
// systemd SIGTERM. This is the regression guard for the 2-week "corpus does nothing"
// freeze.
func TestPromote_HungJudgeUpstream_HeldNotHung(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block // accept, then never respond
	}))
	// Unblock the handler BEFORE closing the server: srv.Close() waits for the
	// outstanding (hung) request, so the channel must be closed first.
	t.Cleanup(func() { close(block); srv.Close() })

	t.Setenv("TWICESHY_JUDGE_TIMEOUT", "1") // bound the hung call to 1s
	hung, err := judge.NewModelJudge(judge.Config{Endpoint: srv.URL, Model: "gemini-2.5-pro", Advisory: true})
	if err != nil {
		t.Fatalf("NewModelJudge: %v", err)
	}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: hung},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(panel))
	rec := advisoryRecord() // clean (carries a fixed version) — reaches the panel

	type result struct {
		out promote.Outcome
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		out, e := p.Promote(context.Background(), rec)
		done <- result{out, e}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("a hung judge must be a fail-safe hold, not a hard error: %v", r.err)
		}
		if r.out.Promoted {
			t.Fatal("must not promote when a panel member is unreachable")
		}
		if rec.Status != "quarantined" {
			t.Fatalf("record must stay quarantined (held), got %q", rec.Status)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("Promote took %v — the per-call timeout isn't bounding the hung member", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Promote hung on the wedged judge — freeze regression")
	}
}

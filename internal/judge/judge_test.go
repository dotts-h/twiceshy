// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// sampleRequest builds a proven-record judging request for the model-edge tests.
func sampleRequest() judge.Request {
	return judge.Request{
		Record: &record.Record{
			ID:     "exp-0043",
			Kind:   "trap",
			Status: "quarantined",
			Title:  "io/ioutil is deprecated — ReadAll/ReadFile moved to io and os in Go 1.16",
			AppliesTo: []record.AppliesTo{
				{Ecosystem: "Go", Package: "io/ioutil"},
			},
			Resolution: &record.Resolution{
				RootCause: "ioutil was a grab-bag; Go 1.16 redistributed it.",
				Fix:       "Use io.ReadAll and os.ReadFile.",
			},
		},
		Attestation: repro.Attestation{
			RecordID:        "exp-0043",
			RanAt:           "2026-06-19T00:00:00Z",
			Holds:           true,
			Inconclusive:    false,
			ReproducedUnder: []string{"go1.25"},
		},
		Repros: []judge.ReproArtifact{
			{Path: "experience/repro/0043-ioutil.sh", Kind: "positive", Content: "#!/bin/sh\ngo build ./... 2>&1 | grep ioutil"},
		},
	}
}

// approveBody is a well-formed approving verdict the four checks all pass.
func approveBody() string {
	return `{"decision":"approve","checks":[
		{"check":"meaning","pass":true,"reason":"repro exercises the deprecation"},
		{"check":"scope","pass":true,"reason":"applies_to matches"},
		{"check":"license","pass":true,"reason":"facts only"},
		{"check":"poison","pass":true,"reason":"accurate, not misleading"}]}`
}

func TestVerdictApproved(t *testing.T) {
	all := func(p1, p2, p3, p4 bool) []judge.Check {
		return []judge.Check{
			{Name: judge.Meaning, Pass: p1},
			{Name: judge.Scope, Pass: p2},
			{Name: judge.License, Pass: p3},
			{Name: judge.Poison, Pass: p4},
		}
	}
	cases := []struct {
		name string
		v    judge.Verdict
		want bool
	}{
		{"approve all four pass", judge.Verdict{Decision: judge.Approve, Checks: all(true, true, true, true)}, true},
		{"reject blocks", judge.Verdict{Decision: judge.Reject, Checks: all(true, true, true, true)}, false},
		{"approve but one check fails", judge.Verdict{Decision: judge.Approve, Checks: all(true, false, true, true)}, false},
		{"approve but a check missing", judge.Verdict{Decision: judge.Approve, Checks: all(true, true, true, true)[:3]}, false},
		{"empty verdict (fail-safe default)", judge.Verdict{}, false},
		{"approve with a failing extra check", judge.Verdict{Decision: judge.Approve, Checks: append(all(true, true, true, true), judge.Check{Name: "extra", Pass: false})}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.Approved(); got != c.want {
				t.Fatalf("Approved() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNewModelJudgeRejectsLocalModel(t *testing.T) {
	for _, m := range []string{"llama3.2", "qwen2.5-coder", "nomic-embed-text", "codellama:13b"} {
		if _, err := judge.NewModelJudge(judge.Config{Endpoint: "http://judge.local", Model: m}); err == nil {
			t.Fatalf("NewModelJudge(%q) = nil error; the cheap local model must be rejected by construction", m)
		}
	}
}

func TestNewModelJudgeRejectsMonoculture(t *testing.T) {
	_, err := judge.NewModelJudge(judge.Config{
		Endpoint:     "http://judge.local",
		Model:        "claude-opus-4-8",
		DrafterModel: "claude-sonnet-4-6",
	})
	if err == nil {
		t.Fatal("NewModelJudge: judge sharing the drafter's family must be rejected (anti-monoculture)")
	}
}

func TestNewModelJudgeAcceptsDiverseFrontier(t *testing.T) {
	j, err := judge.NewModelJudge(judge.Config{
		Endpoint:     "http://judge.local/",
		Model:        "gemini-2.5-pro",
		DrafterModel: "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("NewModelJudge(diverse frontier) errored: %v", err)
	}
	if j == nil {
		t.Fatal("NewModelJudge returned nil judge")
	}
}

func TestNewModelJudgeRequiresEndpointAndModel(t *testing.T) {
	if _, err := judge.NewModelJudge(judge.Config{Model: "gemini-2.5-pro"}); err == nil {
		t.Fatal("missing endpoint must error")
	}
	if _, err := judge.NewModelJudge(judge.Config{Endpoint: "http://judge.local"}); err == nil {
		t.Fatal("missing model must error")
	}
}

func TestModelJudgeApprove(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		_, _ = w.Write([]byte(approveBody()))
	}))
	defer srv.Close()

	j, err := judge.NewModelJudge(judge.Config{Endpoint: srv.URL, Model: "gemini-2.5-pro", DrafterModel: "claude-opus-4-8", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Judge(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Judge errored: %v", err)
	}
	if !v.Approved() {
		t.Fatalf("expected approved verdict, got %+v", v)
	}
	if v.Model != "gemini-2.5-pro" {
		t.Fatalf("verdict Model = %q, want the configured judge model", v.Model)
	}
	// The prompt must carry the proof so the judge can check meaning/scope.
	for _, want := range []string{"exp-0043", "meaning", "scope", "license", "poison", "ioutil"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %q; the judge cannot reason without it.\nbody=%s", want, gotBody)
		}
	}
}

func TestModelJudgeReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decision":"reject","checks":[
			{"check":"meaning","pass":false,"reason":"repro passes for the wrong reason"},
			{"check":"scope","pass":true,"reason":""},
			{"check":"license","pass":true,"reason":""},
			{"check":"poison","pass":true,"reason":""}]}`))
	}))
	defer srv.Close()

	j, _ := judge.NewModelJudge(judge.Config{Endpoint: srv.URL, Model: "gemini-2.5-pro", Client: srv.Client()})
	v, err := j.Judge(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("a valid reject is a verdict, not an error: %v", err)
	}
	if v.Approved() {
		t.Fatal("reject verdict must not be approved")
	}
}

func TestModelJudgeFailSafe(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"500 status", http.StatusInternalServerError, `{"decision":"approve"}`},
		{"empty body", http.StatusOK, ``},
		{"garbled json", http.StatusOK, `{not json`},
		{"unknown decision", http.StatusOK, `{"decision":"maybe","checks":[]}`},
		{"approve missing a check", http.StatusOK, `{"decision":"approve","checks":[
			{"check":"meaning","pass":true},{"check":"scope","pass":true},{"check":"license","pass":true}]}`},
		{"garbled approve (no checks)", http.StatusOK, `{"decision":"approve"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			j, _ := judge.NewModelJudge(judge.Config{Endpoint: srv.URL, Model: "gemini-2.5-pro", Client: srv.Client()})
			v, err := j.Judge(context.Background(), sampleRequest())
			if err == nil {
				t.Fatalf("%s: expected a fail-safe error (no verdict), got verdict %+v", c.name, v)
			}
			if v.Approved() {
				t.Fatalf("%s: a failed judge call must never be approved", c.name)
			}
		})
	}
}

func TestModelJudgeNetworkErrorFailsSafe(t *testing.T) {
	// A closed server → transport error → no verdict.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	j, _ := judge.NewModelJudge(judge.Config{Endpoint: url, Model: "gemini-2.5-pro"})
	if _, err := j.Judge(context.Background(), sampleRequest()); err == nil {
		t.Fatal("transport failure must fail safe (error, not approve)")
	}
}

func TestStubJudge(t *testing.T) {
	stub := &judge.StubJudge{Verdict: judge.ApproveVerdict("test-model")}
	v, err := stub.Judge(context.Background(), sampleRequest())
	if err != nil || !v.Approved() {
		t.Fatalf("approving stub: v=%+v err=%v", v, err)
	}
	if stub.Calls != 1 {
		t.Fatalf("Calls = %d, want 1", stub.Calls)
	}
	// A stub primed with an error fails safe like a real outage.
	estub := &judge.StubJudge{Err: context.DeadlineExceeded}
	if _, err := estub.Judge(context.Background(), sampleRequest()); err == nil {
		t.Fatal("error-primed stub must return the error")
	}
}

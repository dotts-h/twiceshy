// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judgeeval"
)

func TestSelectConfigs(t *testing.T) {
	all, err := judgeeval.SelectConfigs("all")
	if err != nil || len(all) != len(judgeeval.Configs) {
		t.Fatalf("all: got %d configs, err=%v", len(all), err)
	}
	if got, _ := judgeeval.SelectConfigs(""); len(got) != len(judgeeval.Configs) {
		t.Errorf("empty spec should mean all, got %d", len(got))
	}
	sub, err := judgeeval.SelectConfigs("rubric-nothink,prose-nothink")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 2 || sub[0].Name != "rubric-nothink" || sub[1].Name != "prose-nothink" {
		t.Errorf("subset preserves order/selection, got %+v", sub)
	}
	if _, err := judgeeval.SelectConfigs("nope"); err == nil {
		t.Error("an unknown config must error")
	}
}

func TestJudgeEvalBetter(t *testing.T) {
	// Fewest false-approves wins, even at the cost of more false-rejects.
	safe := judgeeval.Result{FalseApproves: 0, FalseRejects: 3}
	loose := judgeeval.Result{FalseApproves: 2, FalseRejects: 0}
	if !judgeeval.Better(safe, loose) {
		t.Error("the fail-safe config (fewer false-approves) must rank higher")
	}
	// Tie on false-approves → fewer false-rejects wins.
	a := judgeeval.Result{FalseApproves: 1, FalseRejects: 1}
	b := judgeeval.Result{FalseApproves: 1, FalseRejects: 4}
	if !judgeeval.Better(a, b) {
		t.Error("tie on false-approve should break on false-reject")
	}
	// Tie on both → fewer errors, then higher accuracy.
	c := judgeeval.Result{FalseApproves: 1, FalseRejects: 1, Errors: 0, Accuracy: 0.9}
	d := judgeeval.Result{FalseApproves: 1, FalseRejects: 1, Errors: 0, Accuracy: 0.8}
	if !judgeeval.Better(c, d) {
		t.Error("final tie-break is higher accuracy")
	}
}

func TestRunJudgeEval_RequiresEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := runJudgeEval(context.Background(), nil, &out, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("expected a missing-endpoint error, got %v", err)
	}
}

func TestConfigNames(t *testing.T) {
	got := judgeeval.ConfigNames()
	for _, want := range []string{"prose-nothink", "rubric-think"} {
		if !strings.Contains(got, want) {
			t.Errorf("configNames() = %q, missing %q", got, want)
		}
	}
}

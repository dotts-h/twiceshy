// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

func TestWrapFrontierFallback_NoFallbackURL_ReturnsPrimary(t *testing.T) {
	primary := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-flash")}
	got, err := wrapFrontierFallback(primary, "", "", "qwen2.5-coder", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != judge.Judge(primary) {
		t.Fatal("with no fallback URL, the primary must be returned unchanged")
	}
}

func TestWrapFrontierFallback_WithFallback_WrapsInFallbackJudge(t *testing.T) {
	primary := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-flash")}
	got, err := wrapFrontierFallback(primary, "http://localhost:8725", "claude-sonnet-4-6", "qwen2.5-coder", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.(judge.FallbackJudge); !ok {
		t.Fatalf("a configured fallback must yield a FallbackJudge; got %T", got)
	}
}

func TestWrapFrontierFallback_BadFallbackModel_Errors(t *testing.T) {
	primary := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-flash")}
	// Empty model has no recognizable family — NewModelJudge rejects it, so the
	// wrap surfaces a config error rather than silently dropping the fallback.
	if _, err := wrapFrontierFallback(primary, "http://localhost:8725", "", "qwen2.5-coder", 1); err == nil {
		t.Fatal("a fallback model with no recognizable family must error")
	}
}

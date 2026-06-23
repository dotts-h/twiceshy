// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

func TestFallback_PrimaryOK_SecondaryUntouched(t *testing.T) {
	primary := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-flash")}
	secondary := &judge.StubJudge{Verdict: judge.ApproveVerdict("claude-sonnet-4-6")}
	f := judge.NewFallback(primary, secondary)

	v, err := f.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Model != "gemini-2.5-flash" {
		t.Fatalf("want the primary verdict; got model %q", v.Model)
	}
	if secondary.Calls != 0 {
		t.Fatalf("secondary must NOT be called when the primary succeeds; calls=%d", secondary.Calls)
	}
}

func TestFallback_PrimaryRejects_NoFallback(t *testing.T) {
	// A reject is a real judgement, not a failure — the secondary must not get to
	// override it (that would double-spend and defeat the gate).
	reject := judge.Verdict{Decision: judge.Reject, Model: "gemini-2.5-flash"}
	primary := &judge.StubJudge{Verdict: reject}
	secondary := &judge.StubJudge{Verdict: judge.ApproveVerdict("claude-sonnet-4-6")}
	f := judge.NewFallback(primary, secondary)

	v, err := f.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Decision != judge.Reject {
		t.Fatalf("a primary reject must stand; got %q", v.Decision)
	}
	if secondary.Calls != 0 {
		t.Fatalf("secondary must NOT be consulted on a primary reject; calls=%d", secondary.Calls)
	}
}

func TestFallback_PrimaryErrors_SecondaryAnswers(t *testing.T) {
	primary := &judge.StubJudge{Err: errors.New("HTTP 429 quota exhausted")}
	secondary := &judge.StubJudge{Verdict: judge.ApproveVerdict("claude-sonnet-4-6")}
	f := judge.NewFallback(primary, secondary)

	v, err := f.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("fallback should recover via the secondary; got error %v", err)
	}
	if v.Model != "claude-sonnet-4-6" {
		t.Fatalf("want the secondary verdict on primary failure; got model %q", v.Model)
	}
	if secondary.Calls != 1 {
		t.Fatalf("secondary must be consulted exactly once; calls=%d", secondary.Calls)
	}
}

func TestFallback_BothError_SurfacesPrimaryFailSafe(t *testing.T) {
	primErr := errors.New("primary down")
	primary := &judge.StubJudge{Err: primErr}
	secondary := &judge.StubJudge{Err: errors.New("secondary down too")}
	f := judge.NewFallback(primary, secondary)

	if _, err := f.Judge(context.Background(), judge.Request{}); !errors.Is(err, primErr) {
		t.Fatalf("both failing must surface the primary error (fail-safe quarantine); got %v", err)
	}
}

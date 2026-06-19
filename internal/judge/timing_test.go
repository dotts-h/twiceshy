// SPDX-License-Identifier: AGPL-3.0-only

package judge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPercentileMS_KnownSlice(t *testing.T) {
	durations := make([]time.Duration, 10)
	for i := range durations {
		durations[i] = time.Duration(10*(i+1)) * time.Millisecond
	}
	if got := percentileMS(durations, 0.50); got != 50 {
		t.Fatalf("p50: want 50, got %d", got)
	}
	if got := percentileMS(durations, 0.95); got != 100 {
		t.Fatalf("p95: want 100, got %d", got)
	}
}

func TestPercentileMS_Empty(t *testing.T) {
	if got := percentileMS(nil, 0.50); got != 0 {
		t.Fatalf("empty p50: want 0, got %d", got)
	}
}

// timingSequenceJudge returns primed verdicts/errors in order, one per call.
type timingSequenceJudge struct {
	verdicts []Verdict
	errs     []error
	calls    int
}

func (s *timingSequenceJudge) Judge(context.Context, Request) (Verdict, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return Verdict{}, s.errs[i]
	}
	return s.verdicts[i], nil
}

func timingApprove() Verdict { return ApproveVerdict("test-model") }
func timingReject() Verdict  { return Verdict{Decision: Reject} }

func TestTimingJudge_CountsApprovalsRejections(t *testing.T) {
	sj := &timingSequenceJudge{
		verdicts: []Verdict{timingApprove(), timingApprove(), timingReject(), timingApprove()},
	}
	tj := NewTiming(sj)
	ctx := context.Background()
	for range 4 {
		if _, err := tj.Judge(ctx, Request{}); err != nil {
			t.Fatalf("judge: %v", err)
		}
	}
	stats := tj.Stats()
	if stats.Calls != 4 {
		t.Fatalf("calls: want 4, got %d", stats.Calls)
	}
	if stats.Approvals != 3 {
		t.Fatalf("approvals: want 3, got %d", stats.Approvals)
	}
	if stats.Rejections != 1 {
		t.Fatalf("rejections: want 1, got %d", stats.Rejections)
	}
}

func TestTimingJudge_ErrorIncrementsNeither(t *testing.T) {
	sj := &timingSequenceJudge{
		verdicts: []Verdict{timingApprove(), {}, {}},
		errs:     []error{nil, errors.New("endpoint down"), nil},
	}
	tj := NewTiming(sj)
	ctx := context.Background()
	if _, err := tj.Judge(ctx, Request{}); err != nil {
		t.Fatalf("first judge: %v", err)
	}
	if _, err := tj.Judge(ctx, Request{}); err == nil {
		t.Fatal("second judge must return inner error")
	}
	if _, err := tj.Judge(ctx, Request{}); err != nil {
		t.Fatalf("third judge: %v", err)
	}
	stats := tj.Stats()
	if stats.Calls != 2 {
		t.Fatalf("calls: want 2 (error not counted), got %d", stats.Calls)
	}
	if stats.Approvals != 1 {
		t.Fatalf("approvals: want 1, got %d", stats.Approvals)
	}
	if stats.Rejections != 1 {
		t.Fatalf("rejections: want 1, got %d", stats.Rejections)
	}
}

func TestTimingJudge_ZeroCallsStats(t *testing.T) {
	tj := NewTiming(&timingSequenceJudge{})
	stats := tj.Stats()
	if stats.Calls != 0 || stats.Approvals != 0 || stats.Rejections != 0 ||
		stats.P50ms != 0 || stats.P95ms != 0 {
		t.Fatalf("zero-call stats must be zero-valued: %+v", stats)
	}
}

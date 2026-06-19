// SPDX-License-Identifier: AGPL-3.0-only

package judge

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"
)

// JudgeStats aggregates judge-call counts, verdict distribution, and latency
// percentiles for one promote/adapt run. Serialized in the run manifest and
// logged at run end so a degrading or compromised judge is visible in the daily
// audit without scraping per-record logs.
type JudgeStats struct {
	Calls      int   `json:"calls"`
	Approvals  int   `json:"approvals"`
	Rejections int   `json:"rejections"`
	P50ms      int64 `json:"p50_ms"`
	P95ms      int64 `json:"p95_ms"`
}

// TimingJudge wraps a Judge, recording per-call latency and approve/reject counts
// thread-safely. Inner errors are returned unchanged and do not affect stats.
type TimingJudge struct {
	inner Judge

	mu         sync.Mutex
	calls      int
	approvals  int
	rejections int
	durations  []time.Duration
}

// NewTiming wraps inner so each inner Judge invocation is timed and counted.
func NewTiming(inner Judge) *TimingJudge {
	return &TimingJudge{inner: inner}
}

// Judge calls the inner judge, records latency and verdict on success, and
// returns the inner verdict and error unchanged.
func (t *TimingJudge) Judge(ctx context.Context, req Request) (Verdict, error) {
	start := time.Now()
	v, err := t.inner.Judge(ctx, req)
	if err != nil {
		return v, err
	}
	elapsed := time.Since(start)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	t.durations = append(t.durations, elapsed)
	if v.Approved() {
		t.approvals++
	} else {
		t.rejections++
	}
	return v, nil
}

// Stats returns a snapshot of call counts and p50/p95 latency in milliseconds.
func (t *TimingJudge) Stats() JudgeStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	durations := append([]time.Duration(nil), t.durations...)
	return JudgeStats{
		Calls:      t.calls,
		Approvals:  t.approvals,
		Rejections: t.rejections,
		P50ms:      percentileMS(durations, 0.50),
		P95ms:      percentileMS(durations, 0.95),
	}
}

// percentileMS returns the nearest-rank p-percentile of durations in milliseconds.
// An empty slice yields 0.
func percentileMS(durations []time.Duration, p float64) int64 {
	if len(durations) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), durations...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	rank := int(math.Ceil(p * float64(len(cp))))
	if rank < 1 {
		rank = 1
	}
	return cp[rank-1].Milliseconds()
}

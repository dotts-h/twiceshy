// SPDX-License-Identifier: AGPL-3.0-only

package judge

import "context"

// FallbackJudge wraps a primary judge with a secondary that is consulted ONLY when
// the primary fails to produce a verdict — endpoint down, throttled/429, quota
// exhausted (any non-nil error). A primary REJECT is a real judgement, not a
// failure, so the secondary never sees it: falling back on a reject would
// double-spend the second model and let it override a legitimate decline, defeating
// the gate. If both fail, the primary's error is surfaced so the caller stays
// fail-safe (the record is not promoted).
//
// It keeps an off-pool primary (e.g. a rate-limited free-tier Gemini) as the default
// frontier judge while a paid/pooled secondary (e.g. Sonnet) covers quota
// exhaustion — off-pool on the happy path, no stall when the primary is throttled
// (ADR-0016 §6 follow-up, #0086).
type FallbackJudge struct {
	primary   Judge
	secondary Judge
}

// NewFallback builds a FallbackJudge. Both judges are required.
func NewFallback(primary, secondary Judge) FallbackJudge {
	return FallbackJudge{primary: primary, secondary: secondary}
}

// Judge tries the primary; on any error it consults the secondary. A verdict from
// either (approve or reject) is returned verbatim — only an error triggers the
// fallback. If the secondary also errors, the primary's error is returned.
func (f FallbackJudge) Judge(ctx context.Context, req Request) (Verdict, error) {
	v, err := f.primary.Judge(ctx, req)
	if err == nil {
		return v, nil
	}
	v2, err2 := f.secondary.Judge(ctx, req)
	if err2 != nil {
		return v, err // both failed: surface the primary error → fail-safe quarantine
	}
	return v2, nil
}

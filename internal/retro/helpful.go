// SPDX-License-Identifier: AGPL-3.0-only

package retro

import (
	"context"
	"fmt"
)

// CardVerdict is the usage judgement for one served/pushed card in a session
// transcript: whether the agent actually applied that card's lesson (#0069). It is
// the measurement half of #0065 — the trap-extraction half ships in analyzer.go; the
// authoritative attribution against the #0067 decision log and the precision/recall
// reporter are the tracked follow-up (#0069 acceptance 2 and 3).
type CardVerdict struct {
	// ID is the served record id the verdict is about (e.g. "exp-0149"), as it
	// appeared in the injected experience-data block of the transcript.
	ID string
	// Used reports whether the agent applied this card's lesson (true) or ignored it
	// (false). Only a Used verdict feeds the confirmed-helpful reinforcement signal.
	Used bool
}

// UsageJudge inspects a session transcript and returns, per served/pushed card, a
// used-vs-ignored verdict (#0069). Like the trap Analyzer it is an injectable,
// stubbed seam over the same off-pool analysis pass: the transcript is untrusted DATA
// and the model is prompt-injectable (ADR-0018). An error means the transcript could
// not be judged (e.g. the off-pool endpoint is down) — the caller records nothing and
// leaves it for retry, never treating the error as "all ignored".
type UsageJudge interface {
	JudgeUsage(ctx context.Context, transcript string) ([]CardVerdict, error)
}

// StubUsageJudge is a deterministic, network-free UsageJudge for tests.
type StubUsageJudge struct {
	Verdicts []CardVerdict
	Err      error
	Calls    int    // how many times JudgeUsage was called
	Last     string // the last transcript passed in
}

// JudgeUsage returns the primed verdicts (or error) and records the call.
func (s *StubUsageJudge) JudgeUsage(_ context.Context, transcript string) ([]CardVerdict, error) {
	s.Calls++
	s.Last = transcript
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Verdicts, nil
}

// ConfirmHelpfuler records the confirmed-helpful reinforcement signal for a record —
// the seam *index.Index satisfies (its ConfirmHelpful). Kept narrow so the
// helpfulness path depends only on the one method it needs; the signal lives off the
// hot path and never influences ranking (ADR-0013 §4).
type ConfirmHelpfuler interface {
	ConfirmHelpful(ctx context.Context, id string) error
}

// RecordHelpfulness folds a session's usage verdicts into the reinforcement signal:
// for each card the judge marked Used (with a non-empty id), it bumps confirmed_helpful
// via the recorder. Ignored verdicts are recorded nowhere — a served card that did not
// help is an absent positive, not counter-evidence. It returns how many confirmations
// were recorded; on the first recorder error it stops and returns that error with the
// count so far, so the caller can leave the transcript for retry rather than
// double-counting on a re-run.
func RecordHelpfulness(ctx context.Context, rec ConfirmHelpfuler, verdicts []CardVerdict) (int, error) {
	recorded := 0
	for _, v := range verdicts {
		if !v.Used || v.ID == "" {
			continue
		}
		if err := rec.ConfirmHelpful(ctx, v.ID); err != nil {
			return recorded, fmt.Errorf("retro: confirm helpful %s: %w", v.ID, err)
		}
		recorded++
	}
	return recorded, nil
}

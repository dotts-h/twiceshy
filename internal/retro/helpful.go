// SPDX-License-Identifier: AGPL-3.0-only

package retro

import (
	"context"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
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
// for each card the judge marked Used, it bumps confirmed_helpful via the recorder.
// Ignored verdicts are recorded nowhere — a served card that did not help is an absent
// positive, not counter-evidence. It returns how many confirmations were recorded; on
// the first recorder error it stops and returns that error with the count so far, so
// the caller can leave the transcript for retry rather than double-counting on a re-run.
//
// Trust boundary: the verdicts come from a prompt-injectable model judging an UNTRUSTED
// transcript, so an id is never written blindly. record.ValidID is the same format
// firewall the human confirm_helpful path applies before touching the usage table — a
// malformed/garbage/injection-shaped id (empty, whitespace, non-exp, path-shaped) is
// dropped. Within-session dedup keeps one Used card to one confirmation even if the
// judge repeats it. AUTHORITATIVE attribution — that the id was actually SERVED in this
// session, not merely well-formed — is the #0067 decision-log served-set cross-check,
// the deferred acceptance 2 of #0069 (stronger than a bare existence check).
func RecordHelpfulness(ctx context.Context, rec ConfirmHelpfuler, verdicts []CardVerdict) (int, error) {
	seen := make(map[string]bool)
	recorded := 0
	for _, v := range verdicts {
		if !v.Used || !record.ValidID(v.ID) || seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		if err := rec.ConfirmHelpful(ctx, v.ID); err != nil {
			return recorded, fmt.Errorf("retro: confirm helpful %s: %w", v.ID, err)
		}
		recorded++
	}
	return recorded, nil
}

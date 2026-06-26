// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/notify"
	"github.com/dotts-h/twiceshy/internal/promote"
)

func RunStage[T any](
	ctx context.Context,
	out io.Writer,
	getenv func(string) string,
	logger *slog.Logger,
	stage, runID string,
	asJSON, effect bool,
	judgeStats func() *judge.JudgeStats,
	walk func(io.Writer) (T, []promote.RecordAction, error),
	afterWalk func(T, []promote.RecordAction, error) error,
	manifest func(string, bool, T, *judge.JudgeStats, []promote.RecordAction) promote.RunManifest,
	summary func(io.Writer, T),
) error {
	proseOut := out
	if asJSON || effect {
		proseOut = io.Discard
	}
	st, actions, err := walk(proseOut)
	if effect {
		if err != nil && !errors.Is(err, ErrAnomalyHalt) {
			return err
		}
		PrintEffectPreview(out, stage, actions)
		return err
	}
	if afterWalk != nil {
		if werr := afterWalk(st, actions, err); werr != nil {
			return werr
		}
	}
	var stats *judge.JudgeStats
	if judgeStats != nil {
		stats = judgeStats()
	}
	if err != nil && !errors.Is(err, ErrAnomalyHalt) {
		return err
	}
	anomaly := errors.Is(err, ErrAnomalyHalt)
	if asJSON {
		if werr := manifest(runID, anomaly, st, stats, actions).WriteJSON(out); werr != nil {
			return werr
		}
		if err == nil {
			notify.Heartbeat(ctx, getenv("TWICESHY_HEARTBEAT_URL"), logger)
		}
		return err
	}
	summary(out, st)
	if err == nil {
		notify.Heartbeat(ctx, getenv("TWICESHY_HEARTBEAT_URL"), logger)
	}
	return err
}

func WriteDraftSummary(out io.Writer, st DraftStats) {
	_, _ = io.WriteString(out, draftSummary(st))
}

func draftSummary(st DraftStats) string {
	return fmt.Sprintf("draft: attached %d, rejected %d, unsupported %d, skipped %d (already proven)\n",
		st.Attached, st.Rejected, st.Unsupported, st.Skipped)
}

func PromoteManifest(runID string, anomaly bool, st PromoteStats, judgeStats *judge.JudgeStats, actions []promote.RecordAction) promote.RunManifest {
	return promote.RunManifest{
		RunID: runID, Stage: "promote", Anomaly: anomaly,
		Counts:     map[string]int{"promoted": st.Promoted, "held": st.Held, "ineligible": st.Ineligible},
		JudgeStats: judgeStats,
		Actions:    actions,
	}
}

func WritePromoteSummary(out io.Writer, st PromoteStats) {
	_, _ = fmt.Fprintf(out, "promote: promoted %d, held %d (attestation/judge declined), ineligible %d\n",
		st.Promoted, st.Held, st.Ineligible)
}

func AdaptManifest(runID string, anomaly bool, st AdaptStats, judgeStats *judge.JudgeStats, actions []promote.RecordAction) promote.RunManifest {
	return promote.RunManifest{
		RunID: runID, Stage: "adapt", Anomaly: anomaly,
		Counts:     map[string]int{"demoted": st.Demoted, "disputed": st.Disputed, "held": st.Held, "orphan": st.Orphan},
		JudgeStats: judgeStats,
		Actions:    actions,
	}
}

func WriteAdaptSummary(out io.Writer, st AdaptStats) {
	_, _ = fmt.Fprintf(out, "adapt: demoted %d, disputed %d, held %d, orphan %d\n", st.Demoted, st.Disputed, st.Held, st.Orphan)
}

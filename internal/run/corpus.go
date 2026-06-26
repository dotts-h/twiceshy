// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/notify"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// ErrAnomalyHalt marks a promote/adapt run that tripped the anomaly guardrail and
// halted before persisting further.
var ErrAnomalyHalt = errors.New("run halted: anomaly threshold exceeded")

// loopLogger returns logger, or one that discards everything when nil, so the
// promote/adapt core can log unconditionally while tests pass nil to stay quiet.
func loopLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// loopAlerter returns alerter, or a no-op when nil, so the promote/adapt core can
// fire guardrail alerts unconditionally while tests inject a recorder or nil.
func loopAlerter(alerter notify.Alerter) notify.Alerter {
	if alerter != nil {
		return alerter
	}
	return notify.NopAlerter{}
}

// recordPromoter is the seam the promote command drives: promote.Promoter.Promote
// satisfies it. Abstracting it lets the corpus walk + persistence be unit-tested
// without a broker or a live judge.
type recordPromoter interface {
	Promote(ctx context.Context, rec *record.Record) (promote.Outcome, error)
}

// PromoteStats summarizes a promote run.
type PromoteStats struct {
	Promoted   int // holding attestation + judge PASS → flipped to validated
	Held       int // eligible but not promoted (attestation didn't hold or judge declined)
	Ineligible int // not the execution-provable class (left for a human)
}

// PrintEffectPreview reports the would-be transitions of a no-persist run; a
// record whose status is unchanged (held/ineligible/orphan) is shown as no-op.
func PrintEffectPreview(out io.Writer, stage string, actions []promote.RecordAction) {
	changed := 0
	for _, a := range actions {
		if a.FromStatus != a.ToStatus {
			_, _ = fmt.Fprintf(out, "  %s: %s→%s (%s)\n", a.ID, a.FromStatus, a.ToStatus, a.Outcome)
			changed++
		}
	}
	_, _ = fmt.Fprintf(out, "%s -effect: %d of %d record(s) would change status — nothing written\n", stage, changed, len(actions))
}

func JournalPathForRun(corpus, stage string, effect bool) string {
	if effect {
		return ""
	}
	return promote.JournalPath(corpus, stage)
}

// corpusJournal incrementally persists promote/adapt decisions for resume (#0054).
type corpusJournal struct {
	j      *promote.Journal
	path   string
	resume map[string]bool
}

func startCorpusJournal(path, stage string) (*corpusJournal, []promote.RecordAction, error) {
	if path == "" {
		return nil, nil, nil
	}
	loaded, err := promote.LoadJournal(path)
	if err != nil {
		return nil, nil, err
	}
	if loaded != nil && !loaded.Complete {
		return &corpusJournal{j: loaded, path: path, resume: loaded.DoneIDs()},
			append([]promote.RecordAction(nil), loaded.Actions...), nil
	}
	return &corpusJournal{
		j:      &promote.Journal{Stage: stage, Actions: []promote.RecordAction{}},
		path:   path,
		resume: map[string]bool{},
	}, nil, nil
}

func (cj *corpusJournal) skip(id string) bool {
	return cj != nil && cj.resume[id]
}

func (cj *corpusJournal) record(action promote.RecordAction) {
	if cj == nil {
		return
	}
	cj.j.Actions = append(cj.j.Actions, action)
	_ = cj.j.Save(cj.path)
}

func (cj *corpusJournal) abort(recordID string, err error) {
	if cj == nil {
		return
	}
	cj.j.StoppedAt = &promote.JournalStop{RecordID: recordID, Error: err.Error()}
	cj.j.Complete = false
	_ = cj.j.Save(cj.path)
}

func (cj *corpusJournal) complete() {
	if cj == nil {
		return
	}
	cj.j.Complete = true
	cj.j.StoppedAt = nil
	_ = cj.j.Save(cj.path)
}

// PromoteCorpus is the testable core of `twiceshy promote`: it walks the records,
// runs each through the promoter, and persists the records that were promoted
// (the promoter mutated status/validated_at/provenance in place). run and persist
// are injected so the walk is exercised without a sandbox or a live judge. A hard
// promoter error (broker failure, an invalid promoted record) aborts; records
// promoted before it stay written (each is an independently-valid delta).
func PromoteCorpus(ctx context.Context, corpus string, recs []*record.Record, run recordPromoter, persist func(string, *record.Record) error, g guard.Guardrails, logger *slog.Logger, alerter notify.Alerter, out io.Writer, journalPath string) (PromoteStats, []promote.RecordAction, error) {
	log := loopLogger(logger).With("stage", "promote")
	alert := loopAlerter(alerter)
	start := time.Now()
	var st PromoteStats
	actions := []promote.RecordAction{}
	// Emergency stop (ADR-0013 §7): nothing auto-releases; records pile up.
	if g.Engaged() {
		_, _ = fmt.Fprintln(out, "promote: emergency stop engaged (TWICESHY_PAUSE) — no promotions")
		log.Warn("emergency stop engaged", "outcome", "emergency_stop")
		alert.Alert(ctx, "emergency_stop", "promote: emergency stop engaged (TWICESHY_PAUSE) — no promotions")
		log.Info("run complete", "outcome", "summary", "promoted", st.Promoted, "held", st.Held, "ineligible", st.Ineligible, "anomaly", false, "duration_ms", time.Since(start).Milliseconds())
		return st, actions, nil
	}
	journal, prior, err := startCorpusJournal(journalPath, "promote")
	if err != nil {
		return st, actions, fmt.Errorf("load promote journal: %w", err)
	}
	if prior != nil {
		actions = prior
	}
	budget := g.Budget()
	for _, rec := range recs {
		if journal.skip(rec.ID) {
			continue
		}
		// Throughput cap (clean stop, #0084): the intended per-run promotion
		// ceiling. A normal, mergeable batch — distinct from the anomaly halt
		// below. Placed FIRST and set below MaxActions so a full batch stops here
		// cleanly (zero exit, "re-run to continue") instead of tripping the anomaly
		// halt every run — the bug where MaxActions doubled as the throttle.
		if budget.Capped() {
			msg := fmt.Sprintf("promote: throughput cap reached (%d promotions) — stopping cleanly; re-run to continue", budget.Actions())
			_, _ = fmt.Fprintln(out, msg)
			log.Info("throughput cap reached", "outcome", "throughput_cap", "promotions", budget.Actions())
			break
		}
		// Anomaly HALT (ADR-0013 §D1): a promotion spike already past the alert
		// threshold is the "judge approving everything" signal. Check BEFORE doing
		// any further work — the old path persisted, then checked, then continued,
		// then exited 0, so a compromised judge wrote bad records and reported
		// success. Stop here; the post-loop summary flags it + a non-zero exit.
		if budget.Anomalous() {
			msg := fmt.Sprintf("promote: ANOMALY HALT — %d promotions exceed the alert threshold; stopping with nothing further written (investigate a compromised judge; TWICESHY_PAUSE=1)", budget.Actions())
			_, _ = fmt.Fprintln(out, msg)
			log.Warn("anomaly halt — stopping before further writes", "outcome", "anomaly_halt", "actions", budget.Actions())
			alert.Alert(ctx, "anomaly", msg)
			break
		}
		if ok, reason := promote.Promotable(rec); !ok {
			st.Ineligible++
			log.Info("decision", "record_id", rec.ID, "outcome", "ineligible", "reason", reason)
			action := promote.RecordAction{ID: rec.ID, Outcome: "ineligible", FromStatus: rec.Status, ToStatus: rec.Status, Reason: reason}
			actions = append(actions, action)
			journal.record(action)
			continue
		}
		// Budget cap: stop draining the sandbox past the per-run ceiling.
		if !budget.AllowRun() {
			msg := fmt.Sprintf("promote: budget cap reached (%d runs) — stopping; re-run to continue", budget.Runs())
			_, _ = fmt.Fprintln(out, msg)
			log.Warn("budget cap reached", "outcome", "budget_cap", "runs", budget.Runs())
			alert.Alert(ctx, "budget_cap", msg)
			break
		}
		budget.StartRun()
		from := rec.Status
		recStart := time.Now()
		outcome, err := run.Promote(ctx, rec)
		dur := time.Since(recStart).Milliseconds()
		if err != nil {
			log.Error("decision", "record_id", rec.ID, "outcome", "error", "reason", err.Error(), "duration_ms", dur)
			promoteErr := fmt.Errorf("promote %s: %w", rec.ID, err)
			journal.abort(rec.ID, promoteErr)
			return st, actions, promoteErr
		}
		if !outcome.Promoted {
			st.Held++
			_, _ = fmt.Fprintf(out, "  held %s (%s)\n", rec.ID, outcome.Reason)
			log.Info("decision", "record_id", rec.ID, "outcome", "held", "reason", outcome.Reason, "duration_ms", dur)
			action := promote.RecordAction{ID: rec.ID, Outcome: "held", FromStatus: from, ToStatus: rec.Status, Reason: outcome.Reason}
			actions = append(actions, action)
			journal.record(action)
			continue
		}
		if err := persist(corpus, rec); err != nil {
			log.Error("decision", "record_id", rec.ID, "outcome", "error", "reason", err.Error(), "duration_ms", dur)
			persistErr := fmt.Errorf("persist %s: %w", rec.ID, err)
			journal.abort(rec.ID, persistErr)
			return st, actions, persistErr
		}
		st.Promoted++
		budget.CountAction()
		advisory := rec.Provenance.Promotion != nil && len(rec.Provenance.Promotion.Panel) > 0
		if advisory {
			_, _ = fmt.Fprintf(out, "  promoted %s -> validated (advisory panel %s)\n",
				rec.ID, outcome.Verdict.Model)
		} else {
			_, _ = fmt.Fprintf(out, "  promoted %s -> validated (judge %s, reproduced under %s)\n",
				rec.ID, outcome.Verdict.Model, strings.Join(outcome.Attestation.ReproducedUnder, ", "))
		}
		log.Info("decision",
			"record_id", rec.ID,
			"outcome", "promoted",
			"judge_model", outcome.Verdict.Model,
			"judge_decision", string(outcome.Verdict.Decision),
			"reproduced_under", outcome.Attestation.ReproducedUnder,
			"attestation_ran_at", outcome.Attestation.RanAt,
			"advisory", advisory,
			"duration_ms", dur,
		)
		action := promote.RecordAction{
			ID: rec.ID, Outcome: "promoted", FromStatus: from, ToStatus: rec.Status,
			JudgeModel: outcome.Verdict.Model, JudgeDecision: string(outcome.Verdict.Decision),
			ReproducedUnder: outcome.Attestation.ReproducedUnder,
			Advisory:        advisory,
		}
		actions = append(actions, action)
		journal.record(action)
	}
	anomaly := budget.Anomalous()
	// Approval-RATE anomaly (#0085): the count anomaly above is moot once a throughput
	// cap is set (a normal run stops at the cap). A compromised judge approving
	// ~everything instead shows as a high promoted/judged fraction, which survives the
	// cap — assess it post-loop on the full sample and fold it into the halt/alert.
	if budget.RateAnomalous() {
		anomaly = true
		msg := fmt.Sprintf("promote: APPROVAL-RATE ANOMALY — %d/%d promoted (%.0f%%) over the %.0f%% baseline (min sample %d); a batch approving ~everything signals a compromised judge even under a throughput cap (investigate; TWICESHY_PAUSE=1)",
			budget.Actions(), budget.Runs(), 100*budget.ActionRate(), 100*g.MaxActionRate, g.MinSample)
		_, _ = fmt.Fprintln(out, msg)
		log.Warn("approval-rate anomaly", "outcome", "rate_anomaly", "promoted", budget.Actions(), "judged", budget.Runs(), "rate", budget.ActionRate())
		alert.Alert(ctx, "rate_anomaly", msg)
	}
	log.Info("run complete", "outcome", "summary", "promoted", st.Promoted, "held", st.Held, "ineligible", st.Ineligible, "anomaly", anomaly, "duration_ms", time.Since(start).Milliseconds())
	// Marking complete here (including an anomaly halt or a budget-cap break) is
	// deliberate: only a hard mid-record error is a resumable abort (it set
	// StoppedAt). An anomaly halt is held for human review and a budget cap means
	// "re-run to continue" with a fresh walk — neither should auto-resume.
	journal.complete()
	if anomaly {
		return st, actions, ErrAnomalyHalt
	}
	return st, actions, nil
}

// counterRunner re-runs an original record's repro and the report's evidence as
// a counter-repro, returning both attestations. The broker-backed impl needs
// docker+runsc; a fake drives the AdaptCorpus walk in tests.
type counterRunner interface {
	Run(ctx context.Context, original, report *record.Record) (promote.CounterEvidence, error)
}

// AdaptStats summarizes an adapt run.
type AdaptStats struct {
	Demoted  int // reproduced failure + judge PASS → validated→stale
	Disputed int // non-reproducing reports corroborated past threshold → validated→disputed
	Held     int // no execution-backed counter-evidence and uncorroborated — no change
	Orphan   int // report disputes a record not in the corpus
}

// ReportDisputes returns the disputed record id if rec is a quarantined outcome
// report (carries provenance.disputes), else "".
func ReportDisputes(rec *record.Record) string {
	if rec.Status == "quarantined" && rec.Provenance.Disputes != nil {
		return *rec.Provenance.Disputes
	}
	return ""
}

// AdaptCorpus is the testable core of `twiceshy adapt`: it pairs each outcome
// report with the record it disputes, runs the counter-evidence through `run`,
// adjudicates it with the Adapter, and persists the disputed record when it is
// demoted or disputed. The corroboration count (other reports disputing the same
// record) is computed from the corpus. run and persist are injected so the walk
// is exercised without a sandbox or a live judge.
func AdaptCorpus(ctx context.Context, corpus string, recs []*record.Record, run counterRunner, adapter *promote.Adapter, persist func(string, *record.Record) error, g guard.Guardrails, logger *slog.Logger, alerter notify.Alerter, out io.Writer, journalPath string) (AdaptStats, []promote.RecordAction, error) {
	log := loopLogger(logger).With("stage", "adapt")
	alert := loopAlerter(alerter)
	start := time.Now()
	byID := make(map[string]*record.Record, len(recs))
	disputesCount := make(map[string]int)
	for _, r := range recs {
		byID[r.ID] = r
		if d := ReportDisputes(r); d != "" {
			disputesCount[d]++
		}
	}

	var st AdaptStats
	actions := []promote.RecordAction{}
	// Emergency stop (ADR-0013 §7) halts auto-demotion too.
	if g.Engaged() {
		_, _ = fmt.Fprintln(out, "adapt: emergency stop engaged (TWICESHY_PAUSE) — no demotions")
		log.Warn("emergency stop engaged", "outcome", "emergency_stop")
		alert.Alert(ctx, "emergency_stop", "adapt: emergency stop engaged (TWICESHY_PAUSE) — no demotions")
		log.Info("run complete", "outcome", "summary", "demoted", st.Demoted, "disputed", st.Disputed, "held", st.Held, "orphan", st.Orphan, "anomaly", false, "duration_ms", time.Since(start).Milliseconds())
		return st, actions, nil
	}
	journal, prior, err := startCorpusJournal(journalPath, "adapt")
	if err != nil {
		return st, actions, fmt.Errorf("load adapt journal: %w", err)
	}
	if prior != nil {
		actions = prior
	}
	budget := g.Budget()
	for _, rep := range recs {
		// Throughput cap (clean stop, #0084): the intended per-run demote/dispute
		// ceiling. A normal, mergeable batch — set below MaxActions so a full run
		// stops here cleanly instead of tripping the anomaly halt every time.
		if budget.Capped() {
			msg := fmt.Sprintf("adapt: throughput cap reached (%d actions) — stopping cleanly; re-run to continue", budget.Actions())
			_, _ = fmt.Fprintln(out, msg)
			log.Info("throughput cap reached", "outcome", "throughput_cap", "actions", budget.Actions())
			break
		}
		// Anomaly HALT (ADR-0013 §D1): a demote/dispute spike past the alert
		// threshold is the "compromised judge" signal — stop BEFORE persisting any
		// more (the old path persisted, then checked, then continued, then exited 0).
		// The post-loop summary flags it + a non-zero exit.
		if budget.Anomalous() {
			msg := fmt.Sprintf("adapt: ANOMALY HALT — %d demote/dispute actions exceed the alert threshold; stopping with nothing further written (investigate a compromised judge; TWICESHY_PAUSE=1)", budget.Actions())
			_, _ = fmt.Fprintln(out, msg)
			log.Warn("anomaly halt — stopping before further writes", "outcome", "anomaly_halt", "actions", budget.Actions())
			alert.Alert(ctx, "anomaly", msg)
			break
		}
		origID := ReportDisputes(rep)
		if origID == "" {
			continue
		}
		original, ok := byID[origID]
		skipID := origID
		if !ok {
			skipID = rep.ID
		}
		if journal.skip(skipID) {
			continue
		}
		if !ok {
			st.Orphan++
			_, _ = fmt.Fprintf(out, "  orphan report %s disputes unknown %s\n", rep.ID, origID)
			log.Info("decision", "record_id", rep.ID, "outcome", "orphan", "reason", "disputes unknown "+origID)
			action := promote.RecordAction{ID: rep.ID, Outcome: "orphan", FromStatus: rep.Status, ToStatus: rep.Status, Reason: "disputes unknown " + origID}
			actions = append(actions, action)
			journal.record(action)
			continue
		}
		// Budget cap: a report flood can't drain the broker/judge past the ceiling.
		if !budget.AllowRun() {
			msg := fmt.Sprintf("adapt: budget cap reached (%d runs) — stopping; re-run to continue", budget.Runs())
			_, _ = fmt.Fprintln(out, msg)
			log.Warn("budget cap reached", "outcome", "budget_cap", "runs", budget.Runs())
			alert.Alert(ctx, "budget_cap", msg)
			break
		}
		budget.StartRun()
		from := original.Status
		recStart := time.Now()
		ev, err := run.Run(ctx, original, rep)
		if err != nil {
			log.Error("decision", "record_id", original.ID, "outcome", "error", "reason", err.Error(), "duration_ms", time.Since(recStart).Milliseconds())
			adaptErr := fmt.Errorf("adapt %s: %w", rep.ID, err)
			journal.abort(rep.ID, adaptErr)
			return st, actions, adaptErr
		}
		outcome, err := adapter.Adapt(ctx, original, rep, ev, disputesCount[origID]-1)
		dur := time.Since(recStart).Milliseconds()
		if err != nil {
			log.Error("decision", "record_id", original.ID, "outcome", "error", "reason", err.Error(), "duration_ms", dur)
			adaptErr := fmt.Errorf("adapt %s: %w", rep.ID, err)
			journal.abort(rep.ID, adaptErr)
			return st, actions, adaptErr
		}
		switch outcome.Action {
		case promote.ActionDemote:
			if err := persist(corpus, original); err != nil {
				log.Error("decision", "record_id", original.ID, "outcome", "error", "reason", err.Error(), "duration_ms", dur)
				persistErr := fmt.Errorf("persist %s: %w", original.ID, err)
				journal.abort(original.ID, persistErr)
				return st, actions, persistErr
			}
			st.Demoted++
			budget.CountAction()
			_, _ = fmt.Fprintf(out, "  demoted %s -> stale (report %s, judge %s)\n", original.ID, rep.ID, outcome.Verdict.Model)
			log.Info("decision", "record_id", original.ID, "outcome", "demoted", "report_id", rep.ID,
				"judge_model", outcome.Verdict.Model, "judge_decision", string(outcome.Verdict.Decision), "duration_ms", dur)
			action := promote.RecordAction{ID: original.ID, Outcome: "demoted", FromStatus: from, ToStatus: original.Status,
				JudgeModel: outcome.Verdict.Model, JudgeDecision: string(outcome.Verdict.Decision)}
			actions = append(actions, action)
			journal.record(action)
		case promote.ActionDispute:
			if err := persist(corpus, original); err != nil {
				log.Error("decision", "record_id", original.ID, "outcome", "error", "reason", err.Error(), "duration_ms", dur)
				persistErr := fmt.Errorf("persist %s: %w", original.ID, err)
				journal.abort(original.ID, persistErr)
				return st, actions, persistErr
			}
			st.Disputed++
			budget.CountAction()
			_, _ = fmt.Fprintf(out, "  disputed %s (corroborated by %d reports)\n", original.ID, disputesCount[origID])
			log.Info("decision", "record_id", original.ID, "outcome", "disputed", "report_id", rep.ID,
				"reason", outcome.Reason, "corroborating", disputesCount[origID], "duration_ms", dur)
			action := promote.RecordAction{ID: original.ID, Outcome: "disputed", FromStatus: from, ToStatus: original.Status, Reason: outcome.Reason}
			actions = append(actions, action)
			journal.record(action)
		default:
			st.Held++
			log.Info("decision", "record_id", original.ID, "outcome", "held", "report_id", rep.ID, "reason", outcome.Reason, "duration_ms", dur)
			action := promote.RecordAction{ID: original.ID, Outcome: "held", FromStatus: from, ToStatus: original.Status, Reason: outcome.Reason}
			actions = append(actions, action)
			journal.record(action)
		}
	}
	anomaly := budget.Anomalous()
	// Action-RATE anomaly (#0085): like promote, the count anomaly is moot under a
	// throughput cap; a judge demoting/disputing ~everything instead shows as a high
	// action/judged fraction that survives the cap. Assess post-loop on the full
	// sample and fold into the halt/alert.
	if budget.RateAnomalous() {
		anomaly = true
		msg := fmt.Sprintf("adapt: ACTION-RATE ANOMALY — %d/%d demote/dispute actions (%.0f%%) over the %.0f%% baseline (min sample %d); a judge demoting ~everything signals compromise even under a throughput cap (investigate; TWICESHY_PAUSE=1)",
			budget.Actions(), budget.Runs(), 100*budget.ActionRate(), 100*g.MaxActionRate, g.MinSample)
		_, _ = fmt.Fprintln(out, msg)
		log.Warn("action-rate anomaly", "outcome", "rate_anomaly", "actions", budget.Actions(), "judged", budget.Runs(), "rate", budget.ActionRate())
		alert.Alert(ctx, "rate_anomaly", msg)
	}
	log.Info("run complete", "outcome", "summary", "demoted", st.Demoted, "disputed", st.Disputed, "held", st.Held, "orphan", st.Orphan, "anomaly", anomaly, "duration_ms", time.Since(start).Milliseconds())
	// Complete (incl. anomaly halt / budget-cap break) is deliberate: only a hard
	// mid-record error is a resumable abort (StoppedAt set). See PromoteCorpus.
	journal.complete()
	if anomaly {
		return st, actions, ErrAnomalyHalt
	}
	return st, actions, nil
}

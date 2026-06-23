// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// defaultHoldCooldown is how long a HELD (judged-and-declined) record waits before
// the promote walk will re-run the costly panel on it again (#0084). Without it a
// declined record is re-judged every scheduled run (~every 30 min) forever, so the
// held backlog re-judges itself and each run grows slower. 0 disables the cooldown.
const defaultHoldCooldown = 7 * 24 * time.Hour

// holdLedgerName is the ledger file under <corpus>/runs/ — a sibling of the run
// journals. It is operational state on the -corpus path, NOT an experience record,
// so the corpus's data-only record validation ignores it and engine CI (frozen
// fixtures, #0079) never loads it. Same seam (#0081) the journals already ride.
const holdLedgerName = "promote.holds.json"

// holdLedger maps a record id to the time it was last held, so a recently-declined
// record can be skipped instead of re-judged. A nil *holdLedger means the cooldown
// is disabled and every method is a no-op (records are always eligible).
type holdLedger struct {
	path     string
	cooldown time.Duration
	entries  map[string]time.Time
}

// loadHoldLedger reads the ledger under <corpus>/runs/. cooldown <= 0 disables the
// cooldown (returns nil). A missing or corrupt ledger starts empty — fail-open: the
// worst case is a record judged one extra time, never a stuck pipeline.
func loadHoldLedger(corpus string, cooldown time.Duration) *holdLedger {
	if cooldown <= 0 {
		return nil
	}
	l := &holdLedger{
		path:     filepath.Join(corpus, "runs", holdLedgerName),
		cooldown: cooldown,
		entries:  map[string]time.Time{},
	}
	if b, err := os.ReadFile(l.path); err == nil {
		var raw map[string]time.Time
		if json.Unmarshal(b, &raw) == nil && raw != nil {
			l.entries = raw
		}
	}
	return l
}

// inCooldown reports whether id was held within the cooldown window ending at now —
// i.e. it should be skipped this run rather than re-judged.
func (l *holdLedger) inCooldown(id string, now time.Time) bool {
	if l == nil {
		return false
	}
	held, ok := l.entries[id]
	return ok && now.Sub(held) < l.cooldown
}

// note folds one run outcome into the ledger: a held record starts/refreshes its
// cooldown; any other resolution (promoted, demoted away from quarantine) clears it
// so a future re-quarantine is judged promptly.
func (l *holdLedger) note(id string, held bool, now time.Time) {
	if l == nil {
		return
	}
	if held {
		l.entries[id] = now
	} else {
		delete(l.entries, id)
	}
}

// save prunes entries whose cooldown has lapsed (they are eligible again, so they
// need no record) and writes the ledger to <corpus>/runs/ via a temp-file + rename
// so a crash mid-write can never leave a half-written (corrupt) ledger — the reader
// fails open on a corrupt file, but atomicity avoids needing to. A write error is
// returned for the caller to log; the only consequence is less cooldown next run.
func (l *holdLedger) save(now time.Time) error {
	if l == nil {
		return nil
	}
	for id, held := range l.entries {
		if now.Sub(held) >= l.cooldown {
			delete(l.entries, id)
		}
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".promote.holds.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed; cleans up on any error path
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, l.path)
}

// noteOutcomes folds a promote run's actions into the ledger: held records start
// their cooldown, promoted records are cleared. Outcomes other than held/promoted
// (ineligible) leave the ledger untouched — they were never judged.
func noteOutcomes(l *holdLedger, actions []promote.RecordAction, now time.Time) {
	for _, a := range actions {
		switch a.Outcome {
		case "held":
			l.note(a.ID, true, now)
		case "promoted":
			l.note(a.ID, false, now)
		}
	}
}

// filterCooldown drops the records that are still in their hold cooldown so the
// promote walk never reaches them — the cheap pre-filter that stops the held
// backlog re-judging itself. A nil ledger keeps every record.
func filterCooldown(recs []*record.Record, l *holdLedger, now time.Time) (kept []*record.Record, skipped int) {
	if l == nil {
		return recs, 0
	}
	kept = make([]*record.Record, 0, len(recs))
	for _, r := range recs {
		if l.inCooldown(r.ID, now) {
			skipped++
			continue
		}
		kept = append(kept, r)
	}
	return kept, skipped
}

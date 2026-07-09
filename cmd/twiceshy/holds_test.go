// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestNoteOutcomes_HeldStartsPromotedClears(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	l := loadHoldLedger(t.TempDir(), 7*24*time.Hour)
	l.note("exp-0700", true, now.Add(-time.Hour)) // previously held

	noteOutcomes(l, []promote.RecordAction{
		{ID: "exp-0700", Outcome: "promoted"},   // now promoted -> clear
		{ID: "exp-0701", Outcome: "held"},       // newly held -> start cooldown
		{ID: "exp-0702", Outcome: "ineligible"}, // untouched
		{ID: "exp-0703", Outcome: "deferred"},   // untouched (#0123)
	}, now)

	if l.inCooldown("exp-0700", now) {
		t.Fatal("a promoted record must be cleared from cooldown")
	}
	if !l.inCooldown("exp-0701", now) {
		t.Fatal("a newly held record must enter cooldown")
	}
	if l.inCooldown("exp-0702", now) {
		t.Fatal("an ineligible record must not be added to the ledger")
	}
	if l.inCooldown("exp-0703", now) {
		t.Fatal("a deferred record must not be added to the ledger")
	}
}

func TestHoldLedger_DisabledIsNoop(t *testing.T) {
	if l := loadHoldLedger(t.TempDir(), 0); l != nil {
		t.Fatal("cooldown <= 0 must disable the ledger (nil)")
	}
	var nilLedger *holdLedger
	if nilLedger.inCooldown("exp-0001", time.Now()) {
		t.Fatal("a nil ledger must never report cooldown")
	}
	nilLedger.note("exp-0001", true, time.Now()) // must not panic
	if err := nilLedger.save(time.Now()); err != nil {
		t.Fatalf("nil ledger save: %v", err)
	}
}

func TestHoldLedger_HeldRecordInCooldownThenExpires(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	l := loadHoldLedger(t.TempDir(), 7*24*time.Hour)
	l.note("exp-0565", true, now)

	if !l.inCooldown("exp-0565", now.Add(24*time.Hour)) {
		t.Fatal("a record held 1 day ago must be in a 7-day cooldown")
	}
	if l.inCooldown("exp-0565", now.Add(8*24*time.Hour)) {
		t.Fatal("after the window elapses the record must be eligible again")
	}
	if l.inCooldown("exp-9999", now) {
		t.Fatal("a never-held record must not be in cooldown")
	}
}

func TestHoldLedger_PromoteClearsCooldown(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	l := loadHoldLedger(t.TempDir(), 7*24*time.Hour)
	l.note("exp-0565", true, now)
	l.note("exp-0565", false, now.Add(time.Hour)) // promoted/resolved
	if l.inCooldown("exp-0565", now.Add(2*time.Hour)) {
		t.Fatal("a resolved record must be cleared from cooldown")
	}
}

func TestHoldLedger_PersistsAndPrunesExpired(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	l := loadHoldLedger(dir, 7*24*time.Hour)
	l.note("fresh", true, now)
	l.note("stale", true, now.Add(-8*24*time.Hour)) // already past the window
	if err := l.save(now); err != nil {
		t.Fatalf("save: %v", err)
	}

	path := filepath.Join(dir, "runs", holdLedgerName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ledger not written under runs/: %v", err)
	}

	// Reload: the stale entry was pruned on save; the fresh one survives.
	l2 := loadHoldLedger(dir, 7*24*time.Hour)
	if !l2.inCooldown("fresh", now) {
		t.Fatal("fresh entry must survive a save/reload round-trip")
	}
	if l2.inCooldown("stale", now) {
		t.Fatal("an expired entry must be pruned on save")
	}
}

func TestFilterCooldown_DropsHeldKeepsRest(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	l := loadHoldLedger(t.TempDir(), 7*24*time.Hour)
	l.note("exp-0002", true, now) // recently held -> should be filtered

	recs := []*record.Record{{ID: "exp-0001"}, {ID: "exp-0002"}, {ID: "exp-0003"}}
	kept, skipped := filterCooldown(recs, l, now.Add(time.Hour))
	if skipped != 1 || len(kept) != 2 {
		t.Fatalf("want 1 skipped / 2 kept; got skipped=%d kept=%d", skipped, len(kept))
	}
	for _, r := range kept {
		if r.ID == "exp-0002" {
			t.Fatal("the in-cooldown record must be filtered out of the walk")
		}
	}

	// A nil ledger keeps everything.
	all, n := filterCooldown(recs, nil, now)
	if n != 0 || len(all) != 3 {
		t.Fatalf("nil ledger must keep all records; got skipped=%d kept=%d", n, len(all))
	}
}

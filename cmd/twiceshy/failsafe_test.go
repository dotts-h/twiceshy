// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// outageCounterRunner simulates a broker/docker outage during the counter-repro.
type outageCounterRunner struct{ err error }

func (o outageCounterRunner) Run(context.Context, *record.Record, *record.Record) (promote.CounterEvidence, error) {
	return promote.CounterEvidence{}, o.err
}

// Fail-safe verification (#0053, §D3): the two under-covered failure modes —
// a broker/docker outage mid-run, and a poison/unparseable record — must fail
// safe rather than promote something bad or kill the whole run.

// (a) A broker outage surfaces as a promoter error. The run must abort, exit
// NON-ZERO, persist nothing for the failing record, and keep prior valid deltas.
func TestFailsafe_BrokerOutageExitsNonZeroNothingBadPromoted(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{
		promote: map[string]bool{"exp-0100": true},
		// exp-0101's attestation hits a dead docker/runsc substrate.
		err: map[string]error{"exp-0101": errors.New("broker: cannot connect to docker daemon")},
	}
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }

	_, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err == nil {
		t.Fatal("a broker outage must abort the run")
	}
	if code := exitCode(err); code == 0 {
		t.Fatalf("a broker-outage abort must exit non-zero; got exit code %d", code)
	}
	// Nothing bad promoted: the failing record is never persisted; only the
	// record that fully passed before it stays written.
	if strings.Join(persisted, ",") != "exp-0100" {
		t.Fatalf("persisted = %v, want only the pre-outage record [exp-0100]", persisted)
	}
}

// (a, adapt side) The same outage during adapt must abort non-zero and persist
// nothing — a failed counter-repro must never silently demote a good record.
func TestFailsafe_AdaptBrokerOutageExitsNonZeroNoDemote(t *testing.T) {
	orig := validatedRec("exp-0043")         // adapt_test.go (same package)
	rep := reportRec("exp-0200", "exp-0043") // adapt_test.go
	recs := []*record.Record{orig, rep}
	runner := outageCounterRunner{err: errors.New("broker: cannot connect to docker daemon")}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	var persisted []string
	persist := func(_ string, r *record.Record) error { persisted = append(persisted, r.ID); return nil }

	_, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err == nil {
		t.Fatal("a broker outage during adapt must abort the run")
	}
	if code := exitCode(err); code == 0 {
		t.Fatalf("an adapt broker-outage abort must exit non-zero; got %d", code)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted = %v, want nothing — a failed counter-repro must not demote", persisted)
	}
	if orig.Status != "validated" {
		t.Fatalf("original status = %q, want unchanged 'validated'", orig.Status)
	}
}

// writeValidRecord writes a known-good quarantined record (mirrors the shape the
// record package's own fixtures use) into corpusRoot.
func writeValidRecord(t *testing.T, corpusRoot string) {
	t.Helper()
	rec := &record.Record{
		SchemaVersion: 1, ID: "exp-9100", Kind: "convention", Status: "quarantined",
		Title:     "A valid placeholder record for the resilient-load test",
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go"}},
		Provenance: record.Provenance{
			Source:     record.Source{Author: "twiceshy-test"},
			RecordedAt: "2026-06-18",
			Valid:      record.Validity{From: "2026-06-18"},
		},
		Body: "Distilled fact, authored in twiceshy's own words.",
		Path: "experience/2026/9100-valid.md",
	}
	writeFixture(t, corpusRoot, rec) // main_test.go (same package)
}

// (b) A poison/unparseable record must NOT kill the whole run — the run loads
// the survivors, reports the skip, and proceeds.
func TestFailsafe_PoisonRecordDoesNotKillRun(t *testing.T) {
	dir := t.TempDir()
	writeValidRecord(t, dir)
	poison := filepath.Join(dir, "experience", "2026", "9999-poison.md")
	if err := os.WriteFile(poison, []byte("not a record {{{ broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", dir, "-dry-run"}, &buf,
		func(string) string { return "" })
	if err != nil {
		t.Fatalf("a poison record must not kill the run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "skipped unparseable record") || !strings.Contains(out, "9999-poison.md") {
		t.Errorf("the skipped poison record should be reported; got %q", out)
	}
	// The run walked to completion despite the poison (the survivor loaded; the
	// dry-run summary is printed rather than aborting at load).
	if !strings.Contains(out, "promote (dry-run):") {
		t.Errorf("the run should complete past the poison record; got %q", out)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// mkRecordKindOrigin is mkRecord with kind and provenance.source.author made
// variable, for the push-eligibility gate (#0107): a trap/fix carries the
// resolution+guard an episodic kind requires; a convention needs neither.
func mkRecordKindOrigin(t *testing.T, num int, kind, author, title, summary string) *record.Record {
	t.Helper()
	var body string
	switch kind {
	case "trap", "fix":
		body = fmt.Sprintf(`symptom:
  summary: %q
resolution:
  root_cause: "a cause"
  fix: "a fix"
guard: { repro: null, guarding_test: "TestSomething" }
`, summary)
	default:
		body = ""
	}
	src := fmt.Sprintf(`---
schema_version: 1
id: exp-%04d
kind: %s
status: validated
title: %q
%sprovenance:
  source: { author: %q, session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
---

Narrative for %s.
`, num, kind, title, body, author, title)
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%04d-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("fixture record invalid: %v", err)
	}
	return rec
}

// TestPushEligibilityExcludesImporterOriginAndIneligibleKind is #0107's
// acceptance case: an importer-origin trap and a convention-kind record are
// never served by push even on an exact topical match, while both stay
// reachable via pull (Retrieve) — push eligibility narrows the push channel
// only, it does not quarantine the record.
func TestPushEligibilityExcludesImporterOriginAndIneligibleKind(t *testing.T) {
	ctx := context.Background()
	importerTrap := mkRecordKindOrigin(t, 500, "trap", "twiceshy-importer",
		"Zorbnaxos dependency advisory", "the zorbnaxos package has a known cve zorbnaxos zorbnaxos")
	convention := mkRecordKindOrigin(t, 501, "convention", "horia",
		"Use quixotic naming for test helpers", "quixotic naming keeps test helpers consistent quixotic quixotic")
	ix := openIndex(t, []*record.Record{importerTrap, convention})

	for _, q := range []string{"zorbnaxos", "quixotic"} {
		// ErrorTrigger relaxes corroboration to one token, isolating the
		// eligibility gate as the thing under test here.
		dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: q, ErrorTrigger: true})
		if err != nil {
			t.Fatalf("RetrievePushTraced(%q): %v", q, err)
		}
		if len(dec.Discriminative) != 0 || len(dec.Served) != 0 {
			t.Errorf("push(%q) = %+v, want a closed gate (ineligible origin/kind)", q, dec)
		}
	}

	for _, tc := range []struct{ q, id string }{
		{"zorbnaxos", "exp-0500"}, {"quixotic", "exp-0501"},
	} {
		hits, err := ix.Retrieve(ctx, index.Query{Text: tc.q, Floor: index.FloorOff})
		if err != nil {
			t.Fatalf("Retrieve(%q): %v", tc.q, err)
		}
		found := false
		for _, h := range hits {
			if h.ID == tc.id {
				found = true
			}
		}
		if !found {
			t.Errorf("pull Retrieve(%q) = %v, want %s reachable (push eligibility must not affect pull)", tc.q, hits, tc.id)
		}
	}
}

// TestPushEligibilityExcludesAlphaOriginEvenValidated is #0128's defense-in-depth
// acceptance case (ADR-0030 phase 2): a VALIDATED trap/fix record whose origin
// is "alpha:<token_id>" (an untrusted alpha tenant's contribution) must never
// be served on the push channel, however discriminative its terms are — the
// low-trust tier stays excluded even after promotion, over and above the
// ordinary quarantine floor. It stays reachable via pull (agent-initiated,
// k<=3, floor), same as an importer-origin record.
func TestPushEligibilityExcludesAlphaOriginEvenValidated(t *testing.T) {
	ctx := context.Background()
	alphaTrap := mkRecordKindOrigin(t, 502, "trap", "alpha:tok_deadbeef",
		"Blorptastic queue starvation under load", "the blorptastic worker queue starves blorptastic blorptastic under load")
	ix := openIndex(t, []*record.Record{alphaTrap})

	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: "blorptastic", ErrorTrigger: true})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if len(dec.Discriminative) != 0 || len(dec.Served) != 0 {
		t.Errorf("push(blorptastic) = %+v, want a closed gate (alpha origin, even validated)", dec)
	}

	hits, err := ix.Retrieve(ctx, index.Query{Text: "blorptastic", Floor: index.FloorOff})
	if err != nil {
		t.Fatalf("Retrieve(blorptastic): %v", err)
	}
	found := false
	for _, h := range hits {
		if h.ID == "exp-0502" {
			found = true
		}
	}
	if !found {
		t.Errorf("pull Retrieve(blorptastic) = %v, want exp-0502 reachable (push eligibility must not affect pull)", hits)
	}
}

// pushCorroborationFixture builds four eligible (trap, non-importer) records:
// a single-token record (frobnicator), two records that each carry ONE of a
// pair of tokens (zeeble / quonk — no record has both, the specimen class),
// and one record carrying BOTH of a different pair (wibblesnout +
// florptastic). Filler records give the corpus enough scale for a realistic
// BM25 magnitude (mirrors TestRetrievePushBypassServesOnlyFingerprintHits).
func pushCorroborationFixture(t *testing.T) *index.Index {
	t.Helper()
	var recs []*record.Record
	recs = append(recs, mkRecord(t, 600, "Frobnicator overload during batch processing",
		"the frobnicator subsystem frobnicator frobnicator overloads under batch load", nil, "Go", "frob"))
	recs = append(recs, mkRecord(t, 601, "Zeeble handling failure in the pipeline",
		"zeeble zeeble records fail to process during the pipeline run", nil, "Go", "zeeble"))
	recs = append(recs, mkRecord(t, 602, "Quonk retries exhausted on the worker",
		"quonk quonk retries are exhausted when the worker restarts", nil, "Go", "quonk"))
	recs = append(recs, mkRecord(t, 603, "Wibblesnout florptastic joint failure",
		"the wibblesnout florptastic joint fails wibblesnout florptastic under load", nil, "Go", "wib"))
	for i := 0; i < 10; i++ {
		recs = append(recs, mkRecord(t, 610+i, "unrelated filler", "cache eviction retry budget notes", nil, "Go", "frob"))
	}
	return openIndex(t, recs)
}

// TestRetrievePushSingleTokenGateClosedForPromptOpenForError is #0108's core
// contract: a single discriminative token never opens the gate for a
// prompt-triggered query, but the same query with ErrorTrigger set (a verbatim
// error line, #0087) keeps the pre-#0108 single-token behavior.
func TestRetrievePushSingleTokenGateClosedForPromptOpenForError(t *testing.T) {
	ctx := context.Background()
	ix := pushCorroborationFixture(t)

	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: "frobnicator"})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if len(dec.Served) != 0 {
		t.Errorf("prompt-triggered single-token query served %v, want 0 (gate closed)", dec.Served)
	}
	if len(dec.Discriminative) != 1 || dec.Discriminative[0] != "frobnicator" {
		t.Errorf("Discriminative = %v, want [frobnicator] recorded for telemetry even though gate closed", dec.Discriminative)
	}

	dec, err = ix.RetrievePushTraced(ctx, index.Query{Text: "frobnicator", ErrorTrigger: true})
	if err != nil {
		t.Fatalf("RetrievePushTraced (error trigger): %v", err)
	}
	if len(dec.Served) != 1 || dec.Served[0].ID != "exp-0600" {
		t.Errorf("error-triggered single-token query = %v, want exp-0600 served", dec.Served)
	}
}

// TestRetrievePushCorroborationRejectsTokensAcrossDifferentRecords is the
// specimen regression (#0106): two discriminative tokens that each live in a
// DIFFERENT eligible record must serve NEITHER — the OR-joined disc-subset
// search matches each record on its own token, but corroboration requires two
// DISTINCT tokens on the SAME record.
func TestRetrievePushCorroborationRejectsTokensAcrossDifferentRecords(t *testing.T) {
	ctx := context.Background()
	ix := pushCorroborationFixture(t)

	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: "zeeble quonk"})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if len(dec.Served) != 0 {
		t.Errorf("cross-record disc tokens served %v, want 0 (neither record carries both)", dec.Served)
	}
	if len(dec.Discriminative) != 2 {
		t.Errorf("Discriminative = %v, want both gate-opening tokens recorded", dec.Discriminative)
	}
}

// TestRetrievePushSpecimenPromptServesNothing pins the literal reproduced
// specimen from #0106 (a deep-analysis prompt whose only discriminative tokens
// were "application" and "llm", each present in only one unrelated validated
// record): it must inject nothing under the new gate.
func TestRetrievePushSpecimenPromptServesNothing(t *testing.T) {
	ctx := context.Background()
	recs := []*record.Record{
		mkRecord(t, 620, "Docker Compose application restart loop",
			"the application container restarts in a loop application application under compose", nil, "Go", "compose"),
		mkRecord(t, 621, "Selftest convention for argument parsing",
			"use an llm judge selftest llm llm to check argument parsing invariants", nil, "Go", "selftest"),
	}
	for i := 0; i < 10; i++ {
		recs = append(recs, mkRecord(t, 630+i, "unrelated filler", "cache eviction retry budget notes", nil, "Go", "frob"))
	}
	ix := openIndex(t, recs)

	const specimen = "need a deep analysis of this application and why it is still not working well not helping any llm"
	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: specimen})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if len(dec.Served) != 0 {
		t.Errorf("specimen prompt served %v, want 0", dec.Served)
	}
}

// TestRetrievePushCorroborationServesWhenTokensCooccur is the other half of
// #0108: two discriminative tokens that DO co-occur in one eligible record
// clear the gate and are served.
func TestRetrievePushCorroborationServesWhenTokensCooccur(t *testing.T) {
	ctx := context.Background()
	ix := pushCorroborationFixture(t)

	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: "wibblesnout florptastic"})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if len(dec.Served) != 1 || dec.Served[0].ID != "exp-0603" {
		t.Errorf("co-occurring disc tokens = %v, want exp-0603 served", dec.Served)
	}
}

// TestRetrievePushFingerprintBypassUnaffectedByEligibilityAndTrigger pins that
// the fingerprint-exact bypass is UNCHANGED by #0107/#0108: any validated
// record — importer-origin, ineligible kind, whatever the trigger — is served
// on an exact stack-signature match, because a deterministic signature is
// precision by construction (ADR-0015).
func TestRetrievePushFingerprintBypassUnaffectedByEligibilityAndTrigger(t *testing.T) {
	ctx := context.Background()
	const sig = "panic: nonlinear flux capacitor desync at frame 42"
	rec := mkRecordKindOrigin(t, 700, "convention", "twiceshy-importer",
		"An importer-origin convention that happens to carry a signature", "irrelevant")
	rec.Symptom = &record.Symptom{Summary: "carries the fingerprint on purpose", ErrorSignatures: []string{sig}}
	ix := openIndex(t, []*record.Record{rec})

	for _, errTrig := range []bool{false, true} {
		dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: sig, ErrorTrigger: errTrig})
		if err != nil {
			t.Fatalf("RetrievePushTraced(ErrorTrigger=%v): %v", errTrig, err)
		}
		if !dec.FingerprintBypass {
			t.Fatalf("ErrorTrigger=%v: expected a fingerprint bypass, got %+v", errTrig, dec)
		}
		if len(dec.Served) != 1 || dec.Served[0].ID != "exp-0700" {
			t.Errorf("ErrorTrigger=%v: bypass served %v, want exp-0700 (importer-origin/ineligible-kind must not matter)", errTrig, dec.Served)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"fmt"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/screen"
)

// ReportInput is an outcome report a consuming agent submits via report_outcome
// (#0031): the served lesson that did not hold, what happened, and the evidence.
type ReportInput struct {
	// RecordID is the existing record the report disputes (exp-NNNN).
	RecordID string
	// Outcome is a short label of what happened, e.g. "failed", "reproduced",
	// "incorrect". The caller bounds its length.
	Outcome string
	// Evidence is the failing repro / error text. May be empty — a bare report
	// is a triage flag, not a reproducible artifact (ADR-0013 §3, signal quality).
	Evidence string
}

// BuildReport turns an outcome report into a quarantined counter-record: a
// `dead-end` that **disputes** RecordID (ADR-0013 §3). It is propose-only — it
// never mutates the disputed record, only proposes re-validation work that the
// #0032 gate adjudicates. The evidence is untrusted text headed for a repro, so
// it passes the same content screen as any ingest (#0011/#0019); a hit is
// documented in security_flags (quarantine-with-flag), or refused outright when
// Meta.RejectOnFlag is set.
//
// Unlike Prepare, BuildReport does NOT deduplicate against the corpus: a report
// is fresh counter-evidence *about* an existing record, never a duplicate of it
// — running it through dedup would reject it as a match on the very record it
// contests.
//
// The record is a `dead-end` by kind, but `provenance.disputes` is the
// discriminator that sets it apart from a curated dead-end lesson: it marks the
// record as an outcome report for #0032 to adjudicate, and (carrying no repro)
// it is never eligible for #0029 auto-promotion.
func BuildReport(in ReportInput, m Meta) (*record.Record, error) {
	if !record.ValidID(in.RecordID) {
		return nil, fmt.Errorf("ingest: report references %q, not a valid record id", in.RecordID)
	}
	outcome := strings.TrimSpace(in.Outcome)
	if outcome == "" {
		outcome = "unspecified"
	}
	evidence := strings.TrimSpace(in.Evidence)

	title := capRunes(fmt.Sprintf("Outcome report against %s — reported %s", in.RecordID, outcome), 120)

	whyFailed := evidence
	if whyFailed == "" {
		whyFailed = "reported outcome: " + outcome + "; no reproducible artifact provided (triage flag)"
	}

	var body strings.Builder
	body.WriteString("## Outcome report\n\n")
	fmt.Fprintf(&body, "A consuming agent reported that the lesson in **%s** did not hold.\n\n", in.RecordID)
	fmt.Fprintf(&body, "- **disputes:** %s\n- **outcome:** %s\n\n", in.RecordID, outcome)
	body.WriteString("### Evidence\n\n")
	if evidence != "" {
		body.WriteString("```\n")
		body.WriteString(evidence)
		body.WriteString("\n```\n")
	} else {
		body.WriteString("_No reproducible artifact provided — this is a triage flag, not execution-backed counter-evidence._\n")
	}
	body.WriteString("\nThis is a quarantined counter-record. It is evidence, not a verdict: the #0032 gate turns it into a repro and re-runs the original plus the counter before anything is demoted.")

	disputes := in.RecordID
	rec := &record.Record{
		SchemaVersion: record.SchemaVersion,
		ID:            m.ID,
		Kind:          "dead-end",
		Status:        "quarantined",
		Title:         title,
		Symptom: &record.Symptom{
			Summary: fmt.Sprintf("A consuming agent reported that the lesson in %s did not work (outcome: %s).", in.RecordID, outcome),
		},
		Resolution: &record.Resolution{
			DeadEnds: []record.DeadEnd{{
				Tried:       fmt.Sprintf("Relied on the lesson in %s", in.RecordID),
				WhyItFailed: whyFailed,
			}},
		},
		Body: strings.TrimSpace(body.String()),
		Provenance: record.Provenance{
			Source:     record.Source{Author: m.Author, Session: m.Session},
			RecordedAt: m.Now,
			Valid:      record.Validity{From: m.Now},
			Disputes:   &disputes,
		},
		Path: BuildPath(m.Now, m.ID, title),
	}

	// Same safety gate as any ingest: the evidence is untrusted text bound for a
	// repro. Document hazards in security_flags (quarantine-with-flag), or refuse
	// outright under RejectOnFlag. The error never echoes a secret.
	if fs := screen.Scan(scanTexts(rec)...); len(fs) > 0 {
		flags := screen.Flags(fs)
		if m.RejectOnFlag {
			return nil, fmt.Errorf("ingest: report rejected by safety gate: %v", flags)
		}
		rec.Provenance.SecurityFlags = flags
	}

	if err := record.Validate(rec); err != nil {
		return nil, fmt.Errorf("ingest: invalid counter-record: %w", err)
	}
	return rec, nil
}

// capRunes truncates s to at most n runes (the schema's title max), so a long
// outcome label can never blow the 120-rune title bound.
func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

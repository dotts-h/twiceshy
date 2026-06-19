// SPDX-License-Identifier: AGPL-3.0-only

// Package judge is the keystone of ADR-0013's closed loop: a seam that decides
// what a green broker attestation cannot. A holding attestation proves a
// record's repro ran fail-pre / pass-post — that the claim *behaves* as stated.
// It cannot tell whether the proof captures the *intended, correctly-scoped*
// lesson, whether the record is license-clean, or whether it could mislead a
// future agent. A diverse frontier model — different family from the drafter,
// never the cheap local LLM (standing rule) — is the secondary filter that
// checks those four things ("a gate is a lead, not a verdict").
//
// The judge is an injectable seam (stubbed in tests, no network in CI), like the
// embedder and endoflife seams. A judge that errors, times out, or returns a
// garbled response yields NO verdict, and the caller treats "no verdict" as
// not-approved (fail-safe): nothing is ever auto-promoted without a recorded
// PASS (ADR-0013 §6).
package judge

import (
	"context"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// Decision is the judge's overall call on a proven record.
type Decision string

const (
	// Approve means the record may be auto-promoted (subject to the gate + soak).
	Approve Decision = "approve"
	// Reject means it stays quarantined for a human (or correction).
	Reject Decision = "reject"
)

// CheckName identifies one of the four things the judge inspects that a green
// attestation cannot (ADR-0013 §1).
type CheckName string

const (
	// Meaning: does the repro capture the *intended* lesson, or pass for the
	// wrong reason?
	Meaning CheckName = "meaning"
	// Scope: does applies_to match what was actually proven?
	Scope CheckName = "scope"
	// License: is the record license-clean per ADR-0003?
	License CheckName = "license"
	// Poison: could this record mislead a future agent? (best-effort —
	// backstopped by the veto window, monitoring, and the outcome-feedback loop)
	Poison CheckName = "poison"
)

// Checks is the canonical, ordered set every complete verdict must cover.
var Checks = []CheckName{Meaning, Scope, License, Poison}

// Check is one check's structured outcome.
type Check struct {
	Name   CheckName `json:"check"`
	Pass   bool      `json:"pass"`
	Reason string    `json:"reason"`
}

// Verdict is the judge's structured output. It is recorded verbatim in a
// promoted record's provenance for audit (ADR-0013 §2); Model names the model
// that produced it, so a later spot-audit knows which judge to distrust.
type Verdict struct {
	Decision Decision `json:"decision"`
	Checks   []Check  `json:"checks"`
	Model    string   `json:"model"`
}

// Approved reports a clean PASS: Decision is Approve AND every one of the four
// canonical checks is present and passing, AND no check (canonical or extra)
// failed. Anything else — a Reject, a missing check, any failing check, the
// zero Verdict — is NOT approved. The promotion path treats !Approved() (and
// any judge error) as "stay quarantined" (fail-safe, ADR-0013 §6). This is the
// single chokepoint that keeps a malformed or partial verdict from promoting.
func (v Verdict) Approved() bool {
	if v.Decision != Approve {
		return false
	}
	seen := make(map[CheckName]bool, len(v.Checks))
	for _, c := range v.Checks {
		if !c.Pass {
			return false // any failing check blocks, even an unexpected one
		}
		seen[c.Name] = true
	}
	for _, name := range Checks {
		if !seen[name] {
			return false // every canonical check must be present and passing
		}
	}
	return true
}

// ApproveVerdict is a fully-passing verdict for the given model — a convenience
// for tests and stubs (the happy path; real approvals come from a model).
func ApproveVerdict(model string) Verdict {
	checks := make([]Check, len(Checks))
	for i, name := range Checks {
		checks[i] = Check{Name: name, Pass: true, Reason: "ok"}
	}
	return Verdict{Decision: Approve, Checks: checks, Model: model}
}

// ReproArtifact is one repro script the judge reads to check meaning and scope:
// the proof's executable body, not just the attestation's pass/fail result. The
// caller resolves guard.repro / guard.repros to their contents.
type ReproArtifact struct {
	Path    string
	Kind    string
	Label   string
	Content string
}

// Request bundles everything the judge sees: the record under judgement, the
// holding attestation that proves its claim ran fail-pre/pass-post, and the
// repro scripts behind that proof.
type Request struct {
	Record      *record.Record
	Attestation repro.Attestation
	Repros      []ReproArtifact
}

// Judge decides what a green attestation cannot. A non-nil error means NO
// verdict — the caller MUST treat it as not-approved (fail-safe); the judge
// never returns a spurious approve on error (ADR-0013 §6, issue #0028).
type Judge interface {
	Judge(ctx context.Context, req Request) (Verdict, error)
}

// StubJudge is a deterministic, network-free Judge for tests and for wiring the
// promote/demote paths (#0029/#0032) without a live endpoint. It returns Verdict
// (or Err) verbatim and counts calls. It is NOT a real judge — no diversity, no
// reasoning — so it must never be wired into a production promotion path.
type StubJudge struct {
	Verdict Verdict
	Err     error
	Calls   int
}

// Judge returns the primed verdict or error.
func (s *StubJudge) Judge(_ context.Context, _ Request) (Verdict, error) {
	s.Calls++
	if s.Err != nil {
		return Verdict{}, s.Err
	}
	return s.Verdict, nil
}

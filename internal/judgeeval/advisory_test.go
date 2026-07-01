// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"fmt"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// BuildAdvisoryGold maps each audited verdict onto an advisory-class gold case: approve
// → mode approve / no checks; reject → its failing checks (mode = first); a reject the
// audit left unlabelled → a check inferred from the reason (#0074).
func TestBuildAdvisoryGold_MapsVerdictsToCases(t *testing.T) {
	adv := func(id string) *record.Record {
		return &record.Record{
			ID: id, Kind: "trap", Title: "GHSA-" + id + ": vulnerability in example.com/pkg",
			Symptom: &record.Symptom{ErrorSignatures: []string{"GHSA-" + id}},
		}
	}
	corpus := map[string]*record.Record{
		"exp-0007": adv("exp-0007"), "exp-0010": adv("exp-0010"),
		"exp-0053": adv("exp-0053"), "exp-0054": adv("exp-0054"),
	}
	lookup := func(id string) (*record.Record, error) {
		if r, ok := corpus[id]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("record %s not in corpus", id)
	}
	audit := AdvisoryAudit{
		Approved: []AdvisoryEntry{{ID: "exp-0007", Decision: "approve", Reason: "clean advisory, approve"}},
		Rejected: []AdvisoryEntry{
			{ID: "exp-0010", Decision: "reject", FailedChecks: []string{"meaning", "poison"}, Reason: "wrong and poisoned"},
			{ID: "exp-0053", Decision: "reject", Reason: "Internal contradiction: fixed=null yet the fix text says upgrade — fails meaning and poison checks."},
			{ID: "exp-0054", Decision: "reject", Reason: "Package path omits the required major-version suffix — fails scope checks."},
		},
	}

	doc, err := BuildAdvisoryGold(audit, lookup)
	if err != nil {
		t.Fatalf("BuildAdvisoryGold: %v", err)
	}

	var gf goldFile
	if err := yaml.Unmarshal([]byte(doc), &gf); err != nil {
		t.Fatalf("generated doc must be valid gold YAML: %v\n%s", err, doc)
	}
	if len(gf.Cases) != 4 {
		t.Fatalf("want 4 cases, got %d", len(gf.Cases))
	}
	byID := map[string]struct {
		mode, decision string
		checks         []string
		advisory       bool
		repros         int
	}{}
	for _, c := range gf.Cases {
		byID[c.ID] = struct {
			mode, decision string
			checks         []string
			advisory       bool
			repros         int
		}{c.Mode, c.WantDecision, c.WantFailingChecks, record.IsAdvisoryClass(c.Record), len(c.Repros)}
	}

	// Approve entry → mode approve, no checks.
	if g := byID["exp-0007"]; g.mode != "approve" || g.decision != "approve" || len(g.checks) != 0 {
		t.Errorf("exp-0007: got %+v, want approve/approve/no-checks", g)
	}
	// Reject with explicit checks → mode = first check, both checks carried.
	if g := byID["exp-0010"]; g.mode != "meaning" || g.decision != "reject" || len(g.checks) != 2 {
		t.Errorf("exp-0010: got %+v, want meaning/reject/[meaning poison]", g)
	}
	// Reject, empty failed_checks, reason names "meaning and poison" → both recovered.
	if g := byID["exp-0053"]; g.mode != "meaning" || len(g.checks) != 2 {
		t.Errorf("exp-0053: got %+v, want meaning + [meaning poison] recovered from prose", g)
	}
	// Reject, empty failed_checks, reason names "scope" → scope recovered.
	if g := byID["exp-0054"]; g.mode != "scope" || len(g.checks) != 1 || g.checks[0] != "scope" {
		t.Errorf("exp-0054: got %+v, want scope recovered from prose", g)
	}
	// Every case is advisory-class and carries no repro.
	for id, g := range byID {
		if !g.advisory {
			t.Errorf("%s: not advisory-class", id)
		}
		if g.repros != 0 {
			t.Errorf("%s: advisory case must have no repro, got %d", id, g.repros)
		}
	}
}

// inferChecks recovers failing checks from reject reason prose by WHOLE WORD, so a
// check name embedded in a larger word ("telescope", "licensed", "meaningful") does not
// falsely register; a reason naming no check defaults to meaning.
func TestInferChecks_WholeWordOnly(t *testing.T) {
	cases := []struct {
		reason string
		want   []string
	}{
		{"fails meaning and poison checks", []string{"meaning", "poison"}},
		{"a telescope, a licensed and meaningful thing", []string{"meaning"}}, // no whole-word check → default
		{"the scope of the package is wrong", []string{"scope"}},
		{"nothing relevant here", []string{"meaning"}},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			got := inferChecks(tc.reason)
			if len(got) != len(tc.want) {
				t.Fatalf("inferChecks(%q) = %v, want %v", tc.reason, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("inferChecks(%q) = %v, want %v", tc.reason, got, tc.want)
				}
			}
		})
	}
}

// canonicalFirstCheck picks the representative mode by fixed precedence, so the same
// check SET yields the same mode regardless of input order.
func TestCanonicalFirstCheck(t *testing.T) {
	if got := canonicalFirstCheck([]string{"poison", "meaning"}); got != "meaning" {
		t.Errorf("got %q, want meaning (canonical-first regardless of input order)", got)
	}
	if got := canonicalFirstCheck([]string{"scope"}); got != "scope" {
		t.Errorf("got %q, want scope", got)
	}
}

// A reject whose record is missing from the corpus is a hard error — the gold set must
// stay internally consistent.
func TestBuildAdvisoryGold_MissingRecordErrors(t *testing.T) {
	lookup := func(id string) (*record.Record, error) { return nil, fmt.Errorf("record %s not in corpus", id) }
	_, err := BuildAdvisoryGold(AdvisoryAudit{
		Approved: []AdvisoryEntry{{ID: "exp-9999", Decision: "approve", Reason: "x"}},
	}, lookup)
	if err == nil {
		t.Fatal("expected an error for a missing record")
	}
}

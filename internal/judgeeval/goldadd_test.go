// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

func TestGoldCaseStanza_RoundTrips(t *testing.T) {
	rejectRec := &record.Record{
		ID:    "exp-9999",
		Kind:  "fix",
		Title: "Reject fixture title",
	}
	rejectRepro := judge.ReproArtifact{
		Path:    "experience/repro/exp-9999/repro.sh",
		Kind:    "positive",
		Label:   "license repro",
		Content: "#!/bin/sh\necho LICENSE_REPRO_OK\n",
	}
	rejectStanza, err := GoldCaseStanza(GoldStanzaInput{
		ID:        "G99",
		Mode:      "license",
		Rationale: "judge missed license encumbrance",
		Checks:    []string{"license"},
		Record:    rejectRec,
		Repros:    []judge.ReproArtifact{rejectRepro},
		Attestation: struct {
			Holds, Inconclusive bool
			ReproducedUnder     []string
		}{Holds: true, ReproducedUnder: []string{"go1.25"}},
	})
	if err != nil {
		t.Fatalf("GoldCaseStanza reject: %v", err)
	}

	var gf goldFile
	if err := yaml.Unmarshal([]byte("cases:\n"+rejectStanza), &gf); err != nil {
		t.Fatalf("unmarshal reject stanza: %v\n---\n%s", err, rejectStanza)
	}
	if len(gf.Cases) != 1 {
		t.Fatalf("cases len = %d, want 1", len(gf.Cases))
	}
	gc := gf.Cases[0]
	if gc.ID != "G99" || gc.Mode != "license" || gc.WantDecision != "reject" {
		t.Errorf("got id=%q mode=%q want_decision=%q", gc.ID, gc.Mode, gc.WantDecision)
	}
	if len(gc.WantFailingChecks) != 1 || gc.WantFailingChecks[0] != "license" {
		t.Errorf("want_failing_checks = %v", gc.WantFailingChecks)
	}
	if gc.Record == nil || gc.Record.Title != rejectRec.Title {
		t.Errorf("record title = %q, want %q", gc.Record.Title, rejectRec.Title)
	}
	if len(gc.Repros) != 1 || !strings.Contains(gc.Repros[0].Content, "LICENSE_REPRO_OK") {
		t.Errorf("repro content = %q", gc.Repros[0].Content)
	}

	approveRec := &record.Record{
		ID:    "exp-8888",
		Kind:  "fix",
		Title: "Approve fixture title",
	}
	approveStanza, err := GoldCaseStanza(GoldStanzaInput{
		ID:        "G88",
		Mode:      "approve",
		Rationale: "clean record the judge wrongly rejected",
		Record:    approveRec,
		Repros: []judge.ReproArtifact{{
			Path: "experience/repro/exp-8888/repro.sh", Kind: "positive", Content: "echo APPROVE_OK",
		}},
	})
	if err != nil {
		t.Fatalf("GoldCaseStanza approve: %v", err)
	}
	if err := yaml.Unmarshal([]byte("cases:\n"+approveStanza), &gf); err != nil {
		t.Fatalf("unmarshal approve stanza: %v\n---\n%s", err, approveStanza)
	}
	gc = gf.Cases[0]
	if gc.ID != "G88" || gc.Mode != "approve" || gc.WantDecision != "approve" {
		t.Errorf("got id=%q mode=%q want_decision=%q", gc.ID, gc.Mode, gc.WantDecision)
	}
	if len(gc.WantFailingChecks) != 0 {
		t.Errorf("approve case want_failing_checks = %v, want empty", gc.WantFailingChecks)
	}
}

func TestGoldCaseStanza_RejectsBad(t *testing.T) {
	base := GoldStanzaInput{
		ID: "Gx", Mode: "license", Rationale: "x",
		Record: &record.Record{ID: "exp-1", Title: "t"},
		Repros: []judge.ReproArtifact{{Path: "p", Kind: "positive", Content: "c"}},
	}
	cases := []struct {
		name string
		in   GoldStanzaInput
		want string
	}{
		{
			name: "approve with checks",
			in: func() GoldStanzaInput {
				in := base
				in.Mode = "approve"
				in.Checks = []string{"license"}
				return in
			}(),
			want: "no failing checks",
		},
		{
			name: "reject without checks",
			in:   base,
			want: "at least one failing check",
		},
		{
			name: "missing repro",
			in: func() GoldStanzaInput {
				in := base
				in.Checks = []string{"license"}
				in.Repros = nil
				return in
			}(),
			want: "repro",
		},
		{
			name: "unknown mode",
			in: func() GoldStanzaInput {
				in := base
				in.Mode = "bogus"
				in.Checks = []string{"license"}
				return in
			}(),
			want: "unknown mode",
		},
		{
			name: "empty id",
			in: func() GoldStanzaInput {
				in := base
				in.ID = ""
				in.Checks = []string{"license"}
				return in
			}(),
			want: "id is required",
		},
		{
			name: "missing title",
			in: func() GoldStanzaInput {
				in := base
				in.Checks = []string{"license"}
				in.Record = &record.Record{ID: "exp-1"}
				return in
			}(),
			want: "title is required",
		},
		{
			name: "unknown check",
			in: func() GoldStanzaInput {
				in := base
				in.Checks = []string{"not-a-check"}
				return in
			}(),
			want: "unknown check",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GoldCaseStanza(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"os"
	"path/filepath"
	"testing"
)

// The embedded prospect_gold.yaml starts empty (no cases yet drafted) — loading it
// must return zero cases without error, never a parse failure on an empty file.
func TestLoadProspectGold_EmptyIsTolerated(t *testing.T) {
	cases, err := LoadProspectGold()
	if err != nil {
		t.Fatalf("LoadProspectGold: %v", err)
	}
	if len(cases) != 0 {
		t.Errorf("want 0 cases from the (currently empty) checked-in gold set, got %d", len(cases))
	}
}

func TestMergeProspectGold_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prospect_gold.yaml")
	cases := []ProspectCase{
		{TrapID: "exp-1001", Prompt: "build a search query", VerifyID: "gobuild", Card: "the card text"},
		{TrapID: "exp-1002", Prompt: "style a component", VerifyID: "tsc", Deps: []string{"typescript", "@types/react@19"}, Card: "the other card"},
	}
	if err := MergeProspectGold(path, cases); err != nil {
		t.Fatalf("MergeProspectGold: %v", err)
	}

	got, err := loadProspectGoldFrom(path)
	if err != nil {
		t.Fatalf("loadProspectGoldFrom: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 round-tripped cases, got %d: %+v", len(got), got)
	}
	byID := map[string]TaskCase{}
	for _, c := range got {
		byID[c.TrapID] = c
	}
	if byID["exp-1001"].Prompt != "build a search query" || byID["exp-1001"].VerifyID != "gobuild" {
		t.Errorf("exp-1001 round-tripped wrong: %+v", byID["exp-1001"])
	}
	if byID["exp-1001"].Card != "the card text" {
		t.Errorf("exp-1001 Card = %q, want %q", byID["exp-1001"].Card, "the card text")
	}
	if len(byID["exp-1002"].Deps) != 2 || byID["exp-1002"].Deps[0] != "typescript" {
		t.Errorf("exp-1002 Deps = %v, want [typescript @types/react@19]", byID["exp-1002"].Deps)
	}
}

// A second merge with an overlapping TrapID must NOT overwrite the existing
// entry — existing entries win, per the spec.
func TestMergeProspectGold_ExistingEntriesWin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prospect_gold.yaml")
	first := []ProspectCase{{TrapID: "exp-2001", Prompt: "original prompt", VerifyID: "gobuild"}}
	if err := MergeProspectGold(path, first); err != nil {
		t.Fatalf("first MergeProspectGold: %v", err)
	}
	second := []ProspectCase{
		{TrapID: "exp-2001", Prompt: "CHANGED — must be ignored", VerifyID: "tsc", Deps: []string{"typescript"}},
		{TrapID: "exp-2002", Prompt: "a genuinely new case", VerifyID: "gobuild"},
	}
	if err := MergeProspectGold(path, second); err != nil {
		t.Fatalf("second MergeProspectGold: %v", err)
	}

	got, err := loadProspectGoldFrom(path)
	if err != nil {
		t.Fatalf("loadProspectGoldFrom: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 deduped cases (exp-2001, exp-2002), got %d: %+v", len(got), got)
	}
	byID := map[string]TaskCase{}
	for _, c := range got {
		byID[c.TrapID] = c
	}
	if byID["exp-2001"].Prompt != "original prompt" {
		t.Errorf("existing entry must win: exp-2001 Prompt = %q, want %q", byID["exp-2001"].Prompt, "original prompt")
	}
	if _, ok := byID["exp-2002"]; !ok {
		t.Error("exp-2002 must be appended as a new case")
	}
}

// MergeProspectGold must tolerate a not-yet-existing path (the first prospect run
// against a fresh -gold-out target).
func TestMergeProspectGold_AbsentFileIsCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist-yet", "prospect_gold.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := MergeProspectGold(path, []ProspectCase{{TrapID: "exp-3001", Prompt: "p", VerifyID: "gobuild"}}); err != nil {
		t.Fatalf("MergeProspectGold on a fresh path: %v", err)
	}
	got, err := loadProspectGoldFrom(path)
	if err != nil {
		t.Fatalf("loadProspectGoldFrom: %v", err)
	}
	if len(got) != 1 || got[0].TrapID != "exp-3001" {
		t.Errorf("got = %+v, want one case exp-3001", got)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

package corpusquality_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/corpusquality"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestBuildCountsQualityAndRightsSignals(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "experience/repro/one.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "experience/repro/set/repro.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "experience/repro/not-runnable/readme.md"), "notes\n")

	legacy := "experience/repro/one.sh"
	set := "experience/repro/set"
	notRunnable := "experience/repro/not-runnable"
	recs := []*record.Record{
		{
			ID: "exp-0001", Kind: "trap", Status: "validated",
			Guard:      &record.Guard{Repro: &legacy},
			Provenance: record.Provenance{Source: record.Source{Author: "human"}, SourceLicense: "MIT", SourceURL: "https://example.test/one"},
		},
		{
			ID: "exp-0002", Kind: "fix", Status: "validated",
			Guard:      &record.Guard{Repros: []record.Repro{{Path: set, Kind: "positive"}, {Path: notRunnable, Kind: "negative"}}},
			Provenance: record.Provenance{Source: record.Source{Author: "human"}, SourceLicense: record.SourceLicenseFactsOnly},
		},
		{
			ID: "exp-0003", Kind: "trap", Status: "validated",
			Symptom:    &record.Symptom{ErrorSignatures: []string{"CVE-2026-1234"}},
			Provenance: record.Provenance{Source: record.Source{Author: "osv"}, SourceLicense: "CC-BY-4.0", SourceURL: "https://osv.dev/vulnerability/CVE-2026-1234"},
		},
		{ID: "exp-0004", Kind: "convention", Status: "quarantined"},
	}

	got := corpusquality.Build(root, recs)
	if got.TotalRecords != 4 {
		t.Fatalf("TotalRecords = %d, want 4", got.TotalRecords)
	}
	if got.StatusCounts["validated"] != 3 || got.StatusCounts["quarantined"] != 1 {
		t.Errorf("StatusCounts = %#v", got.StatusCounts)
	}
	if got.KindCounts["trap"] != 2 || got.KindCounts["fix"] != 1 || got.KindCounts["convention"] != 1 {
		t.Errorf("KindCounts = %#v", got.KindCounts)
	}
	if got.ValidatedActionableBehavioral != 2 {
		t.Errorf("ValidatedActionableBehavioral = %d, want 2", got.ValidatedActionableBehavioral)
	}
	if got.RecordsWithGuard != 2 || got.DeclaredRepros != 3 || got.RunnableRepros != 2 {
		t.Errorf("guard/declared/runnable counts = %d/%d/%d, want 2/3/2", got.RecordsWithGuard, got.DeclaredRepros, got.RunnableRepros)
	}
	if got.Coverage.Provenance != 3 || got.Coverage.SourceLicense != 3 || got.Coverage.SourceURL != 2 {
		t.Errorf("Coverage = %#v, want provenance/license/url 3/3/2", got.Coverage)
	}
	if got.LicenseCounts["MIT"] != 1 || got.LicenseCounts["CC-BY-4.0"] != 1 || got.LicenseCounts[record.SourceLicenseFactsOnly] != 1 || got.LicenseCounts[corpusquality.Missing] != 1 {
		t.Errorf("LicenseCounts = %#v", got.LicenseCounts)
	}
}

func TestBuildIncludesZeroesForKnownStatusesAndKinds(t *testing.T) {
	got := corpusquality.Build(t.TempDir(), nil)
	for _, status := range record.Statuses {
		if n, ok := got.StatusCounts[status]; !ok || n != 0 {
			t.Errorf("status %q = %d, present=%v", status, n, ok)
		}
	}
	for _, kind := range record.Kinds {
		if n, ok := got.KindCounts[kind]; !ok || n != 0 {
			t.Errorf("kind %q = %d, present=%v", kind, n, ok)
		}
	}
}

func TestIsValidatedActionableBehavioralUsesRecordClass(t *testing.T) {
	cases := []struct {
		name string
		rec  *record.Record
		want bool
	}{
		{"validated trap", &record.Record{Kind: "trap", Status: "validated"}, true},
		{"validated fix", &record.Record{Kind: "fix", Status: "validated"}, true},
		{"validated dead-end", &record.Record{Kind: "dead-end", Status: "validated"}, true},
		{"advisory trap", &record.Record{Kind: "trap", Status: "validated", Symptom: &record.Symptom{ErrorSignatures: []string{"GHSA-aaaa-bbbb-cccc"}}}, false},
		{"quarantined trap", &record.Record{Kind: "trap", Status: "quarantined"}, false},
		{"convention", &record.Record{Kind: "convention", Status: "validated"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := corpusquality.IsValidatedActionableBehavioral(tc.rec); got != tc.want {
				t.Errorf("IsValidatedActionableBehavioral() = %v, want %v", got, tc.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

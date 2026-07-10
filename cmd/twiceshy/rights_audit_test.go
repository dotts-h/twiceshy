// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/rightsaudit"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

func TestRightsAuditEmitsJSONAndMachineActionableQueue(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "remediation.json")
	var out bytes.Buffer
	if err := runRightsAudit([]string{"-corpus", testcorpus.Root(), "-json", "-queue", queue}, &out); err != nil {
		t.Fatalf("runRightsAudit: %v", err)
	}
	var report rightsaudit.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("report JSON: %v\n%s", err, out.String())
	}
	if report.TotalRecords == 0 || report.UnresolvedEvidence == 0 {
		t.Fatalf("unexpected fixture report: %+v", report)
	}
	data, err := os.ReadFile(queue)
	if err != nil {
		t.Fatal(err)
	}
	var items []rightsaudit.Remediation
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("queue JSON: %v\n%s", err, data)
	}
	if len(items) != report.UnresolvedEvidence {
		t.Fatalf("queue items = %d, unresolved = %d", len(items), report.UnresolvedEvidence)
	}
}

func TestRightsAuditFailOnUnknownStillEmitsReportAndQueue(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "remediation.json")
	var out bytes.Buffer
	err := runRightsAudit([]string{"-corpus", testcorpus.Root(), "-json", "-queue", queue, "-fail-on-unknown"}, &out)
	if err == nil {
		t.Fatal("missing rights evidence must fail the CI mode")
	}
	if out.Len() == 0 {
		t.Fatal("CI failure must still emit the audit report")
	}
	if _, statErr := os.Stat(queue); statErr != nil {
		t.Fatalf("CI failure must still emit remediation queue: %v", statErr)
	}
}

func TestRightsAuditJSONAndQueueAreDeterministic(t *testing.T) {
	dir := t.TempDir()
	var first, second bytes.Buffer
	q1 := filepath.Join(dir, "first.json")
	q2 := filepath.Join(dir, "second.json")
	if err := runRightsAudit([]string{"-corpus", testcorpus.Root(), "-json", "-queue", q1}, &first); err != nil {
		t.Fatal(err)
	}
	if err := runRightsAudit([]string{"-corpus", testcorpus.Root(), "-json", "-queue", q2}, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("rights-audit JSON is not deterministic")
	}
	b1, err := os.ReadFile(q1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(q2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatal("remediation queue is not deterministic")
	}
}

func TestRunDispatchesRightsAudit(t *testing.T) {
	var out bytes.Buffer
	if err := run(t.Context(), []string{"rights-audit", "-corpus", testcorpus.Root(), "-json"}, &out, noEnv); err != nil {
		t.Fatal(err)
	}
}

func TestRightsAuditValidatesCommercialPackArtifacts(t *testing.T) {
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatal(err)
	}
	manifest := pack.BuildManifest(recs, true, false)
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "MANIFEST.json")
	noticesPath := filepath.Join(dir, "ATTRIBUTION.md")
	packLicensePath := filepath.Join(dir, "LICENSE")
	packLicense := []byte("Fixture commercial pack terms\n")
	manifest.PackLicenseSHA256 = pack.LicenseDigest(packLicense)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(manifestJSON, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, pack.NoticeDocument(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packLicensePath, packLicense, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"-corpus", testcorpus.Root(), "-json", "-manifest", manifestPath, "-notices", noticesPath, "-pack-license", packLicensePath}
	if err := runRightsAudit(args, &out); err != nil {
		t.Fatalf("canonical pack artifacts: %v", err)
	}
	var report rightsaudit.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.ArtifactValidation == nil || !report.ArtifactValidation.Valid {
		t.Fatalf("artifact validation = %+v", report.ArtifactValidation)
	}

	if err := os.WriteFile(noticesPath, []byte("# incomplete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runRightsAudit(args, &out); err == nil {
		t.Fatal("drifted notice document must fail validation")
	}
}

func TestRightsAuditRejectsManifestTrailingData(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "MANIFEST.json")
	noticesPath := filepath.Join(dir, "ATTRIBUTION.md")
	licensePath := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(manifestPath, []byte(`{"commercial":true} trailing`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, []byte("# notices\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(licensePath, []byte("pack terms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runRightsAudit([]string{"-corpus", testcorpus.Root(), "-manifest", manifestPath, "-notices", noticesPath, "-pack-license", licensePath}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("manifest trailing bytes must be rejected")
	}
}

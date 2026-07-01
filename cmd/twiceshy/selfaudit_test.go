// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// self-audit dogfoods twiceshy on its own go.mod (#0014): it loads the corpus,
// matches each dependency against the ingested vulnerability advisories, and
// exits non-zero (so a timer / CI alerts) when a dependency is affected.
func TestSelfAudit(t *testing.T) {
	corpus := writeAdvisoryCorpus(t) // one Go advisory: example.com/vuln affected <1.5.0

	t.Run("flags an affected dependency and exits non-zero", func(t *testing.T) {
		gomod := writeGoMod(t, "example.com/vuln v1.2.0") // 1.2.0 < fixed 1.5.0 → affected
		var buf bytes.Buffer
		err := runSelfAudit([]string{"-corpus", corpus, "-gomod", gomod}, &buf)
		if err == nil {
			t.Fatalf("an affected dep must make self-audit fail; output:\n%s", buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "example.com/vuln@v1.2.0") || !strings.Contains(out, "GHSA-tttt-tttt-tttt") {
			t.Errorf("report missing the hit:\n%s", out)
		}
	})

	t.Run("clean when every dependency is past the fix", func(t *testing.T) {
		gomod := writeGoMod(t, "example.com/vuln v1.6.0") // 1.6.0 >= fixed 1.5.0 → clean
		var buf bytes.Buffer
		if err := runSelfAudit([]string{"-corpus", corpus, "-gomod", gomod}, &buf); err != nil {
			t.Fatalf("a patched dep must pass; got %v\n%s", err, buf.String())
		}
		if !strings.Contains(buf.String(), "no advisory matches") {
			t.Errorf("expected a clean report, got:\n%s", buf.String())
		}
	})

	t.Run("a missing go.mod is a clear error", func(t *testing.T) {
		var buf bytes.Buffer
		err := runSelfAudit([]string{"-corpus", corpus, "-gomod", filepath.Join(t.TempDir(), "nope.mod")}, &buf)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing go.mod should surface os.ErrNotExist; got %v", err)
		}
	})
}

func writeGoMod(t *testing.T, require string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "go.mod")
	body := "module example.com/test\n\ngo 1.25.0\n\nrequire " + require + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeAdvisoryCorpus(t *testing.T) string {
	t.Helper()
	corpus := t.TempDir()
	dir := filepath.Join(corpus, "experience", "2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const rec = `---
schema_version: 1
id: exp-0001
kind: trap
status: quarantined
title: 'GHSA-tttt-tttt-tttt: vulnerability in example.com/vuln'
symptom:
    summary: 'GHSA-tttt-tttt-tttt: known vulnerability in example.com/vuln'
    error_signatures:
        - GHSA-tttt-tttt-tttt
applies_to:
    - ecosystem: Go
      package: example.com/vuln
      versions:
        introduced: "0"
        fixed: "1.5.0"
resolution:
    root_cause: Known vulnerability documented in an OSV advisory.
    fix: Upgrade affected packages past the fixed version.
provenance:
    source:
        author: twiceshy-importer
        session: null
        pr: null
    recorded_at: "2026-06-21"
    validated_at: null
    valid:
        from: "2026-06-21"
        until: null
    source_license: CC-BY-4.0
    source_url: https://osv.dev/vulnerability/GHSA-tttt-tttt-tttt
    superseded_by: null
---

OSV advisory GHSA-tttt-tttt-tttt affects example.com/vuln (introduced 0, fixed 1.5.0).
`
	if err := os.WriteFile(filepath.Join(dir, "0001-ghsa-tttt-example-vuln.md"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	return corpus
}

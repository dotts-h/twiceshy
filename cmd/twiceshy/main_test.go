// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// lockedBuffer lets the test read serve's output while the server
// goroutine is still writing to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// the repo itself is a valid corpus (the three worked examples).
const corpus = "../.."

// writeFixture marshals rec into dir's corpus at its Path (mirrors how a real
// record lands on disk), for tests that need a controlled corpus.
func writeFixture(t *testing.T, dir string, rec *record.Record) {
	t.Helper()
	md, err := record.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal fixture %s: %v", rec.ID, err)
	}
	dst := filepath.Join(dir, filepath.FromSlash(rec.Path))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, md, 0o644); err != nil {
		t.Fatal(err)
	}
}

func packFixture(id, status, license, url string) *record.Record {
	prov := record.Provenance{
		Source:        record.Source{Author: "twiceshy-importer"},
		RecordedAt:    "2026-06-18",
		Valid:         record.Validity{From: "2026-06-18"},
		SourceLicense: license,
		SourceURL:     url,
	}
	if status == "validated" {
		v := "2026-06-18"
		prov.ValidatedAt = &v // a validated record must record validated_at
	}
	return &record.Record{
		SchemaVersion: 1,
		ID:            "exp-" + id,
		Kind:          "convention",
		Status:        status,
		Title:         "Pack fixture record " + id,
		AppliesTo:     []record.AppliesTo{{Ecosystem: "Go"}},
		Provenance:    prov,
		Body:          "Distilled fact for the pack-builder test.",
		Path:          "experience/2026/" + id + "-pack-fixture.md",
	}
}

func TestRunPackCommercialExcludesAndAttributes(t *testing.T) {
	dir := tempCorpus(t)
	writeFixture(t, dir, packFixture("0101", "validated", "MIT", ""))
	writeFixture(t, dir, packFixture("0102", "validated", "CC-BY-4.0", "https://github.com/advisories/GHSA-x"))
	writeFixture(t, dir, packFixture("0103", "validated", "GPL-3.0-only", ""))
	writeFixture(t, dir, packFixture("0104", "quarantined", "MIT", ""))

	outDir := filepath.Join(t.TempDir(), "pack")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"pack", "-corpus", dir, "-out", outDir, "-commercial"}, &out, noEnv); err != nil {
		t.Fatalf("pack: %v", err)
	}

	manifest, err := os.ReadFile(filepath.Join(outDir, "MANIFEST.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	ms := string(manifest)
	if !strings.Contains(ms, "exp-0101") || !strings.Contains(ms, "exp-0102") {
		t.Errorf("manifest should include exp-0101 + exp-0102:\n%s", ms)
	}
	for _, reason := range []string{"copyleft", "not validated"} {
		if !strings.Contains(ms, reason) {
			t.Errorf("manifest should record exclusion reason %q:\n%s", reason, ms)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "experience", "2026", "0101-pack-fixture.md")); err != nil {
		t.Errorf("included record file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "experience", "2026", "0103-pack-fixture.md")); err == nil {
		t.Error("excluded (GPL) record must not be written into the pack")
	}
	attr, err := os.ReadFile(filepath.Join(outDir, "ATTRIBUTION.md"))
	if err != nil {
		t.Fatalf("read attribution: %v", err)
	}
	if !strings.Contains(string(attr), "exp-0102") || !strings.Contains(string(attr), "GHSA-x") {
		t.Errorf("ATTRIBUTION.md must attribute the CC-BY record:\n%s", attr)
	}
}

func TestRunPackRequiresOut(t *testing.T) {
	if err := run(context.Background(), []string{"pack", "-corpus", tempCorpus(t)}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("pack without -out must error")
	}
}

func TestRunPackIncludeQuarantined(t *testing.T) {
	dir := tempCorpus(t)
	writeFixture(t, dir, packFixture("0201", "quarantined", "MIT", ""))
	outDir := filepath.Join(t.TempDir(), "pack")
	if err := run(context.Background(), []string{"pack", "-corpus", dir, "-out", outDir, "-include-quarantined"}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "experience", "2026", "0201-pack-fixture.md")); err != nil {
		t.Errorf("-include-quarantined should write the quarantined record: %v", err)
	}
}

func TestRunRejectsBadInvocations(t *testing.T) {
	env := func(string) string { return "" }
	cases := map[string][]string{
		"no subcommand":      {},
		"unknown subcommand": {"frobnicate"},
		"bad flag":           {"index", "-nope"},
		"serve needs token":  {"serve", "-corpus", corpus, "-db", filepath.Join(t.TempDir(), "ix.db")},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if err := run(context.Background(), args, &bytes.Buffer{}, env); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

// `-h` is a help request, not a failure: run returns flag.ErrHelp (which main
// maps to exit 0), and the usage text never lands on the program's stdout writer
// (it goes to stderr) — guards L4/L5.
func TestRunHelpExitsCleanly(t *testing.T) {
	for _, sub := range []string{"index", "serve"} {
		var out bytes.Buffer
		err := run(context.Background(), []string{sub, "-h"}, &out, func(string) string { return "" })
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s -h: want flag.ErrHelp (exit 0), got %v", sub, err)
		}
		if out.Len() != 0 {
			t.Errorf("%s -h: usage must not go to stdout, got %q", sub, out.String())
		}
	}
}

func TestRunIndexBuildsTheIndex(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ix.db")
	var out bytes.Buffer
	err := run(context.Background(), []string{"index", "-corpus", corpus, "-db", db},
		&out, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run index: %v", err)
	}
	if !strings.Contains(out.String(), "indexed ") || !strings.Contains(out.String(), " records into ") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunIndexRejectsInvalidCorpus(t *testing.T) {
	err := run(context.Background(), []string{"index", "-corpus", t.TempDir(), "-db",
		filepath.Join(t.TempDir(), "ix.db")}, &bytes.Buffer{}, func(string) string { return "" })
	if err == nil {
		t.Error("a corpus without experience/ must fail")
	}
}

// noEnv is the empty environment for non-serve subcommands.
func noEnv(string) string { return "" }

// tempCorpus makes an empty-but-valid corpus (experience/ exists) so buildIndex
// succeeds and NextID starts at exp-0001.
func tempCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "experience", "2026"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ingest go writes quarantined, license-provenanced records to disk; a second
// run is idempotent because they are now part of the corpus (Known → skipped).
func TestRunIngestGoCreatesQuarantinedRecords(t *testing.T) {
	dir := tempCorpus(t)
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db")}, &out, noEnv)
	if err != nil {
		t.Fatalf("ingest go: %v", err)
	}
	if !strings.Contains(out.String(), "created") {
		t.Errorf("output = %q", out.String())
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "experience", "2026", "*.md"))
	if len(matches) < 2 {
		t.Fatalf("want >=2 records written, got %d: %v", len(matches), matches)
	}
	for _, p := range matches {
		rel, _ := filepath.Rel(dir, p)
		rec, err := record.ParseFile(dir, filepath.ToSlash(rel))
		if err != nil {
			t.Fatalf("written record %s does not parse: %v", p, err)
		}
		if rec.Status != "quarantined" {
			t.Errorf("%s: status = %q, want quarantined", rec.ID, rec.Status)
		}
		if rec.Provenance.SourceLicense == "" || rec.Provenance.SourceURL == "" {
			t.Errorf("%s: imported record missing source provenance", rec.ID)
		}
	}

	var out2 bytes.Buffer
	if err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix2.db")}, &out2, noEnv); err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if !strings.Contains(out2.String(), "created 0") {
		t.Errorf("second run must create nothing (idempotent), output = %q", out2.String())
	}
}

func TestRunIngestDryRunWritesNothing(t *testing.T) {
	dir := tempCorpus(t)
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-dry-run"}, &out, noEnv)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "experience", "2026", "*.md"))
	if len(matches) != 0 {
		t.Errorf("dry-run wrote files: %v", matches)
	}
	if !strings.Contains(out.String(), "would") {
		t.Errorf("dry-run output should say what it would do: %q", out.String())
	}
}

func TestRunIngestBadInvocations(t *testing.T) {
	cases := map[string][]string{
		"no source":      {"ingest"},
		"unknown source": {"ingest", "bogus", "-corpus", corpus, "-db", filepath.Join(t.TempDir(), "ix.db")},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if err := run(context.Background(), args, &bytes.Buffer{}, noEnv); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

func TestRunServeServesUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{
			"serve", "-corpus", corpus,
			"-db", filepath.Join(t.TempDir(), "ix.db"),
			"-addr", "127.0.0.1:0",
		}, &out, func(k string) string {
			if k == "TWICESHY_TOKEN" {
				return "test-token"
			}
			return ""
		})
	}()

	addrRe := regexp.MustCompile(`listening on (\S+)`)
	var addr string
	deadline := time.Now().Add(10 * time.Second)
	for addr == "" {
		if time.Now().After(deadline) {
			t.Fatalf("server never reported its address; output: %q", out.String())
		}
		if m := addrRe.FindStringSubmatch(out.String()); m != nil {
			addr = m[1]
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Unauthenticated requests bounce at the door.
	resp, err := http.Post("http://"+addr, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v after cancel", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not shut down after cancel")
	}
}

// doctor staleness runs over a corpus and reports (offline: -endoflife-url ""
// means only the valid.until signal runs, so a fresh import yields no findings).
func TestRunDoctorStaleness(t *testing.T) {
	dir := tempCorpus(t)
	// populate the corpus with importer records (no past valid.until → clean)
	if err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db")}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("ingest go: %v", err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"doctor", "staleness", "-corpus", dir,
		"-endoflife-url", ""}, &out, noEnv); err != nil {
		t.Fatalf("doctor staleness: %v", err)
	}
	if !strings.Contains(out.String(), "doctor staleness:") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunDoctorBadInvocations(t *testing.T) {
	for name, args := range map[string][]string{
		"no doctor":      {"doctor"},
		"unknown doctor": {"doctor", "nope", "-corpus", t.TempDir()},
	} {
		if err := run(context.Background(), args, &bytes.Buffer{}, noEnv); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

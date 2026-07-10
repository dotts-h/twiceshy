// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
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

// corpus is the frozen fixture (internal/testcorpus) — the live corpus is now a
// separate data product (twiceshy-corpus, ADR-0021), so command tests run against
// the bundled fixture instead of the repo root.
var corpus = testcorpus.Root()

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
	switch strings.ToLower(license) {
	case "mit":
		prov.SourceAttribution = &record.SourceAttribution{Title: "Fixture Work", CopyrightNotice: "Copyright 2026 Fixture Authors", LicenseText: pack.ApprovedMITLicenseText}
	case "cc-by-4.0":
		prov.SourceAttribution = &record.SourceAttribution{Creator: "Fixture Creator", Title: "Fixture Work", LicenseURL: "https://creativecommons.org/licenses/by/4.0/", Changes: "Condensed into a test record.", LicenseText: "Creative Commons Attribution 4.0 legal code"}
	}
	if status == "validated" {
		v := "2026-06-18"
		prov.ValidatedAt = &v // a validated record must record validated_at
	}
	r := &record.Record{
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
	r.Provenance.RightsReview = &record.RightsReview{Reviewer: "Jane Rights Reviewer", ReviewedAt: "2026-07-10T12:00:00Z", SourceSHA256: "sha256:" + strings.Repeat("a", 64), Policy: pack.RightsPolicyV1}
	r.Provenance.RightsReview.EvidenceSHA256 = pack.EvidenceDigest(r)
	return r
}

func corpusWithLocal2758AndBase2768(t *testing.T) (string, string) {
	t.Helper()
	dir := tempCorpus(t)
	gitCmd(t, dir, "init", "-q")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	gitCmd(t, dir, "config", "user.name", "Test User")
	writeBaseRecordPath(t, dir, "experience/2026/2768-base.md")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")
	base := strings.TrimSpace(gitCmd(t, dir, "rev-parse", "HEAD"))

	if err := os.Remove(filepath.Join(dir, "experience", "2026", "2768-base.md")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, packFixture("2758", "quarantined", "MIT", ""))
	return dir, base
}

func writeBaseRecordPath(t *testing.T, dir, rel string) {
	t.Helper()
	dst := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("base high-water mark\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRunPackCommercialExcludesAndAttributes(t *testing.T) {
	dir := tempCorpus(t)
	writeFixture(t, dir, packFixture("0101", "validated", "MIT", "https://example.com/upstream/commit/0123456789abcdef"))
	writeFixture(t, dir, packFixture("0102", "validated", "CC-BY-4.0", "https://github.com/advisories/GHSA-x"))
	writeFixture(t, dir, packFixture("0103", "validated", "GPL-3.0-only", ""))
	writeFixture(t, dir, packFixture("0104", "quarantined", "MIT", ""))
	writeFixture(t, dir, packFixture("0105", "validated", record.SourceLicenseProjectAuthored, ""))

	outDir := filepath.Join(t.TempDir(), "pack")
	packLicense := filepath.Join(t.TempDir(), "PACK-LICENSE.txt")
	if err := os.WriteFile(packLicense, []byte("Fixture commercial pack terms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"pack", "-corpus", dir, "-out", outDir, "-commercial", "-license", packLicense}, &out, noEnv); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(outDir, "LICENSE")); err != nil || string(got) != "Fixture commercial pack terms\n" {
		t.Fatalf("commercial pack LICENSE = %q, %v", got, err)
	}

	manifest, err := os.ReadFile(filepath.Join(outDir, "MANIFEST.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	ms := string(manifest)
	if !strings.Contains(ms, "exp-0101") || !strings.Contains(ms, "exp-0105") {
		t.Errorf("manifest should include licensed and explicitly project-authored records:\n%s", ms)
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
	for _, want := range []string{"Source and License Notices", "exp-0101", "MIT", "0123456789abcdef", "THIRD_PARTY/exp-0101-LICENSE.txt"} {
		if !strings.Contains(string(attr), want) {
			t.Errorf("ATTRIBUTION.md must describe copied-source license/notice entries (missing %q):\n%s", want, attr)
		}
	}
	if got, err := os.ReadFile(filepath.Join(outDir, "THIRD_PARTY", "exp-0101-LICENSE.txt")); err != nil || string(got) != pack.ApprovedMITLicenseText {
		t.Fatalf("bundled MIT license = %q, %v", got, err)
	}
}

func TestRunPackCommercialRequiresPackLicenseTerms(t *testing.T) {
	dir := tempCorpus(t)
	writeFixture(t, dir, packFixture("0101", "validated", record.SourceLicenseProjectAuthored, ""))
	err := run(context.Background(), []string{"pack", "-corpus", dir, "-out", filepath.Join(t.TempDir(), "pack"), "-commercial"}, &bytes.Buffer{}, noEnv)
	if err == nil {
		t.Fatal("commercial pack without pack-level LICENSE terms must fail closed")
	}
}

func TestRunPackRejectsNonEmptyOutputWithoutChangingIt(t *testing.T) {
	dir := tempCorpus(t)
	writeFixture(t, dir, packFixture("0101", "validated", record.SourceLicenseProjectAuthored, ""))
	outDir := t.TempDir()
	marker := filepath.Join(outDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	license := filepath.Join(t.TempDir(), "LICENSE")
	if err := os.WriteFile(license, []byte("pack terms"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runPack([]string{"-corpus", dir, "-out", outDir, "-commercial", "-license", license}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("non-empty output directory must be rejected")
	}
	got, readErr := os.ReadFile(marker)
	if readErr != nil || string(got) != "keep me" {
		t.Fatalf("rejected pack changed existing output: %q, %v", got, readErr)
	}
}

func TestRunPackRejectsSymlinkCorpusAndOutputPaths(t *testing.T) {
	realCorpus := tempCorpus(t)
	writeFixture(t, realCorpus, packFixture("0101", "validated", record.SourceLicenseProjectAuthored, ""))
	corpusLink := filepath.Join(t.TempDir(), "corpus-link")
	if err := os.Symlink(realCorpus, corpusLink); err != nil {
		t.Fatal(err)
	}
	if err := runPack([]string{"-corpus", corpusLink, "-out", filepath.Join(t.TempDir(), "pack")}, &bytes.Buffer{}); err == nil {
		t.Fatal("symlink corpus path must be rejected")
	}
	realOut := t.TempDir()
	outLink := filepath.Join(t.TempDir(), "pack-link")
	if err := os.Symlink(realOut, outLink); err != nil {
		t.Fatal(err)
	}
	if err := runPack([]string{"-corpus", realCorpus, "-out", outLink}, &bytes.Buffer{}); err == nil {
		t.Fatal("symlink output path must be rejected")
	}
	outside := t.TempDir()
	linkedParent := filepath.Join(t.TempDir(), "linked-parent")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatal(err)
	}
	if err := runPack([]string{"-corpus", realCorpus, "-out", filepath.Join(linkedParent, "pack")}, &bytes.Buffer{}); err == nil {
		t.Fatal("symlink in an output parent component must be rejected")
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

func TestRunIngestBaseAllocatesPastBaseMax(t *testing.T) {
	dir, base := corpusWithLocal2758AndBase2768(t)
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-base", base, "-limit", "1"}, &out, noEnv)
	if err != nil {
		t.Fatalf("ingest go: %v", err)
	}
	if !strings.Contains(out.String(), "created exp-2769") {
		t.Fatalf("ingest with -base must allocate past base max; output:\n%s", out.String())
	}
}

// -open-prs must thread the Forgejo open-PR scan into allocation end-to-end:
// the stub's one open PR carries exp-3197, well above the local (2758) and
// base (2768) high-water marks, so the batch must start at exp-3198.
func TestRunIngestOpenPRsAllocatesPastOpenPRMax(t *testing.T) {
	dir, base := corpusWithLocal2758AndBase2768(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token stubtok" {
			t.Errorf("request %s lacks the origin-derived token: Authorization = %q", r.URL, got)
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls") && page == "1":
			_, _ = w.Write([]byte(`[{"number":5}]`))
		case strings.HasSuffix(r.URL.Path, "/pulls/5/files") && page == "1":
			_, _ = w.Write([]byte(`[{"filename":"experience/2026/3197-open-pr-draft.md"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	origin := strings.Replace(srv.URL, "http://", "http://claude:stubtok@", 1) + "/claude/twiceshy-corpus.git"
	gitCmd(t, dir, "remote", "add", "origin", origin)

	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-base", base, "-open-prs", "-limit", "1"}, &out, noEnv)
	if err != nil {
		t.Fatalf("ingest go -open-prs: %v", err)
	}
	if !strings.Contains(out.String(), "created exp-3198") {
		t.Fatalf("ingest with -open-prs must allocate past the open-PR max; output:\n%s", out.String())
	}
}

// A dry run writes nothing, so -open-prs must not touch the network: the
// corpus here has no git origin at all, and an attempted scan would error.
func TestRunIngestDryRunSkipsOpenPRScan(t *testing.T) {
	dir := tempCorpus(t)
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db"), "-open-prs", "-dry-run"}, &out, noEnv)
	if err != nil {
		t.Fatalf("dry-run with -open-prs must skip the scan, got: %v", err)
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

// TestRunServeFailsFastOnInvalidTrustedProxies is #0131 finding 3's config
// side: TWICESHY_TRUSTED_PROXIES must fail startup on a typo rather than
// silently trust nothing (or, worse, the wrong network) at runtime.
func TestRunServeFailsFastOnInvalidTrustedProxies(t *testing.T) {
	var out lockedBuffer
	err := run(context.Background(), []string{
		"serve", "-corpus", corpus,
		"-db", filepath.Join(t.TempDir(), "ix.db"),
		"-addr", "127.0.0.1:0",
	}, &out, func(k string) string {
		switch k {
		case "TWICESHY_TOKEN":
			return "test-token"
		case "TWICESHY_TRUSTED_PROXIES":
			return "not-a-cidr"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected an error for an invalid TWICESHY_TRUSTED_PROXIES entry")
	}
	if !strings.Contains(err.Error(), "TWICESHY_TRUSTED_PROXIES") {
		t.Fatalf("error = %q, want it to name TWICESHY_TRUSTED_PROXIES", err.Error())
	}
}

// SIGHUP hot-reloads the corpus in place (#0060): serve starts on an empty
// corpus (/readyz 503), a record is written to the corpus dir, and a SIGHUP
// makes serve rebuild its index without a restart — /readyz flips to ready. This
// is what lets the corpus-sync timer signal instead of `docker restart`.
func TestRunServeReloadsCorpusOnSIGHUP(t *testing.T) {
	dir := tempCorpus(t) // empty corpus: serves nothing until a reload
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{
			"serve", "-corpus", dir,
			"-db", filepath.Join(t.TempDir(), "ix.db"),
			"-addr", "127.0.0.1:0",
		}, &out, func(k string) string {
			if k == "TWICESHY_TOKEN" {
				return "test-token"
			}
			return ""
		})
	}()

	// Wait until serve reports its address — by then the SIGHUP handler is
	// registered, so the signal we send below can't terminate the process.
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

	readyz := func() int {
		resp, err := http.Get("http://" + addr + "/readyz") // unauthenticated probe
		if err != nil {
			t.Fatalf("GET /readyz: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if code := readyz(); code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz on empty corpus = %d, want 503", code)
	}

	// Drop a validated record into the live corpus and signal a reload.
	writeFixture(t, dir, packFixture("0301", "validated", "MIT", ""))
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP: %v", err)
	}

	deadline = time.Now().Add(10 * time.Second)
	for readyz() != http.StatusOK {
		if time.Now().After(deadline) {
			t.Fatalf("/readyz never became ready after SIGHUP; output: %q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
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

// A SIGHUP whose reload FAILS must never blip the service: serve keeps the prior
// good index (loadAndRebuild's walk errors, ix.Rebuild is not run / rolls back),
// fires the "serve-reload-failed" alert, and stays ready (main.go:433-438). This is
// the operationally critical sibling of the happy-reload test above — a corpus-sync
// that ships a broken corpus must not drop the listener or serve an empty index.
//
// The alert is observed through the real notify seam: runServe builds its alerter
// from getenv("TWICESHY_ALERT_URL") and POSTs there (notify.HTTPNotifier), so an
// httptest sink that captures the POST is the recordingAlerter — no product seam
// needed. The failure is forced by making experience/ unreadable (chmod 0) so
// walkCorpusSkipping returns a directory-walk error rather than a skipped record (a
// poison record is tolerated, not fatal). /readyz staying 200 (unauthenticated, as
// in the happy-path test) is the proxy for "still serving the prior good index".
func TestRunServeKeepsServingWhenSIGHUPReloadFails(t *testing.T) {
	// The reload failure is injected by chmod 0 on the corpus dir, which root
	// bypasses — so this case is a no-op under root (the CI gVisor sandbox runs as
	// uid 0). Skip there rather than hang waiting for an alert that can't fire.
	if os.Geteuid() == 0 {
		t.Skip("chmod-based reload-failure injection is a no-op as root")
	}
	dir := tempCorpus(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// recordingAlerter: capture which alert events were POSTed.
	var (
		alertMu sync.Mutex
		alerts  []string
	)
	alertSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertMu.Lock()
		alerts = append(alerts, r.Header.Get("Title")) // "twiceshy: <event>"
		alertMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer alertSrv.Close()
	sawAlert := func(event string) bool {
		alertMu.Lock()
		defer alertMu.Unlock()
		for _, a := range alerts {
			if a == "twiceshy: "+event {
				return true
			}
		}
		return false
	}

	var out lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{
			"serve", "-corpus", dir,
			"-db", filepath.Join(t.TempDir(), "ix.db"),
			"-addr", "127.0.0.1:0",
		}, &out, func(k string) string {
			switch k {
			case "TWICESHY_TOKEN":
				return "test-token"
			case "TWICESHY_ALERT_URL":
				return alertSrv.URL
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

	readyz := func() int {
		resp, err := http.Get("http://" + addr + "/readyz")
		if err != nil {
			t.Fatalf("GET /readyz: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// Load a good corpus first so there is a prior good index to keep serving.
	writeFixture(t, dir, packFixture("0301", "validated", "MIT", ""))
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP: %v", err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for readyz() != http.StatusOK {
		if time.Now().After(deadline) {
			t.Fatalf("/readyz never became ready after the good reload; output: %q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Now make the corpus walk fail: an unreadable experience/ dir makes
	// filepath.WalkDir surface a permission error, so loadAndRebuild errors (vs a
	// poison FILE, which is skipped, not fatal). Restore perms so TempDir teardown
	// works regardless of test outcome.
	expDir := filepath.Join(dir, "experience")
	if err := os.Chmod(expDir, 0); err != nil {
		t.Fatalf("chmod experience/ unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(expDir, 0o755) })

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP (failing reload): %v", err)
	}

	// A failed reload is invisible on the HTTP surface (readyz stays 200 either
	// way), so synchronize on the alert POST before asserting — that is the only
	// signal the failure branch actually ran.
	deadline = time.Now().Add(10 * time.Second)
	for !sawAlert("serve-reload-failed") {
		if time.Now().After(deadline) {
			t.Fatalf("serve-reload-failed alert never fired; output: %q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The failure branch ran; the service must still be serving the prior index.
	if code := readyz(); code != http.StatusOK {
		t.Fatalf("/readyz after a FAILED reload = %d, want 200 (still serving prior corpus)", code)
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

// TestRunEvalPush asserts 100% push precision AND recall, both corpus-relative, so
// it needs the live corpus (now twiceshy-corpus, ADR-0021) and lives in
// eval_push_livecorpus_test.go behind the livecorpus tag.

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

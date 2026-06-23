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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
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

// eval -push runs the push-precision eval over the real corpus: off-domain
// prompts must inject nothing (precision) and genuine traps must surface (recall).
// End-to-end guard for the CLI path the push channel is gated on.
func TestRunEvalPush(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{
		"eval", "-push", "-corpus", "../..", "-db", filepath.Join(t.TempDir(), "pe.db"),
	}, &out, noEnv)
	if err != nil {
		t.Fatalf("eval -push: %v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "precision: 100.0%") {
		t.Errorf("want precision 100%%, got:\n%s", s)
	}
	if !strings.Contains(s, "recall:    100.0%") {
		t.Errorf("want recall 100%%, got:\n%s", s)
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

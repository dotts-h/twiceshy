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
	if !strings.Contains(out.String(), "indexed 3 records") {
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

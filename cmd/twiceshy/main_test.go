// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
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

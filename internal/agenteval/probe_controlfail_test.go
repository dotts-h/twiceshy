// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

// Live investigation probe for #0144 acceptance-1 (control-fail). NOT a CI test:
// it needs a live drafter model + the docker/runsc broker, so it self-skips unless
// PROBE_CONTROLFAIL=1. Run:
//   PROBE_CONTROLFAIL=1 TWICESHY_AGENTEVAL_URL=http://192.168.50.150:11434/v1/chat/completions \
//   TWICESHY_AGENTEVAL_MODEL=qwen2.5-coder:14b go test ./internal/agenteval/ \
//   -run TestProbe_ControlFail -v -timeout 20m -count=1
//
// For each record id in PROBE_IDS (comma-sep, default a set of 0140 control-fail
// records), it drafts the task, then runs the DRAFTED CONTROL through the same
// broker job Verifier.Avoided uses — and prints the drafted control code, the
// prepare/execute exit codes, and the stderr. That output is the raw material for
// categorizing WHY controls fail (trap actually bit vs unrelated compile error vs
// bad drafter control).

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

func TestProbe_ControlFail(t *testing.T) {
	if os.Getenv("PROBE_CONTROLFAIL") == "" {
		t.Skip("live probe; set PROBE_CONTROLFAIL=1 (needs drafter model + docker/runsc)")
	}
	corpus := os.Getenv("PROBE_CORPUS")
	if corpus == "" {
		corpus = "/home/ori/twiceshy-corpus"
	}
	ids := strings.Split(os.Getenv("PROBE_IDS"), ",")
	if len(ids) == 1 && ids[0] == "" {
		ids = []string{"exp-2868"}
	}

	all, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	byID := map[string]*record.Record{}
	for _, r := range all {
		byID[r.ID] = r
	}

	drafter, err := NewModelTaskDrafter(DrafterConfig{
		Endpoint: os.Getenv("TWICESHY_AGENTEVAL_URL"),
		Model:    os.Getenv("TWICESHY_AGENTEVAL_MODEL"),
	})
	if err != nil {
		t.Fatalf("drafter: %v", err)
	}
	broker := repro.NewBroker(VerifierImages())
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	if err := broker.Healthy(ctx); err != nil {
		t.Fatalf("broker unhealthy: %v", err)
	}
	v := NewBrokerVerifier(broker)

	for _, id := range ids {
		rec := byID[strings.TrimSpace(id)]
		if rec == nil {
			t.Logf("=== %s: NOT IN CORPUS ===", id)
			continue
		}
		tc, err := drafter.DraftTask(ctx, rec)
		if err != nil {
			t.Logf("=== %s: DRAFT ERROR: %v ===", id, err)
			continue
		}
		code := extractCode(tc.Control)
		job, err := v.buildJob(tc.VerifyID, tc.Deps, code)
		if err != nil {
			t.Logf("=== %s: buildJob error: %v ===", id, err)
			continue
		}
		res, err := broker.Run(ctx, job)
		if err != nil {
			t.Logf("=== %s: broker.Run error: %v ===", id, err)
			continue
		}
		t.Logf("\n==================== %s (VerifyID=%s deps=%v) ====================", id, tc.VerifyID, tc.Deps)
		t.Logf("PROMPT:\n%s", tc.Prompt)
		t.Logf("CONTROL (extracted code):\n%s", code)
		t.Logf("PREPARE: exit=%d stderr=%s", res.Prepare.ExitCode, tail(res.Prepare.Stderr, 600))
		t.Logf("EXECUTE: exit=%d avoided=%v", res.Execute.ExitCode, res.Execute.ExitCode == 0)
		t.Logf("EXECUTE stderr:\n%s", tail(res.Execute.Stderr, 900))
		t.Logf("EXECUTE stdout:\n%s", tail(res.Execute.Stdout, 400))
	}
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// TestProbe_RelevanceGuardCalibration validates the #0144 relevance guard against the
// REAL corpus records and the prompts the live drafter actually produced for them
// (captured by TestProbe_ControlFail). It self-skips when the live corpus isn't present
// (CI), so it never gates CI — it is the "verify on real signal" check for the guard's
// calibration: fabricated drafts (exp-2861/exp-4231) void, faithful ones keep.
func TestProbe_RelevanceGuardCalibration(t *testing.T) {
	corpus := os.Getenv("PROBE_CORPUS")
	if corpus == "" {
		corpus = "/home/ori/twiceshy-corpus"
	}
	if _, err := os.Stat(corpus); err != nil {
		t.Skipf("live corpus not present (%v)", err)
	}
	all, err := record.LoadCorpus(corpus)
	if err != nil {
		t.Skipf("load corpus: %v", err)
	}
	byID := map[string]*record.Record{}
	for _, r := range all {
		byID[r.ID] = r
	}
	// The prompt each is the verbatim drafter output observed in the live probe.
	cases := []struct {
		id, prompt string
		wantKeep   bool
	}{
		{"exp-2861", "Write a function in TypeScript that takes two strings as input and returns the concatenated result.", false},
		{"exp-4231", "Write a Go program that imports the 'fmt' and 'math/rand' packages and prints a random integer between 1 and 100.", false},
		{"exp-2868", "Upgrade a TypeScript project to React 19 and update the necessary types. You need to fix any type errors in your code due to changes in the 'useRef' hook.", true},
	}
	for _, c := range cases {
		rec := byID[c.id]
		if rec == nil {
			t.Skipf("%s not in corpus", c.id)
		}
		if got := draftRelevantToRecord(c.prompt, rec); got != c.wantKeep {
			t.Errorf("%s: draftRelevantToRecord = %v, want %v (record terms=%v)", c.id, got, c.wantKeep, recordDistinctiveTerms(rec))
		}
	}
}

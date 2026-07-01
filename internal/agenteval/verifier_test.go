// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/repro"
)

// stubBroker captures the Job it was handed and returns a canned exit code, so the
// verifier's job-construction and exit-code→avoided logic are tested WITHOUT a real
// gVisor sandbox (the integration path runs on the brain, never in CI).
type stubBroker struct {
	gotJob repro.Job
	exit   int
	err    error
}

func (s *stubBroker) Run(_ context.Context, j repro.Job) (repro.Result, error) {
	s.gotJob = j
	if s.err != nil {
		return repro.Result{}, s.err
	}
	return repro.Result{Execute: repro.PhaseResult{ExitCode: s.exit}}, nil
}
func (s *stubBroker) Healthy(_ context.Context) error { return nil }

// The avoidance verdict is "the scaffolded output passes the toolchain": exit 0 = the
// trap was avoided, non-zero = it bit. This pins that mapping and that the Go/FTS5 case
// scaffolds the EXTRACTED code into a Go project the broker compiles.
func TestBrokerVerifier_FTS5_GoJobAndExitCodeMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		exit        int
		wantAvoided bool
	}{
		{"compiles → avoided", 0, true},
		{"build error → hit the trap", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			br := &stubBroker{exit: tc.exit}
			v := NewBrokerVerifier(br)
			c := TaskCase{TrapID: "exp-0001", VerifyID: "fts5-match"}
			got, err := v.Avoided(context.Background(), c, "```go\npackage main\nfunc main() {}\n```")
			if err != nil {
				t.Fatalf("Avoided: %v", err)
			}
			if got != tc.wantAvoided {
				t.Errorf("avoided = %v, want %v (exit %d)", got, tc.wantAvoided, tc.exit)
			}
			if !strings.Contains(strings.ToLower(br.gotJob.Image), "golang") {
				t.Errorf("fts5 job image = %q, want the pinned Go image", br.gotJob.Image)
			}
			main, ok := br.gotJob.Files["main.go"]
			if !ok {
				t.Fatalf("fts5 job must scaffold main.go; files = %v", keys(br.gotJob.Files))
			}
			if strings.Contains(string(main), "```") {
				t.Error("scaffolded main.go must be the EXTRACTED code, not the fenced output")
			}
			if !strings.Contains(strings.Join(br.gotJob.Execute, " "), "go build") {
				t.Errorf("fts5 job Execute = %v, want a 'go build'", br.gotJob.Execute)
			}
		})
	}
}

// The two TS traps scaffold a .tsx file and type-check it with tsc in the Node image —
// the trap is a type error (TS2554 / TS2769), so tsc exit 0 = avoided.
func TestBrokerVerifier_TS_NodeTscJob(t *testing.T) {
	for _, vid := range []string{"react19-useref", "rn-viewstyle"} {
		t.Run(vid, func(t *testing.T) {
			br := &stubBroker{exit: 0}
			v := NewBrokerVerifier(br)
			c := TaskCase{VerifyID: vid}
			if _, err := v.Avoided(context.Background(), c, "const x: number = 1"); err != nil {
				t.Fatalf("Avoided: %v", err)
			}
			if !strings.Contains(strings.ToLower(br.gotJob.Image), "node") {
				t.Errorf("%s job image = %q, want the pinned Node image", vid, br.gotJob.Image)
			}
			if !hasFileWithSuffix(br.gotJob.Files, ".tsx") {
				t.Errorf("%s job must scaffold a .tsx file; files = %v", vid, keys(br.gotJob.Files))
			}
			if !strings.Contains(strings.Join(br.gotJob.Execute, " "), "tsc") {
				t.Errorf("%s job Execute = %v, want a tsc type-check", vid, br.gotJob.Execute)
			}
		})
	}
}

func TestBrokerVerifier_UnknownVerifyID(t *testing.T) {
	v := NewBrokerVerifier(&stubBroker{})
	if _, err := v.Avoided(context.Background(), TaskCase{VerifyID: "nope"}, "x"); err == nil {
		t.Error("an unknown VerifyID must error, not silently pass")
	}
}

// The "gobuild" verify class is the generic compile-only Go check the prospector
// uses (mirroring fts5-match's job shape, but for any Go trap, not just FTS5).
func TestBuildJob_GoBuild(t *testing.T) {
	v := NewBrokerVerifier(&stubBroker{})
	job, err := v.buildJob("gobuild", nil, "package main\nfunc main() {}\n")
	if err != nil {
		t.Fatalf("buildJob: %v", err)
	}
	if !strings.Contains(strings.ToLower(job.Image), "golang") {
		t.Errorf("gobuild job image = %q, want the pinned Go image", job.Image)
	}
	if _, ok := job.Files["main.go"]; !ok {
		t.Fatalf("gobuild job must scaffold main.go; files = %v", keys(job.Files))
	}
	if !strings.Contains(strings.Join(job.Execute, " "), "go build") {
		t.Errorf("gobuild job Execute = %v, want a 'go build'", job.Execute)
	}
}

// The generic "tsc" verify class type-checks with the caller-supplied Deps (the
// prospector's drafted npm packages), unlike the literal react19-useref/rn-viewstyle
// classes which carry their own hardcoded deps.
func TestBuildJob_Tsc(t *testing.T) {
	v := NewBrokerVerifier(&stubBroker{})
	job, err := v.buildJob("tsc", []string{"typescript", "@types/react@19"}, "const x: number = 1")
	if err != nil {
		t.Fatalf("buildJob: %v", err)
	}
	if !strings.Contains(strings.ToLower(job.Image), "node") {
		t.Errorf("tsc job image = %q, want the pinned Node image", job.Image)
	}
	if !hasFileWithSuffix(job.Files, ".tsx") {
		t.Errorf("tsc job must scaffold a .tsx file; files = %v", keys(job.Files))
	}
	prepare := strings.Join(job.Prepare, " ")
	if !strings.Contains(prepare, "typescript") || !strings.Contains(prepare, "@types/react@19") {
		t.Errorf("tsc job Prepare = %q, want it to install the given Deps", prepare)
	}
}

// tsc has no fixed deps of its own (unlike react19-useref/rn-viewstyle) — an empty
// Deps means the drafter/caller forgot to supply the npm packages the code needs,
// so this must error rather than silently run tsc against nothing installed.
func TestBuildJob_TscRequiresDeps(t *testing.T) {
	v := NewBrokerVerifier(&stubBroker{})
	if _, err := v.buildJob("tsc", nil, "const x = 1"); err == nil {
		t.Error("buildJob(\"tsc\", nil, ...) must error when Deps is empty")
	}
}

func TestExtractCode(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"go fence", "```go\npackage main\n```", "package main"},
		{"tsx fence", "```tsx\nconst x = 1\n```", "const x = 1"},
		{"bare fence", "```\nraw\n```", "raw"},
		{"prose around fence", "Here you go:\n```go\ncode\n```\nHope that helps!", "code"},
		{"no fence is returned trimmed", "  const x = 1  ", "const x = 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.TrimSpace(extractCode(tc.in)); got != tc.want {
				t.Errorf("extractCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func hasFileWithSuffix(m map[string][]byte, suffix string) bool {
	for k := range m {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

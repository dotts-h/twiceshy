package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNtfyAuthorization(t *testing.T) {
	scripts := []string{
		"scheduled-import.sh",
		"scheduled-validate.sh",
		"scheduled-retro.sh",
		"corpus-stall-alarm.sh",
		"sync-corpus-to-nas.sh",
		"twiceshy-watchdog.sh",
		"twiceshy-growth-watchdog.sh",
	}
	for _, script := range scripts {
		for _, token := range []string{"", "matrix-token"} {
			name := "unset"
			if token != "" {
				name = "set"
			}
			t.Run(script+"/"+name, func(t *testing.T) {
				h := newShellHarness(t)
				alertURL := "http://ntfy.invalid/ops"
				configureAlertPath(h, script, alertURL)
				if token != "" {
					h.set("NTFY_TOKEN", token)
				}
				result := h.run(script)
				requireSingleAlert(t, result, alertURL, token)
			})
		}
	}
}

func TestCorpusStallAlarmReportsOpenRedPR(t *testing.T) {
	h := newShellHarness(t)
	alertURL := "http://ntfy.invalid/stall"
	h.fake("python3", "echo '77|validate/run-red|3|failure'")
	h.set("FORGEJO_TOKEN", "forge-token")
	h.set("TWICESHY_ALERT_URL", alertURL)
	h.set("NTFY_TOKEN", "stall-token")
	h.set("TWICESHY_STALL_STATE", filepath.Join(h.root, "stall.state"))
	h.set("TWICESHY_STALL_COOLDOWN", "0")

	result := h.run("corpus-stall-alarm.sh")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	call := requireSingleAlert(t, result, alertURL, "stall-token")
	if !containsArg(call.Args, "corpus-stall-alarm: corpus pipeline STALLED — 1 pipeline PR(s) open past 120m or red; the corpus may be frozen. Investigate + drain:\n#77 validate/run-red (age 3m, ci=failure)") {
		t.Errorf("notification does not name PR #77: %q", call.Args)
	}
}

func TestScheduledValidateMergeDue(t *testing.T) {
	h := newShellHarness(t)
	configureCommonGit(h)
	now := time.Now().Unix()
	h.fake("curl", `
case "$*" in
  *"/pulls?state=open"*) echo '[fixture pull JSON]' ;;
esac`)
	h.fake("jq", fmt.Sprintf(`cat <<'EOF'
1 validate/young sha-young %d false
2 validate/eligible sha-eligible %d false
3 validate/anomaly sha-anomaly %d true
4 import/not-validate sha-import %d false
5 validate/merge-fails sha-fail %d false
EOF`, now-10, now-1000, now-1000, now-1000, now-1000))
	h.fake("forgejo-ci-merge", `
[ "${2:-}" != "5" ]`)
	bin := h.fake("twiceshy", ":")
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_JUDGE_URL", "http://judge.invalid")
	h.set("TWICESHY_SOAK_SECONDS", "100")
	h.set("NTFY_URL", "http://ntfy.invalid/validate")

	result := h.run("scheduled-validate.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	mergeCalls := result.Invocations["forgejo-ci-merge"]
	if len(mergeCalls) != 2 || !containsArg(mergeCalls[0].Args, "2") || !containsArg(mergeCalls[1].Args, "5") {
		t.Fatalf("merge calls = %s, want PRs 2 and 5 only", formatCalls(mergeCalls))
	}
	if !strings.Contains(result.Stdout, "PR #3") || !strings.Contains(result.Stdout, "ANOMALY") {
		t.Errorf("anomalous PR was not visibly skipped; stdout: %s", result.Stdout)
	}
	alerts := result.callsTo("curl", "http://ntfy.invalid/validate")
	foundFailure := false
	for _, call := range alerts {
		if containsArg(call.Args, "twiceshy validate: PR #5 (validate/merge-fails) merge failed after soak") {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("merge failure notification missing; calls: %s", formatCalls(alerts))
	}
}

// A "never silent again" alarm must not itself be silently mis-wired: ntfy requires a
// topic path (https://host/<topic>); a bare-host URL 400s and the alert is dropped. So
// the alarm warns LOUDLY (stderr) when its resolved ALERT_URL has no topic. This is the
// live #0093 defect — a bare TWICESHY_ALERT_URL in stall-alarm.env shadowed the topic-
// qualified NTFY_URL from the ntfy.env drop-in, so every stall alert 400'd in silence.
func TestCorpusStallAlarmWarnsOnTopiclessURL(t *testing.T) {
	for _, tc := range []struct {
		name     string
		alertURL string
		wantWarn bool
	}{
		{name: "bare host warns", alertURL: "http://ntfy.invalid", wantWarn: true},
		{name: "trailing slash empty topic warns", alertURL: "http://ntfy.invalid/", wantWarn: true},
		{name: "topic-qualified is silent", alertURL: "http://ntfy.invalid/infra", wantWarn: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newShellHarness(t)
			h.fake("python3", "echo '77|validate/run-red|3|failure'")
			h.set("FORGEJO_TOKEN", "forge-token")
			h.set("TWICESHY_ALERT_URL", tc.alertURL)
			h.set("NTFY_TOKEN", "stall-token")
			h.set("TWICESHY_STALL_STATE", filepath.Join(h.root, "stall.state"))
			h.set("TWICESHY_STALL_COOLDOWN", "0")

			result := h.run("corpus-stall-alarm.sh")
			gotWarn := strings.Contains(result.Stderr, "has no ntfy topic")
			if gotWarn != tc.wantWarn {
				t.Fatalf("warn=%v, want %v for ALERT_URL %q; stderr: %q", gotWarn, tc.wantWarn, tc.alertURL, result.Stderr)
			}
		})
	}
}

// TWICESHY_IMPORT_AUTHOR (#0115/#0118, ADR-0028 push-eligibility origin cut on
// provenance.source.author) is optional provenance forwarded to every `twiceshy
// ingest` invocation as `-author "$AUTHOR"`. Left unset, the ingest argv must be
// byte-for-byte unchanged (no -author flag at all) — both on the generic
// single-source path and the osv-live per-ecosystem loop.
func TestScheduledImportAuthorFlag(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		author     string
		wantAuthor []string // nil = no -author flag anywhere in the ingest argv
	}{
		{
			name:       "generic source with TWICESHY_IMPORT_AUTHOR appends -author",
			source:     "node-breaking",
			author:     "node-breaking",
			wantAuthor: []string{"-author", "node-breaking"},
		},
		{
			name:       "generic source with a different author value still appends it verbatim",
			source:     "fixture",
			author:     "quill-reviewer",
			wantAuthor: []string{"-author", "quill-reviewer"},
		},
		{
			name:       "generic source with TWICESHY_IMPORT_AUTHOR unset leaves argv unchanged",
			source:     "fixture",
			author:     "",
			wantAuthor: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newShellHarness(t)
			configureCommonGit(h)
			bin := h.fake("twiceshy", ":")
			h.set("TWICESHY_REPO", h.root)
			h.set("TWICESHY_BIN", bin)
			h.set("TWICESHY_IMPORT_SOURCE", tc.source)
			if tc.author != "" {
				h.set("TWICESHY_IMPORT_AUTHOR", tc.author)
			}

			result := h.run("scheduled-import.sh")
			calls := result.Invocations["twiceshy"]
			if len(calls) != 1 {
				t.Fatalf("twiceshy invocations = %d, want 1; stdout: %s stderr: %s", len(calls), result.Stdout, result.Stderr)
			}
			assertAuthorArgs(t, calls[0].Args, tc.wantAuthor)
		})
	}

	t.Run("osv-live per-ecosystem loop appends -author to every ecosystem call", func(t *testing.T) {
		h := newShellHarness(t)
		configureCommonGit(h)
		bin := h.fake("twiceshy", ":")
		h.set("TWICESHY_REPO", h.root)
		h.set("TWICESHY_BIN", bin)
		h.set("TWICESHY_IMPORT_SOURCE", "osv-live")
		h.set("TWICESHY_IMPORT_ECOSYSTEMS", "npm PyPI")
		h.set("TWICESHY_IMPORT_AUTHOR", "node-breaking")

		result := h.run("scheduled-import.sh")
		calls := result.Invocations["twiceshy"]
		if len(calls) != 2 {
			t.Fatalf("twiceshy invocations = %d, want 2 (one per ecosystem); stdout: %s stderr: %s", len(calls), result.Stdout, result.Stderr)
		}
		for _, call := range calls {
			assertAuthorArgs(t, call.Args, []string{"-author", "node-breaking"})
		}
	})

	t.Run("osv-live per-ecosystem loop omits -author when TWICESHY_IMPORT_AUTHOR is unset", func(t *testing.T) {
		h := newShellHarness(t)
		configureCommonGit(h)
		bin := h.fake("twiceshy", ":")
		h.set("TWICESHY_REPO", h.root)
		h.set("TWICESHY_BIN", bin)
		h.set("TWICESHY_IMPORT_SOURCE", "osv-live")
		h.set("TWICESHY_IMPORT_ECOSYSTEMS", "npm PyPI")

		result := h.run("scheduled-import.sh")
		calls := result.Invocations["twiceshy"]
		if len(calls) != 2 {
			t.Fatalf("twiceshy invocations = %d, want 2 (one per ecosystem); stdout: %s stderr: %s", len(calls), result.Stdout, result.Stderr)
		}
		for _, call := range calls {
			assertAuthorArgs(t, call.Args, nil)
		}
	})
}

// assertAuthorArgs checks that -author immediately followed by its value appears
// (or, when want is nil, does NOT appear at all) in an ingest invocation's argv.
func assertAuthorArgs(t *testing.T, args []string, want []string) {
	t.Helper()
	if want == nil {
		if containsArg(args, "-author") {
			t.Errorf("argv unexpectedly contains -author: %q", args)
		}
		return
	}
	for i, arg := range args {
		if arg == want[0] && i+1 < len(args) && args[i+1] == want[1] {
			return
		}
	}
	t.Errorf("argv %q does not contain %q immediately followed by %q", args, want[0], want[1])
}

func TestScheduledImportMergeFailureIsVisible(t *testing.T) {
	h := newShellHarness(t)
	h.fake("git", `
case "$*" in
  *"status --porcelain -- experience/"*) echo '?? experience/record.md' ;;
  *"config --get remote.origin.url"*) echo 'http://user:forge-token@forge/owner/repo' ;;
  *"rev-parse --verify"*) exit 1 ;;
  *"rev-parse HEAD"*) echo 'head-sha' ;;
esac`)
	h.fake("curl", `case "$*" in *"/pulls"*) echo '{"number":42}' ;; esac`)
	h.fake("jq", `case "$*" in *"-nc"*) echo '{}' ;; *) echo 42 ;; esac`)
	h.fake("forgejo-ci-merge", "exit 1")
	bin := h.fake("twiceshy", ":")
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_IMPORT_SOURCE", "fixture")
	h.set("NTFY_URL", "http://ntfy.invalid/import")

	result := h.run("scheduled-import.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if len(result.Invocations["forgejo-ci-merge"]) != 1 {
		t.Fatalf("merge calls = %s, want one", formatCalls(result.Invocations["forgejo-ci-merge"]))
	}
	alerts := result.callsTo("curl", "http://ntfy.invalid/import")
	if len(alerts) != 1 || !containsArg(alerts[0].Args, "twiceshy: import PR #42 (1 fixture records) left OPEN — auto-merge refused (CI red or timeout); needs attention") {
		t.Fatalf("merge failure was not observable in notification; calls: %s", formatCalls(alerts))
	}
}

// TestScheduledImportPinsForgejoCIMinRunsToOneForCorpusRepo covers the
// scheduled-import.sh repo-derived FORGEJO_CI_MIN_RUNS export: when
// TWICESHY_FORGEJO_REPO targets the corpus repo (claude/twiceshy-corpus),
// which has exactly ONE workflow, the script must export FORGEJO_CI_MIN_RUNS=1
// before forgejo-ci-merge runs. Without this, forgejo-ci-merge falls back to its
// historical default of 3 terminal runs (see TestForgejoCIMergeMinRuns) and every
// corpus import PR times out unmerged because a 1-workflow repo can never produce
// 3 terminal runs.
func TestScheduledImportPinsForgejoCIMinRunsToOneForCorpusRepo(t *testing.T) {
	h := newShellHarness(t)
	h.fake("git", `
case "$*" in
  *"status --porcelain -- experience/"*) echo '?? experience/record.md' ;;
  *"config --get remote.origin.url"*) echo 'http://user:forge-token@forge/owner/repo' ;;
  *"rev-parse --verify"*) exit 1 ;;
  *"rev-parse HEAD"*) echo 'head-sha' ;;
esac`)
	h.fake("curl", `case "$*" in *"/pulls"*) echo '{"number":42}' ;; esac`)
	h.fake("jq", `case "$*" in *"-nc"*) echo '{}' ;; *) echo 42 ;; esac`)
	minRunsSeen := filepath.Join(h.root, "min_runs_seen")
	h.fake("forgejo-ci-merge", fmt.Sprintf(`printf '%%s' "${FORGEJO_CI_MIN_RUNS:-unset}" > %q; exit 0`, minRunsSeen))
	bin := h.fake("twiceshy", ":")
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_IMPORT_SOURCE", "fixture")
	h.set("TWICESHY_FORGEJO_REPO", "claude/twiceshy-corpus")

	result := h.run("scheduled-import.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if len(result.Invocations["forgejo-ci-merge"]) != 1 {
		t.Fatalf("forgejo-ci-merge calls = %s, want exactly one", formatCalls(result.Invocations["forgejo-ci-merge"]))
	}

	seen, err := os.ReadFile(minRunsSeen)
	if err != nil {
		t.Fatalf("reading recorded FORGEJO_CI_MIN_RUNS: %v", err)
	}
	if got := string(seen); got != "1" {
		t.Errorf("FORGEJO_CI_MIN_RUNS observed by forgejo-ci-merge = %q, want %q (corpus repo has exactly 1 workflow)", got, "1")
	}

	// A second, differently-shaped assertion so this test can't be satisfied by
	// a fake that just always writes "1" regardless of what actually ran: the
	// rest of the merge flow (PR number resolution, success notification text)
	// must still complete normally once forgejo-ci-merge reports success.
	wantDone := "done: 1 records, PR #42"
	if !strings.Contains(result.Stdout, wantDone) {
		t.Errorf("stdout = %q, want it to contain %q", result.Stdout, wantDone)
	}
}

func TestScheduledRetroPinsForgejoCIMinRunsToOneForCorpusRepo(t *testing.T) {
	h := newShellHarness(t)
	configureCommonGit(h)
	h.fake("git", `
case "$*" in
  *"status --porcelain -- experience/"*) echo '?? experience/record.md' ;;
  *"config --get remote.origin.url"*) echo 'http://user:forge-token@forge/owner/repo' ;;
  *"rev-parse --verify"*) exit 1 ;;
  *"rev-parse HEAD"*) echo 'head-sha' ;;
esac`)
	h.fake("curl", `case "$*" in *"/pulls"*) echo '{"number":42}' ;; esac`)
	h.fake("jq", `case "$*" in *"-nc"*) echo '{}' ;; *) echo 42 ;; esac`)
	minRunsSeen := filepath.Join(h.root, "min_runs_seen")
	h.fake("forgejo-ci-merge", fmt.Sprintf(`printf '%%s' "${FORGEJO_CI_MIN_RUNS:-unset}" > %q; exit 0`, minRunsSeen))
	bin := h.fake("twiceshy", `echo 'retro ok'`)
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_RETRO_URL", "http://retro.invalid")
	h.set("TWICESHY_FORGEJO_REPO", "claude/twiceshy-corpus")

	result := h.run("scheduled-retro.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if len(result.Invocations["forgejo-ci-merge"]) != 1 {
		t.Fatalf("forgejo-ci-merge calls = %s, want exactly one", formatCalls(result.Invocations["forgejo-ci-merge"]))
	}

	seen, err := os.ReadFile(minRunsSeen)
	if err != nil {
		t.Fatalf("reading recorded FORGEJO_CI_MIN_RUNS: %v", err)
	}
	if got := string(seen); got != "1" {
		t.Errorf("FORGEJO_CI_MIN_RUNS observed by forgejo-ci-merge = %q, want %q", got, "1")
	}
}

func TestScheduledValidateMergeDuePinsForgejoCIMinRunsToOneForCorpusRepo(t *testing.T) {
	h := newShellHarness(t)
	configureCommonGit(h)
	now := time.Now().Unix()
	h.fake("curl", `
case "$*" in
  *"/pulls?state=open"*) echo '[fixture pull JSON]' ;;
esac`)
	h.fake("jq", fmt.Sprintf(`cat <<'EOF'
2 validate/eligible sha-eligible %d false
EOF`, now-1000))
	minRunsSeen := filepath.Join(h.root, "min_runs_seen")
	h.fake("forgejo-ci-merge", fmt.Sprintf(`printf '%%s' "${FORGEJO_CI_MIN_RUNS:-unset}" > %q; exit 0`, minRunsSeen))
	bin := h.fake("twiceshy", ":")
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_JUDGE_URL", "http://judge.invalid")
	h.set("TWICESHY_SOAK_SECONDS", "100")
	h.set("TWICESHY_FORGEJO_REPO", "claude/twiceshy-corpus")

	result := h.run("scheduled-validate.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if len(result.Invocations["forgejo-ci-merge"]) != 1 {
		t.Fatalf("forgejo-ci-merge calls = %s, want exactly one merge_due call", formatCalls(result.Invocations["forgejo-ci-merge"]))
	}

	seen, err := os.ReadFile(minRunsSeen)
	if err != nil {
		t.Fatalf("reading recorded FORGEJO_CI_MIN_RUNS: %v", err)
	}
	if got := string(seen); got != "1" {
		t.Errorf("FORGEJO_CI_MIN_RUNS observed by forgejo-ci-merge = %q, want %q", got, "1")
	}
}

func TestScheduledValidateMergeDueDoesNotPinForgejoCIMinRunsForEngineRepo(t *testing.T) {
	h := newShellHarness(t)
	configureCommonGit(h)
	now := time.Now().Unix()
	h.fake("curl", `
case "$*" in
  *"/pulls?state=open"*) echo '[fixture pull JSON]' ;;
esac`)
	h.fake("jq", fmt.Sprintf(`cat <<'EOF'
2 validate/eligible sha-eligible %d false
EOF`, now-1000))
	minRunsSeen := filepath.Join(h.root, "min_runs_seen")
	h.fake("forgejo-ci-merge", fmt.Sprintf(`printf '%%s' "${FORGEJO_CI_MIN_RUNS:-unset}" > %q; exit 0`, minRunsSeen))
	bin := h.fake("twiceshy", ":")
	h.set("TWICESHY_REPO", h.root)
	h.set("TWICESHY_BIN", bin)
	h.set("TWICESHY_JUDGE_URL", "http://judge.invalid")
	h.set("TWICESHY_SOAK_SECONDS", "100")
	h.set("TWICESHY_FORGEJO_REPO", "claude/twiceshy")

	result := h.run("scheduled-validate.sh")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d; stdout: %s stderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if len(result.Invocations["forgejo-ci-merge"]) != 1 {
		t.Fatalf("forgejo-ci-merge calls = %s, want exactly one merge_due call", formatCalls(result.Invocations["forgejo-ci-merge"]))
	}

	seen, err := os.ReadFile(minRunsSeen)
	if err != nil {
		t.Fatalf("reading recorded FORGEJO_CI_MIN_RUNS: %v", err)
	}
	if got := string(seen); got != "unset" {
		t.Errorf("FORGEJO_CI_MIN_RUNS observed by forgejo-ci-merge = %q, want %q (engine repo must not pin)", got, "unset")
	}
}

// runScriptWithArgs is like shellHarness.run, but forwards positional argv
// to the target script. forgejo-ci-merge (REPO PR SHA [REPO_DIR]) needs
// this; the shared harness.run only ever invokes scripts with no args, so
// we can't reuse it as-is without touching harness_test.go.
func runScriptWithArgs(h *shellHarness, script string, args ...string) result {
	h.t.Helper()
	path := filepath.Join(repoRoot(h.t), "scripts", script)
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Dir = h.root
	cmd.Env = []string{
		"PATH=" + h.binDir + ":/usr/bin:/bin",
		"HOME=" + h.root,
		"TMPDIR=" + h.root,
		"FAKE_LOG_DIR=" + h.logDir,
		"LC_ALL=C",
	}
	for key, value := range h.env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			h.t.Fatalf("run %s: %v", script, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return result{
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		ExitCode:    exitCode,
		Invocations: h.readInvocations(),
	}
}

// forgejoActionsFixture builds a fake Forgejo actions/tasks JSON body with n
// workflow_runs, all matching sha and reporting status.
func forgejoActionsFixture(sha, status string, n int) string {
	var runs []string
	for i := 0; i < n; i++ {
		runs = append(runs, fmt.Sprintf(`{"head_sha":%q,"status":%q}`, sha, status))
	}
	return fmt.Sprintf(`{"workflow_runs":[%s]}`, strings.Join(runs, ","))
}

// TestForgejoCIMergeMinRuns covers scripts/forgejo-ci-merge's
// FORGEJO_CI_MIN_RUNS gate. scripts/forgejo-ci-merge must be a repo-tracked,
// byte-identical copy of the deployed ~/.local/bin/forgejo-ci-merge, except
// the merge gate's hardcoded `len(r)>=3` becomes env-configurable. MIN_RUNS
// exists because the corpus repo (claude/twiceshy-corpus) has only ONE
// workflow, while the engine repo has three — a hardcoded "wait for 3
// terminal runs" gate never fires for the corpus repo and every corpus PR
// times out unmerged. The env value must be parsed defensively: empty or
// non-numeric falls back to the historical default of 3; zero or negative
// floors to 1 (a repo always has at least one workflow to wait on).
func TestForgejoCIMergeMinRuns(t *testing.T) {
	const sha = "deadbeefcafebabe1234567890abcdef12345678"
	const repo = "owner/repo"
	const pr = "9"

	setup := func(t *testing.T, minRuns *string, availableRuns int) result {
		t.Helper()
		h := newShellHarness(t)
		// Collapse the poll loop's real 12s backoff to instant retries so an
		// insufficient-runs case (which must exhaust all 80 attempts before
		// giving up) doesn't turn this test into a 16-minute sleep.
		h.fake("sleep", ":")
		h.fake("git", `
case "$*" in
  *"remote get-url origin"*) echo 'http://user:forge-token@forge.invalid/owner/repo' ;;
esac`)
		h.fake("curl", fmt.Sprintf(`
case "$*" in
  *"/actions/tasks"*) echo %q ;;
  *"/pulls/"*"/merge"*) printf '200' ;;
esac`, forgejoActionsFixture(sha, "success", availableRuns)))
		if minRuns != nil {
			h.set("FORGEJO_CI_MIN_RUNS", *minRuns)
		}
		return runScriptWithArgs(h, "forgejo-ci-merge", repo, pr, sha)
	}

	strp := func(s string) *string { return &s }

	for _, tc := range []struct {
		name          string
		minRuns       *string
		availableRuns int
		wantExitCode  int
		wantMerged    bool
	}{
		{
			name:          "unset FORGEJO_CI_MIN_RUNS still requires 3 terminal runs",
			minRuns:       nil,
			availableRuns: 3,
			wantExitCode:  0,
			wantMerged:    true,
		},
		{
			name:          "FORGEJO_CI_MIN_RUNS=1 merges after just 1 terminal run",
			minRuns:       strp("1"),
			availableRuns: 1,
			wantExitCode:  0,
			wantMerged:    true,
		},
		{
			name:          "invalid FORGEJO_CI_MIN_RUNS falls back to 3, so 2 runs is not enough",
			minRuns:       strp("banana"),
			availableRuns: 2,
			wantExitCode:  3,
			wantMerged:    false,
		},
		{
			name:          "empty FORGEJO_CI_MIN_RUNS falls back to 3, so 2 runs is not enough",
			minRuns:       strp(""),
			availableRuns: 2,
			wantExitCode:  3,
			wantMerged:    false,
		},
		{
			name:          "FORGEJO_CI_MIN_RUNS=0 floors to 1",
			minRuns:       strp("0"),
			availableRuns: 1,
			wantExitCode:  0,
			wantMerged:    true,
		},
		{
			name:          "FORGEJO_CI_MIN_RUNS=-5 floors to 1",
			minRuns:       strp("-5"),
			availableRuns: 1,
			wantExitCode:  0,
			wantMerged:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := setup(t, tc.minRuns, tc.availableRuns)
			if result.ExitCode != tc.wantExitCode {
				t.Fatalf("exit code = %d, want %d; stdout: %s stderr: %s", result.ExitCode, tc.wantExitCode, result.Stdout, result.Stderr)
			}
			mergeCalls := result.callsTo("curl", "http://192.168.50.244:3030/api/v1/repos/"+repo+"/pulls/"+pr+"/merge")
			merged := len(mergeCalls) > 0
			if merged != tc.wantMerged {
				t.Fatalf("merged = %v, want %v; curl calls: %s; stdout: %s", merged, tc.wantMerged, formatCalls(result.Invocations["curl"]), result.Stdout)
			}
		})
	}
}

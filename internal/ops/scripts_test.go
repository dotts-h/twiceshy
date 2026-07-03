package ops

import (
	"fmt"
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
	if !containsArg(call.Args, "corpus-stall-alarm: corpus pipeline STALLED — 1 import/validate PR(s) open past 120m or red; the corpus may be frozen. Investigate + drain:\n#77 validate/run-red (age 3m, ci=failure)") {
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

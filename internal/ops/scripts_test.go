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

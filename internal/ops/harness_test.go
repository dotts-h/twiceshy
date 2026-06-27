package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type invocation struct {
	Args  []string
	Stdin string
}

type result struct {
	Stdout      string
	Stderr      string
	ExitCode    int
	Invocations map[string][]invocation
}

type shellHarness struct {
	t      *testing.T
	root   string
	binDir string
	logDir string
	env    map[string]string
}

func newShellHarness(t *testing.T) *shellHarness {
	t.Helper()
	root := t.TempDir()
	h := &shellHarness{
		t:      t,
		root:   root,
		binDir: filepath.Join(root, "bin"),
		logDir: filepath.Join(root, "log"),
		env:    make(map[string]string),
	}
	if err := os.MkdirAll(h.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(h.logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "curl", "jq", "systemctl", "forgejo-ci-merge"} {
		h.fake(name, ":")
	}
	return h
}

func (h *shellHarness) fake(name, script string) string {
	h.t.Helper()
	path := filepath.Join(h.binDir, name)
	body := `#!/usr/bin/env bash
name="$(basename "$0")"
dir="$FAKE_LOG_DIR/$name"
mkdir -p "$dir"
while ! mkdir "$dir/.lock" 2>/dev/null; do :; done
n=1
[ ! -f "$dir/count" ] || n=$(( $(cat "$dir/count") + 1 ))
printf '%s' "$n" > "$dir/count"
rm -rf "$dir/.lock"
printf '%s\0' "$@" > "$dir/$n.args"
case "$name" in
  forgejo-ci-merge|systemctl) : > "$dir/$n.stdin" ;;
  curl)
    capture=false
    for arg in "$@"; do [ "$arg" = "@-" ] && capture=true; done
    if $capture; then cat > "$dir/$n.stdin"; else : > "$dir/$n.stdin"; fi
    ;;
  *) cat > "$dir/$n.stdin" ;;
esac
` + script + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		h.t.Fatal(err)
	}
	return path
}

func (h *shellHarness) set(key, value string) {
	h.env[key] = value
}

func (h *shellHarness) run(script string) result {
	h.t.Helper()
	path := filepath.Join(repoRoot(h.t), "scripts", script)
	cmd := exec.Command("bash", path)
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

func (h *shellHarness) readInvocations() map[string][]invocation {
	h.t.Helper()
	got := make(map[string][]invocation)
	entries, err := os.ReadDir(h.logDir)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(h.logDir, entry.Name())
		files, err := filepath.Glob(filepath.Join(dir, "*.args"))
		if err != nil {
			h.t.Fatal(err)
		}
		sort.Slice(files, func(i, j int) bool {
			a, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(files[i]), ".args"))
			b, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(files[j]), ".args"))
			return a < b
		})
		for _, argsPath := range files {
			argsData, err := os.ReadFile(argsPath)
			if err != nil {
				h.t.Fatal(err)
			}
			stdinData, err := os.ReadFile(strings.TrimSuffix(argsPath, ".args") + ".stdin")
			if err != nil {
				h.t.Fatal(err)
			}
			var args []string
			for _, arg := range bytes.Split(bytes.TrimSuffix(argsData, []byte{0}), []byte{0}) {
				if len(arg) > 0 {
					args = append(args, string(arg))
				}
			}
			got[entry.Name()] = append(got[entry.Name()], invocation{Args: args, Stdin: string(stdinData)})
		}
	}
	return got
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "..", ".."))
}

func (r result) callsTo(binary, target string) []invocation {
	var calls []invocation
	for _, call := range r.Invocations[binary] {
		if containsArg(call.Args, target) {
			calls = append(calls, call)
		}
	}
	return calls
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func requireSingleAlert(t *testing.T, r result, url, token string) invocation {
	t.Helper()
	calls := r.callsTo("curl", url)
	if len(calls) != 1 {
		t.Fatalf("curl calls to %s = %d, want 1; all calls: %#v\nstdout: %s\nstderr: %s", url, len(calls), r.Invocations["curl"], r.Stdout, r.Stderr)
	}
	header := "Authorization: Bearer " + token
	if token != "" && !containsArg(calls[0].Args, header) {
		t.Errorf("alert curl args %q do not contain %q", calls[0].Args, header)
	}
	if token == "" {
		for _, arg := range calls[0].Args {
			if strings.HasPrefix(arg, "Authorization: Bearer") {
				t.Errorf("alert curl unexpectedly has Authorization header: %q", calls[0].Args)
			}
		}
	}
	return calls[0]
}

func configureCommonGit(h *shellHarness) {
	h.fake("git", `
case "$*" in
  *"status --porcelain -- experience/"*) ;;
  *"status --porcelain"*) ;;
  *"config --get remote.origin.url"*) echo 'http://user:forge-token@forge/owner/repo' ;;
  *"rev-parse --verify"*) exit 1 ;;
  *"rev-parse HEAD"*) echo 'head-sha' ;;
  *"rev-parse origin/main:experience"*) echo 'new-tree' ;;
  *"grep -lI"*) echo 'experience/one.md' ;;
esac`)
}

func configureAlertPath(h *shellHarness, script, alertURL string) {
	h.set("NTFY_URL", alertURL)
	switch script {
	case "scheduled-import.sh":
		configureCommonGit(h)
		bin := h.fake("twiceshy", ":")
		h.set("TWICESHY_REPO", h.root)
		h.set("TWICESHY_BIN", bin)
		h.set("TWICESHY_IMPORT_SOURCE", "fixture")
	case "scheduled-validate.sh":
		h.set("TWICESHY_PAUSE", "1")
	case "scheduled-retro.sh":
		configureCommonGit(h)
		bin := h.fake("twiceshy", "exit 1")
		h.set("TWICESHY_REPO", h.root)
		h.set("TWICESHY_BIN", bin)
		h.set("TWICESHY_RETRO_URL", "http://retro.invalid")
	case "corpus-stall-alarm.sh":
		h.fake("python3", "echo '91|import/fixture|1|failure'")
		h.set("FORGEJO_TOKEN", "forge-token")
		h.set("TWICESHY_ALERT_URL", alertURL)
		h.set("TWICESHY_STALL_STATE", filepath.Join(h.root, "stall.state"))
		h.set("TWICESHY_STALL_COOLDOWN", "0")
	case "sync-corpus-to-nas.sh":
		configureCommonGit(h)
		h.fake("ssh", `case "$*" in *"State.Running"*) echo false ;; esac`)
		h.fake("curl", `case "$*" in *"`+alertURL+`"*) exit 0 ;; *) exit 1 ;; esac`)
		h.set("TWICESHY_ALERT_URL", alertURL)
		h.set("TWICESHY_HEALTH_TIMEOUT", "0")
		h.set("TWICESHY_BREAKER_FILE", filepath.Join(h.root, "breaker"))
	case "twiceshy-watchdog.sh":
		h.fake("ssh", ":")
		h.fake("curl", `case "$*" in *"`+alertURL+`"*) exit 0 ;; *) exit 1 ;; esac`)
		h.set("TWICESHY_ALERT_URL", alertURL)
		h.set("TWICESHY_HEALTH_TIMEOUT", "0")
		h.set("TWICESHY_WATCHDOG_BREAKER", filepath.Join(h.root, "breaker"))
	case "twiceshy-growth-watchdog.sh":
		h.fake("git", "exit 1")
		h.set("TWICESHY_REPO", h.root)
		h.set("TWICESHY_GROWTH_STATE", filepath.Join(h.root, "growth.state"))
	default:
		h.t.Fatalf("unknown script %q", script)
	}
}

func formatCalls(calls []invocation) string {
	var parts []string
	for _, call := range calls {
		parts = append(parts, fmt.Sprint(call.Args))
	}
	return strings.Join(parts, "; ")
}

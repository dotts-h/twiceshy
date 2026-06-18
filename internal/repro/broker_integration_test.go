// SPDX-License-Identifier: AGPL-3.0-only

package repro_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// Integration tests need a Docker daemon with the runsc runtime — which the
// socketless CI runner (ADR-0012) deliberately lacks. They run only when
// TWICESHY_REPRO_INTEGRATION=1 (set on the brain, which has docker+runsc).
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TWICESHY_REPRO_INTEGRATION") != "1" {
		t.Skip("set TWICESHY_REPRO_INTEGRATION=1 to run real-runsc integration tests")
	}
}

func newRealBroker() repro.Broker {
	return repro.NewBroker([]string{repro.PinnedGoImage},
		repro.WithLimits(repro.Limits{
			Memory: "512m", CPUs: "1.0", PidsLimit: 128, TmpfsSize: "64m",
			Timeout: 60 * time.Second,
		}))
}

func TestIntegration_PassingReproExitsZero(t *testing.T) {
	requireIntegration(t)
	b := newRealBroker()
	res, err := b.Run(context.Background(), repro.Job{
		Image:   repro.PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte("#!/bin/sh\necho ok; exit 0\n")},
		Execute: []string{"sh", "/work/repro.sh"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Execute.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.Execute.ExitCode, res.Execute.Stderr)
	}
	if !strings.Contains(res.Execute.Stdout, "ok") {
		t.Errorf("stdout=%q, want it to contain ok", res.Execute.Stdout)
	}
}

func TestIntegration_FailingReproPropagatesExit(t *testing.T) {
	requireIntegration(t)
	b := newRealBroker()
	res, err := b.Run(context.Background(), repro.Job{
		Image:   repro.PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte("#!/bin/sh\nexit 7\n")},
		Execute: []string{"sh", "/work/repro.sh"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Execute.ExitCode != 7 {
		t.Fatalf("exit=%d, want 7", res.Execute.ExitCode)
	}
}

// The execute phase must be genuinely offline AND non-root inside real gVisor.
func TestIntegration_ExecuteIsSandboxed(t *testing.T) {
	requireIntegration(t)
	b := newRealBroker()
	script := `#!/bin/sh
# must be non-root
[ "$(id -u)" != "0" ] || { echo "FAIL: running as root"; exit 1; }
# kernel must be gVisor
uname -a | grep -qi gvisor || { echo "FAIL: not gvisor: $(uname -a)"; exit 1; }
# network must be absent: resolving/connecting must fail
if getent hosts proxy.golang.org >/dev/null 2>&1; then echo "FAIL: DNS works"; exit 1; fi
# work dir must be writable (chowned to us)
echo data > /work/out || { echo "FAIL: cannot write /work"; exit 1; }
# rootfs must be read-only
if touch /should-fail 2>/dev/null; then echo "FAIL: rootfs writable"; exit 1; fi
echo SANDBOX_OK
exit 0
`
	res, err := b.Run(context.Background(), repro.Job{
		Image:   repro.PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte(script)},
		Execute: []string{"sh", "/work/repro.sh"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Execute.ExitCode != 0 {
		t.Fatalf("sandbox assertions failed: exit=%d stdout=%q stderr=%q",
			res.Execute.ExitCode, res.Execute.Stdout, res.Execute.Stderr)
	}
	if !strings.Contains(res.Execute.Stdout, "SANDBOX_OK") {
		t.Errorf("stdout=%q", res.Execute.Stdout)
	}
}

// A runaway script must be killed by the watchdog and leave nothing behind.
func TestIntegration_WatchdogKillsTimeout(t *testing.T) {
	requireIntegration(t)
	b := repro.NewBroker([]string{repro.PinnedGoImage},
		repro.WithLimits(repro.Limits{
			Memory: "512m", CPUs: "1.0", PidsLimit: 128, TmpfsSize: "64m",
			Timeout: 3 * time.Second,
		}))
	res, err := b.Run(context.Background(), repro.Job{
		Image:   repro.PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte("#!/bin/sh\nsleep 600\n")},
		Execute: []string{"sh", "/work/repro.sh"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Execute.TimedOut {
		t.Fatalf("expected TimedOut; got exit=%d", res.Execute.ExitCode)
	}
}

// After a clean run, no labelled container or volume may remain.
func TestIntegration_NoLeaksAfterRun(t *testing.T) {
	requireIntegration(t)
	b := newRealBroker()
	if _, err := b.Run(context.Background(), repro.Job{
		Image:   repro.PinnedGoImage,
		Files:   map[string][]byte{"repro.sh": []byte("#!/bin/sh\nexit 0\n")},
		Execute: []string{"sh", "/work/repro.sh"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The Reaper should find nothing to clean up.
	c, v, err := repro.NewReaper().Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if c != 0 || v != 0 {
		t.Errorf("leaked %d containers and %d volumes after a clean run", c, v)
	}
}

// End-to-end: the revalidate doctor proves a real Go record by running its
// repro through the real broker under runsc, and proposes promotion. The repro
// is stdlib-only so it runs offline in the execute phase (no prepare needed).
func TestIntegration_RevalidatorPromotesGoRecord(t *testing.T) {
	requireIntegration(t)
	root := t.TempDir()
	rel := filepath.Join("experience", "repro", "go-stdlib.sh")
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	// F2P repro: builds a tiny stdlib-only Go test and runs it offline. exit 0 =
	// the claim holds; exit 1 = world changed; exit 75 = can't run (skip).
	script := `#!/bin/sh
set -u
command -v go >/dev/null 2>&1 || { echo "SKIP: no go"; exit 75; }
# TMPDIR must be the exec-able work volume: /tmp is mounted noexec, and 'go test'
# compiles the test binary into TMPDIR and execs it (would be EACCES on /tmp).
export GOCACHE=/work/.gocache GOPATH=/work/.gopath GOTOOLCHAIN=local GOFLAGS=-mod=mod
export TMPDIR=/work/.tmp
mkdir -p "$TMPDIR" || exit 75
cd /work || exit 75
mkdir -p m && cd m || exit 75
cat > go.mod <<'EOM'
module reprotest
go 1.25
EOM
cat > x_test.go <<'EOM'
package reprotest
import ("strconv";"testing")
func TestAtoiLeadingZeros(t *testing.T){
  n,err:=strconv.Atoi("0123") // decimal, not octal
  if err!=nil || n!=123 { t.Fatalf("got %d %v",n,err) }
}
EOM
if go test ./... >out 2>&1; then exit 0; fi
cat out; exit 1
`
	if err := os.WriteFile(abs, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.ToSlash(rel)
	rec := &record.Record{
		ID: "exp-9001", Status: "quarantined", Path: "experience/2026/9001-x.md",
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Runtime: map[string]string{"go": ">=1.25"}}},
		Guard:     &record.Guard{Repros: []record.Repro{{Path: p, Kind: "positive"}}},
	}

	rv := repro.NewRevalidator(newRealBroker(), root)
	rep, atts, err := rv.RunWithAttestations(context.Background(), []*record.Record{rec})
	if err != nil {
		t.Fatalf("RunWithAttestations: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("want 1 attestation, got %d", len(atts))
	}
	if !atts[0].Holds {
		t.Fatalf("expected the record to hold; attestation=%+v", atts[0])
	}
	if rec.Status != "quarantined" {
		t.Errorf("revalidator must be report-only; status changed to %q", rec.Status)
	}
	if len(rep.Findings) != 1 || !strings.Contains(rep.Findings[0].Proposal, "promote to validated") {
		t.Errorf("expected a promotion proposal, got %+v", rep.Findings)
	}
}

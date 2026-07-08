// SPDX-License-Identifier: AGPL-3.0-only

// verifier.go implements BrokerVerifier, which scaffolds agent output into a
// language-appropriate toolchain job and delegates execution to a repro.Broker.
// Exit 0 from the toolchain means the trap was avoided; non-zero means it bit.
package agenteval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/repro"
)

// ErrDepsUnavailable marks a DETERMINISTIC dep-resolution failure (the record's
// dep does not exist on the registry), distinguishing a case-input defect from
// a substrate failure.
var ErrDepsUnavailable = errors.New("agenteval: task deps unavailable")

// npmResolutionFailureMarkers are npm's deterministic dependency-RESOLUTION
// failure codes: the requested package/version/tag does not exist, so retrying
// can never succeed. E404 hit at exp-2776 and ETARGET at exp-2778 in the 0140
// live runs; EINVALIDTAGNAME and ENOVERSIONS are the remaining codes of the
// same family. Extend only with codes that are deterministic by definition.
var npmResolutionFailureMarkers = []string{
	"npm error code E404",
	"npm error code ETARGET",
	"npm error code EINVALIDTAGNAME",
	"npm error code ENOVERSIONS",
	// ERESOLVE: the drafted dep SET is uninstallable (peer conflict) — deterministic
	// for the drafted task even though each dep may exist individually (exp-2847).
	"npm error code ERESOLVE",
}

const (
	// pinnedNodeImage is the digest-pinned Node repro-base used for TypeScript
	// type-check jobs (the React/RN traps are tsc type errors).
	pinnedNodeImage = "node:20-bookworm@sha256:8f693eaa7e0a8e71560c9a82b55fd54c2ae920a2ba5d2cde28bac7d1c01c9ba5"

	verifyTimeout = 5 * time.Minute

	// sandboxWorkDir mirrors repro's (unexported) in-container mount point. Jobs MUST
	// point HOME/TMPDIR here: the sandbox user's home is /nonexistent, so npm/go would
	// fail to write their caches there; and /tmp is mounted noexec (exp-0017).
	sandboxWorkDir = "/work"
)

// sandboxEnv is the writable HOME/TMPDIR every toolchain job needs (npm cache, GOCACHE,
// and an exec-able TMPDIR). Without it the prepare phase fails silently in /nonexistent.
func sandboxEnv() map[string]string {
	return map[string]string{"HOME": sandboxWorkDir, "TMPDIR": sandboxWorkDir}
}

// BrokerVerifier scaffolds agent output into toolchain jobs and runs them via
// a repro.Broker. It satisfies the Verifier interface.
type BrokerVerifier struct {
	broker    repro.Broker
	goImage   string
	nodeImage string
}

// NewBrokerVerifier returns a BrokerVerifier backed by broker. Go jobs use
// repro.PinnedGoImage; Node/TypeScript jobs use the pinned Node image.
func NewBrokerVerifier(broker repro.Broker) *BrokerVerifier {
	return &BrokerVerifier{
		broker:    broker,
		goImage:   repro.PinnedGoImage,
		nodeImage: pinnedNodeImage,
	}
}

// VerifierImages returns the pinned images a repro.Broker needs allowlisted to run
// every job buildJob can construct (Go + Node) — the caller-facing way to build a
// broker for this package without knowing the concrete pinned digests.
func VerifierImages() []string {
	return []string{repro.PinnedGoImage, pinnedNodeImage}
}

// Avoided extracts code from output, builds a toolchain job for c.VerifyID,
// runs it via the broker, and returns true when the job exits 0 (trap avoided).
func (v *BrokerVerifier) Avoided(ctx context.Context, c TaskCase, output string) (bool, error) {
	code := extractCode(output)
	job, err := v.buildJob(c.VerifyID, c.Deps, code)
	if err != nil {
		return false, err
	}
	res, err := v.broker.Run(ctx, job)
	if err != nil {
		return false, err
	}
	// A failed prepare (e.g. npm install couldn't run) means the toolchain never set
	// up — the verdict is UNKNOWN, not "avoided". Surface it instead of scoring a
	// vacuous pass (the bug the discrimination control caught). npm's deterministic
	// RESOLUTION failures are the one exception: the record's dep does not exist as
	// asked (a case-input defect), marked ErrDepsUnavailable so the prospect loop
	// can skip-and-count it (#0142). Transient codes (ETIMEDOUT, ECONNRESET, …)
	// deliberately stay run-fatal — a flaky registry must never look like a bad record.
	if len(job.Prepare) > 0 && res.Prepare.ExitCode != 0 {
		stderr := strings.TrimSpace(res.Prepare.Stderr)
		for _, marker := range npmResolutionFailureMarkers {
			if !strings.Contains(stderr, marker) {
				continue
			}
			firstLine := stderr
			if idx := strings.Index(stderr, "\n"); idx != -1 {
				firstLine = strings.TrimSpace(stderr[:idx])
			}
			return false, fmt.Errorf("agenteval: %s prepare: deps unavailable: %s: %w", c.VerifyID, firstLine, ErrDepsUnavailable)
		}
		return false, fmt.Errorf("agenteval: %s prepare failed (exit %d): %s",
			c.VerifyID, res.Prepare.ExitCode, stderr)
	}
	return res.Execute.ExitCode == 0, nil
}

// buildJob constructs a repro.Job for the given verifyID and extracted code. deps
// is c.Deps, used only by the generic "tsc" class — the literal VerifyIDs below
// carry their own hardcoded deps and ignore it.
func (v *BrokerVerifier) buildJob(verifyID string, deps []string, code string) (repro.Job, error) {
	switch verifyID {
	case "fts5-match":
		// NOTE: `go build` only proves the model's code COMPILES. The FTS5 MATCH trap
		// is a RUNTIME parse error (dots/dashes in a bareword), which compiles fine —
		// so a faithful fts5 check must EXECUTE the query against an FTS5 table, not
		// just build it. This compile gate is a placeholder; the runtime check is the
		// follow-up. The TS traps below ARE compile-time type errors, so tsc checks
		// them exactly.
		return v.goBuildJob(code), nil

	case "react19-useref":
		return v.tscJob(code, "typescript @types/react@19 react@19"), nil
	case "rn-viewstyle":
		return v.tscJob(code, "typescript @types/react@19 react@19 react-native@0.76"), nil

	// "gobuild" and "tsc" are the prospector's GENERIC verify classes (#0113): unlike
	// the literal VerifyIDs above (one hardcoded trap each), they scaffold whatever
	// code a drafted task produced. "tsc" has no deps of its own, so an empty Deps is
	// a caller error, not a silent "install nothing and type-check anyway".
	case "gobuild":
		return v.goBuildJob(code), nil
	case "tsc":
		if len(deps) == 0 {
			return repro.Job{}, fmt.Errorf("agenteval: tsc verify requires Deps (npm packages), got none")
		}
		return v.tscJob(code, strings.Join(deps, " ")), nil

	default:
		return repro.Job{}, fmt.Errorf("unknown VerifyID %q", verifyID)
	}
}

// goBuildJob scaffolds code as a compile-only Go project (main.go + a minimal
// go.mod) — the shape shared by "fts5-match" (see its NOTE above) and the generic
// "gobuild" class.
func (v *BrokerVerifier) goBuildJob(code string) repro.Job {
	return repro.Job{
		Image: v.goImage,
		Files: map[string][]byte{
			"go.mod":  []byte("module agenteval-verify\n\ngo 1.25\n"),
			"main.go": []byte(code),
		},
		Env:     sandboxEnv(),
		Execute: []string{"go", "build", "-o", "/dev/null", "."},
		Timeout: verifyTimeout,
	}
}

// tscJob scaffolds the model's code as trap.tsx and type-checks it with tsc, after
// npm-installing the trap's exact type dependencies in the (networked) prepare phase.
// The React/RN traps are type errors (TS2554 / TS2769): tsc exit 0 = avoided, non-zero
// = hit. skipLibCheck keeps the verdict about the model's code, not the libraries' .d.ts.
func (v *BrokerVerifier) tscJob(code, deps string) repro.Job {
	return repro.Job{
		Image: v.nodeImage,
		Files: map[string][]byte{
			// Drive tsc through tsconfig (NOT a file arg, which makes tsc ignore the
			// config and run under defaults that flag clean code). tsc 6 errors on the
			// deprecated moduleResolution:node, so use "bundler"; skipLibCheck keeps the
			// verdict about the model's code, not the libraries' .d.ts.
			"tsconfig.json": []byte(`{"compilerOptions":{"strict":true,"jsx":"react-jsx","esModuleInterop":true,"skipLibCheck":true,"lib":["ES2020","DOM"],"moduleResolution":"bundler","module":"esnext","noEmit":true}}`),
			"trap.tsx":      []byte(code),
		},
		Env:     sandboxEnv(),
		Prepare: []string{"sh", "-lc", "npm install --no-audit --no-fund --no-progress " + deps},
		Execute: []string{"./node_modules/.bin/tsc"},
		Timeout: verifyTimeout,
	}
}

// extractCode returns the content of the first triple-backtick fenced block in
// output, or strings.TrimSpace(output) when no fence is present.
func extractCode(output string) string {
	start := strings.Index(output, "```")
	if start == -1 {
		return strings.TrimSpace(output)
	}
	// Advance past the opening fence line (``` + optional lang + newline).
	rest := output[start+3:]
	nl := strings.Index(rest, "\n")
	if nl == -1 {
		return strings.TrimSpace(output)
	}
	inner := rest[nl+1:]
	end := strings.Index(inner, "```")
	if end == -1 {
		return strings.TrimSpace(output)
	}
	return inner[:end]
}

// SPDX-License-Identifier: AGPL-3.0-only

// Package repro is twiceshy's execution-validation harness (ADR-0011 §3-4): it
// runs a record's repro test-set inside an ephemeral gVisor (runsc) sandbox so a
// quarantined record can be PROMOTED by execution, not by trust.
//
// This file is the **broker** (#0018) — the only place in twiceshy that runs
// untrusted code, so its isolation is non-negotiable (SECURITY_ANALYSIS Facet 5):
//
//   - The container policy is HARDCODED here; a record can never influence it.
//     Every phase runs under `--runtime=runsc`, non-root, read-only rootfs,
//     `--cap-drop=ALL`, `--security-opt=no-new-privileges`, and memory / cpu /
//     pids / wall-clock caps.
//   - The only writable mount is a per-run Docker **named volume** at /work —
//     never a host bind mount (exp-0004). A root populate step (with only
//     CAP_CHOWN, running only trusted tar+chown) hands it to the non-root exec
//     user; the volume is removed in cleanup.
//   - **Two-phase egress:** an optional `prepare` phase runs a *trusted* caller-
//     supplied command (e.g. `go mod download`) with network on the DEFAULT
//     bridge (a custom network breaks gVisor's embedded DNS, exp-0016) to warm
//     pinned dependency caches into the volume; the `execute` phase — which runs
//     the UNTRUSTED repro script — always has `--network=none`. The untrusted
//     script never runs with a network.
//   - A **watchdog** caps each phase's wall-clock and force-kills on timeout; a
//     deferred cleanup plus the package Reaper remove every labelled container
//     and the volume even on panic/crash, so nothing is ever leaked.
//
// The broker is a seam (the Broker interface) injected into the revalidate doctor
// (#0020), like doctor.EOLSource: unit tests drive a stub; one integration test
// drives real runsc.
package repro

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Broker runs a single repro job in an isolated sandbox and returns the result
// of each phase. It MUST guarantee teardown: no container or volume outlives the
// call, even on error, timeout, or panic.
type Broker interface {
	Run(ctx context.Context, job Job) (Result, error)
}

// Job is one repro to run. Its fields are supplied by the (trusted) caller — the
// revalidate doctor — NOT by the experience record. The record contributes only
// the screened script bytes inside Files. Nothing in a Job can weaken the
// hardcoded sandbox policy.
type Job struct {
	// Image is the pinned repro-base image. It MUST be digest-pinned
	// (contain "@sha256:") and MUST be in the broker's allowlist.
	Image string
	// Files are staged into the work dir (/work) before any phase runs. Keys are
	// slash-relative paths (e.g. "repro.sh"); absolute paths or ".." segments
	// are rejected. The script the execute phase runs is one of these.
	Files map[string][]byte
	// Prepare is an OPTIONAL trusted command run with allowlisted egress (default
	// bridge) to warm pinned dependency caches. The untrusted repro script must
	// NOT be run here. Nil/empty skips the prepare phase entirely (no network is
	// ever exposed).
	Prepare []string
	// Execute is the command that runs the UNTRUSTED repro, always with
	// --network=none. Required. Conventionally ["sh", "/work/repro.sh"].
	Execute []string
	// Env is passed to the prepare and execute phases as `--env KEY=VALUE`
	// (trusted: e.g. GOTOOLCHAIN, GOMODCACHE=/work/.gomodcache). Keys/values may
	// not contain NUL, '=', or newlines in the key.
	Env map[string]string
	// Timeout caps each phase's wall clock. Zero uses the broker default.
	Timeout time.Duration
}

// Result is the outcome of a Run. Prepare is the zero PhaseResult when no
// prepare phase was requested.
type Result struct {
	Prepare PhaseResult
	Execute PhaseResult
}

// PhaseResult captures one container phase's outcome. The repro exit-code
// convention (docs/SCHEMA.md): 0 = reproduced+fix-holds, non-zero = did not
// reproduce, 75 = environment can't run it (skip). Interpreting it is the
// revalidator's job (#0020); the broker only reports it.
type PhaseResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool
}

// Limits are the hardcoded resource caps applied to every container. They are
// broker configuration (twiceshy's, not a record's); a record cannot change them.
type Limits struct {
	Memory    string        // docker --memory, e.g. "512m" (swap is disabled too)
	CPUs      string        // docker --cpus, e.g. "1.0"
	PidsLimit int           // docker --pids-limit (fork-bomb guard)
	TmpfsSize string        // size of the /tmp tmpfs, e.g. "64m"
	Timeout   time.Duration // default per-phase wall-clock cap
}

// DefaultLimits are conservative caps suitable for Go repro scripts.
var DefaultLimits = Limits{
	Memory:    "1g",
	CPUs:      "1.0",
	PidsLimit: 256,
	TmpfsSize: "128m",
	Timeout:   3 * time.Minute,
}

// maxFilesBytes caps the total size of staged Files. makeTar materializes them
// in the broker's RAM, so an unbounded set would be a host-memory DoS even
// before anything runs. A repro test-set is small; 8 MiB is generous.
const maxFilesBytes = 8 << 20

const (
	// labelKey marks every container and volume the broker creates so the Reaper
	// can sweep orphans even after a crash. The value is the per-run id.
	labelKey = "twiceshy.repro"
	// workDir is the in-container mount point of the per-run named volume.
	workDir = "/work"
	// execUID is the non-root user/group the prepare+execute phases run as.
	execUID = "65534:65534"
	// defaultRuntime is the gVisor OCI runtime registered with the daemon.
	defaultRuntime = "runsc"
	// PinnedGoImage is the digest-pinned Go repro-base (exp-0017 / #0017).
	PinnedGoImage = "golang:1.25-bookworm@sha256:bbb255b0e131db500cf0520adc97441d2260cf629c7fa7e39e025ddf53995a24"
)

// dockerBroker is the real Broker: it drives the docker CLI through a runner
// seam (so unit tests need no daemon).
type dockerBroker struct {
	runner  commandRunner
	runtime string
	allowed map[string]bool
	limits  Limits
	newID   func() (string, error)
}

// Option configures a dockerBroker.
type Option func(*dockerBroker)

// WithLimits overrides the hardcoded resource caps (twiceshy config only).
func WithLimits(l Limits) Option { return func(b *dockerBroker) { b.limits = l } }

// WithRuntime overrides the OCI runtime name (tests use a stub; prod uses runsc).
func WithRuntime(rt string) Option { return func(b *dockerBroker) { b.runtime = rt } }

// withRunner injects the command runner (tests use a stub).
func withRunner(r commandRunner) Option { return func(b *dockerBroker) { b.runner = r } }

// withIDFunc injects a deterministic id generator (tests only).
func withIDFunc(f func() (string, error)) Option { return func(b *dockerBroker) { b.newID = f } }

// NewBroker returns a Broker that runs allowed images under gVisor. allowedImages
// is the hardcoded set of digest-pinned images a job may use; an image outside it
// is refused. Defaults: runsc runtime, DefaultLimits, crypto-random run ids.
func NewBroker(allowedImages []string, opts ...Option) Broker {
	allowed := make(map[string]bool, len(allowedImages))
	for _, img := range allowedImages {
		allowed[img] = true
	}
	b := &dockerBroker{
		runner:  dockerRunner{},
		runtime: defaultRuntime,
		allowed: allowed,
		limits:  DefaultLimits,
		newID:   randomID,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Run validates the job, then: create volume → populate (root) → prepare
// (optional, networked, trusted) → execute (no network, untrusted) → teardown.
func (b *dockerBroker) Run(ctx context.Context, job Job) (Result, error) {
	if err := b.validate(job); err != nil {
		return Result{}, err
	}
	id, err := b.newID()
	if err != nil {
		return Result{}, fmt.Errorf("repro: id: %w", err)
	}
	vol := "twiceshy-repro-" + id
	label := labelKey + "=" + id

	// Create the per-run named volume — the only writable mount, never a host
	// bind (exp-0004). It is disk-backed: the prepare and execute phases run as
	// separate containers that must SHARE this state, and a tmpfs-backed volume
	// is re-created empty per mount (verified), so it cannot carry state between
	// phases. The volume is removed in cleanup, reclaiming the space.
	// NOTE: a hard disk-size cap on this volume is not yet enforced (see #0025);
	// untrusted writes are bounded for now only by the phase wall-clock.
	if _, err := b.runner.run(ctx, nil, b.limits.Timeout, "docker", "volume", "create",
		"--label", label, vol); err != nil {
		return Result{}, fmt.Errorf("repro: create volume: %w", err)
	}
	// Teardown is unconditional — even on panic, the deferred cleanup plus the
	// Reaper guarantee nothing is leaked.
	defer b.cleanup(id, vol)

	// Populate: extract Files into the volume and chown to the exec user. Runs as
	// root (to chown) but with NO network and only the trusted tar+chown command;
	// the file bytes are written, never executed.
	if err := b.populate(ctx, id, vol, job); err != nil {
		return Result{}, err
	}

	var res Result
	if len(job.Prepare) > 0 {
		res.Prepare = b.runPhase(ctx, id, vol, "prepare", "bridge", execUID, job, job.Prepare)
		if res.Prepare.TimedOut || res.Prepare.ExitCode != 0 {
			// A failed prepare means deps aren't warm; still return so the caller
			// can see it. Execute would run offline and likely fail anyway.
			return res, nil
		}
	}
	res.Execute = b.runPhase(ctx, id, vol, "execute", "none", execUID, job, job.Execute)
	return res, nil
}

// validate enforces every precondition that keeps the sandbox safe. None of
// these can be satisfied by an attacker via record content.
func (b *dockerBroker) validate(job Job) error {
	if !b.allowed[job.Image] {
		return fmt.Errorf("repro: image %q is not in the allowlist", job.Image)
	}
	if !strings.Contains(job.Image, "@sha256:") {
		return fmt.Errorf("repro: image %q is not digest-pinned", job.Image)
	}
	if len(job.Execute) == 0 {
		return fmt.Errorf("repro: execute command is required")
	}
	if len(job.Files) == 0 {
		return fmt.Errorf("repro: no files to stage")
	}
	total := 0
	for name, body := range job.Files {
		if err := safeRelPath(name); err != nil {
			return fmt.Errorf("repro: file %q: %w", name, err)
		}
		total += len(body)
	}
	if total > maxFilesBytes {
		return fmt.Errorf("repro: staged files total %d bytes exceeds cap %d", total, maxFilesBytes)
	}
	for k := range job.Env {
		if k == "" || strings.ContainsAny(k, "=\x00\n") {
			return fmt.Errorf("repro: invalid env key %q", k)
		}
		if strings.ContainsAny(job.Env[k], "\x00") {
			return fmt.Errorf("repro: invalid env value for %q", k)
		}
	}
	return nil
}

// populate stages the job files into the volume and hands ownership to the
// non-root exec user. It uses the job image (which has sh + tar) as root, with no
// network and a read-only rootfs.
func (b *dockerBroker) populate(ctx context.Context, id, vol string, job Job) error {
	tarball, err := makeTar(job.Files)
	if err != nil {
		return fmt.Errorf("repro: tar files: %w", err)
	}
	// populate stages files into the disk-backed volume and hands it to the
	// non-root exec user. It runs the ONLY trusted command in the broker — the
	// `tar`+`chown` below — never untrusted code, with no network. The
	// disk-backed volume is root-owned on create, so populate runs as root with
	// exactly CAP_CHOWN added back (nothing else) to chown it to execUID; the
	// file bytes are written, never executed. The tar is wrapped in `sh -ec`
	// because under runsc a binary running as PID 1 does not reliably consume a
	// piped stdin, but a shell child does.
	args := []string{
		"run", "--rm", "-i",
		"--name", "twiceshy-repro-" + id + "-populate",
		"--label", labelKey + "=" + id,
		"--runtime", b.runtime,
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=" + b.limits.TmpfsSize,
		"--cap-drop", "ALL",
		"--cap-add", "CHOWN", // only to chown the root-owned volume; no other cap
		"--security-opt", "no-new-privileges",
		"--memory", b.limits.Memory,
		"--memory-swap", b.limits.Memory,
		"--cpus", b.limits.CPUs,
		"--pids-limit", strconv.Itoa(b.limits.PidsLimit),
		"--user", "0:0",
		"-v", vol + ":" + workDir,
		"-w", workDir,
		job.Image,
		"sh", "-ec", "tar -xf - -C " + workDir + " && chown -R " + execUID + " " + workDir,
	}
	r, err := b.runner.run(ctx, tarball, b.phaseTimeout(job), "docker", args...)
	if err != nil {
		return fmt.Errorf("repro: populate: %w", err)
	}
	if r.exitCode != 0 {
		return fmt.Errorf("repro: populate failed (exit %d): %s", r.exitCode, strings.TrimSpace(r.stderr))
	}
	return nil
}

// runPhase runs one sandboxed phase under the hardcoded policy and returns its
// result. The policy args come ENTIRELY from the broker; only the image, env,
// and command come from the (trusted) job.
func (b *dockerBroker) runPhase(ctx context.Context, id, vol, phase, network, user string, job Job, cmd []string) PhaseResult {
	args := b.policyArgs(id, vol, phase, network, user, job.Env)
	args = append(args, job.Image)
	args = append(args, cmd...)

	start := time.Now()
	r, err := b.runner.run(ctx, nil, b.phaseTimeout(job), "docker", args...)
	pr := PhaseResult{
		ExitCode: r.exitCode,
		Stdout:   r.stdout,
		Stderr:   r.stderr,
		Duration: time.Since(start),
		TimedOut: r.timedOut,
	}
	if r.timedOut {
		// Watchdog: the CLI was killed but the container may still run — force it
		// down immediately rather than waiting for the deferred cleanup.
		b.kill(id)
		pr.ExitCode = -1
		if err != nil && pr.Stderr == "" {
			pr.Stderr = err.Error()
		}
	}
	return pr
}

// policyArgs builds the hardcoded `docker run` flags for a sandboxed phase.
func (b *dockerBroker) policyArgs(id, vol, phase, network, user string, env map[string]string) []string {
	args := []string{
		"run", "--rm",
		"--name", "twiceshy-repro-" + id + "-" + phase,
		"--label", labelKey + "=" + id,
		"--runtime", b.runtime,
		"--network", network,
		"--read-only",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=" + b.limits.TmpfsSize,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--memory", b.limits.Memory,
		// --memory-swap == --memory disables swap, so --memory is a hard cap and
		// untrusted code can't spill past it into host swap.
		"--memory-swap", b.limits.Memory,
		"--cpus", b.limits.CPUs,
		"--pids-limit", strconv.Itoa(b.limits.PidsLimit),
		"--user", user,
		"-v", vol + ":" + workDir,
		"-w", workDir,
	}
	for _, k := range sortedKeys(env) {
		args = append(args, "--env", k+"="+env[k])
	}
	return args
}

// kill force-removes this run's containers on the watchdog (timeout) path. It
// sweeps by LABEL — more robust than by name if the daemon is busy — so a
// timed-out container does not outlive its wall-clock cap. The deferred cleanup
// is still the final backstop.
func (b *dockerBroker) kill(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b.removeContainersByLabel(ctx, labelKey+"="+id)
}

// cleanup removes every container carrying this run's label and the named
// volume. Best-effort and idempotent (--rm usually removes containers already),
// with the Reaper as the standing backstop — but failures are LOGGED, not
// swallowed, so a stuck resource is observable rather than a silent leak.
func (b *dockerBroker) cleanup(id, vol string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	b.removeContainersByLabel(ctx, labelKey+"="+id)
	if _, err := b.runner.run(ctx, nil, 30*time.Second, "docker", "volume", "rm", "-f", vol); err != nil {
		log.Printf("repro: WARNING: could not remove volume %s (run %s): %v — reaper will retry", vol, id, err)
	}
}

// removeContainersByLabel force-removes every container matching the label,
// logging any container that resists removal.
func (b *dockerBroker) removeContainersByLabel(ctx context.Context, label string) {
	r, err := b.runner.run(ctx, nil, 30*time.Second, "docker", "ps", "-aq", "--filter", "label="+label)
	if err != nil {
		log.Printf("repro: WARNING: could not list containers for %s: %v — reaper will retry", label, err)
		return
	}
	for _, cid := range strings.Fields(r.stdout) {
		if _, err := b.runner.run(ctx, nil, 30*time.Second, "docker", "rm", "-f", cid); err != nil {
			log.Printf("repro: WARNING: could not remove container %s (%s): %v — reaper will retry", cid, label, err)
		}
	}
}

func (b *dockerBroker) phaseTimeout(job Job) time.Duration {
	if job.Timeout > 0 {
		return job.Timeout
	}
	return b.limits.Timeout
}

// safeRelPath rejects absolute paths and ".." traversal so staged files can only
// land under the work dir.
func safeRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("absolute path not allowed")
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return fmt.Errorf("path escapes work dir")
	}
	return nil
}

// makeTar serializes files into a deterministic tar stream (sorted by name) for
// the populate step's `tar -xf -`.
func makeTar(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		body := files[name]
		hdr := &tar.Header{
			Name:    path.Clean(name),
			Mode:    0o644,
			Size:    int64(len(body)),
			ModTime: time.Unix(0, 0),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// randomID returns a short random hex id for naming a run's container+volume.
func randomID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

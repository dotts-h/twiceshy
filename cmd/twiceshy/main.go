// SPDX-License-Identifier: AGPL-3.0-only

// Command twiceshy is the experience service binary (ADR-0001 §9): one Go
// process serving the Phase 1 read path, plus corpus tooling.
//
//	twiceshy index  -corpus <dir> -db <file>          rebuild the derived index
//	twiceshy serve  -corpus <dir> -db <file> -addr …  rebuild, then serve MCP
//	twiceshy ingest <source> -corpus <dir> -db <file> import quarantined records
//	twiceshy draft  -corpus <dir>                     draft+gate+attach repros (needs docker+runsc)
//	twiceshy promote -corpus <dir>                    attestation+judge auto-promote (needs docker+runsc+judge)
//	twiceshy repromote -corpus <dir> -id <exp-NNNN>   attestation+judge restore demoted record (needs docker+runsc+judge)
//	twiceshy adapt  -corpus <dir>                     counter-evidence gate: demote/dispute (needs docker+runsc+judge)
//	twiceshy pack   -corpus <dir> -out <dir>          build a distributable pack
//	twiceshy doctor <name> -corpus <dir>              run a doctor (staleness | revalidate)
//	twiceshy eval   -corpus <dir> -db <file>          report retrieval recall@k / MRR
//	twiceshy corpus-quality -corpus <dir>              report corpus quality + rights coverage
//	twiceshy pilot-report -telemetry <jsonl> ...       compare design-partner pilot windows
//	twiceshy usage-flush -corpus <dir> -db <file>  materialize usage counters into provenance.usage
//	twiceshy corpus-merge-check -corpus <dir> -base <ref> -head <ref>
//	twiceshy corpus-pr-paths -corpus <dir> -base <ref> -head <ref>
//	twiceshy nextid -corpus <dir> -base <ref>          allocate one past local/base max
//	twiceshy gold-add -record <path> -id <Gxx> -mode <mode> -rationale <text>  render a gold.yaml case from an audit miss
//	twiceshy judge-eval                               A/B the judge prompt vs the gold set (needs judge)
//	twiceshy prospect -corpus <dir>                   model-hard trap prospector, report + gold cases (needs docker+runsc+model)
//
// serve requires the bearer token in TWICESHY_TOKEN. index and serve accept an
// optional -embed-url (Ollama) to enable dense, pull-only retrieval (ADR-0009);
// unset keeps retrieval embedding-free.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/eval"
	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/judgeeval"
	"github.com/dotts-h/twiceshy/internal/lock"
	"github.com/dotts-h/twiceshy/internal/notify"
	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
	"github.com/dotts-h/twiceshy/internal/retro"
	runpkg "github.com/dotts-h/twiceshy/internal/run"
	"github.com/dotts-h/twiceshy/internal/server"
	"github.com/dotts-h/twiceshy/internal/spool"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// errUsage marks a flag parse error whose specifics the flag package already
// printed to stderr; main maps it to exit code 2 without re-printing (no double
// message), distinct from `-h` (flag.ErrHelp → exit 0, not an error).
var errUsage = errors.New("invalid flags")

// errAnomalyHalt marks a promote/adapt run that tripped the anomaly guardrail and
// halted before persisting further (ADR-0013 §D1). main maps it to a distinct
// non-zero exit (3) so an unattended wrapper can react to "the guardrail fired"
// specifically, separate from a usage error (2) or a generic failure (1).
var errAnomalyHalt = runpkg.ErrAnomalyHalt

// errPreflight marks a run aborted by the preflight healthcheck (ADR-0013 §A3):
// the broker substrate (docker/runsc) or the judge endpoint was down before any
// record was processed. main maps it to a distinct non-zero exit (4).
var errPreflight = errors.New("preflight check failed")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], os.Stdout, os.Getenv)
	code := exitCode(err)
	if code == 0 {
		return // success (incl. -h): let deferred stop() run
	}
	if !errors.Is(err, errUsage) { // flag already printed the usage specifics
		fmt.Fprintln(os.Stderr, "twiceshy:", err)
	}
	os.Exit(code)
}

// exitCode maps a run error to the process exit code: 0 success / 2 usage /
// 3 anomaly-halt (a guardrail tripped) / 4 preflight failure (substrate down) /
// 1 any other failure.
func exitCode(err error) int {
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	case errors.Is(err, errAnomalyHalt):
		return 3
	case errors.Is(err, errPreflight):
		return 4
	default:
		return 1
	}
}

// brokerHealth and judgeLive are the minimal preflight seams (ADR-0013 §A3):
// repro.Broker and judge.ModelJudge satisfy them, and a fake drives the
// orchestration in tests.
type brokerHealth interface {
	Healthy(ctx context.Context) error
}
type judgeLive interface {
	Ping(ctx context.Context) error
}

// preflight probes the broker substrate (docker/runsc) and the judge endpoint
// before the loop walks the corpus, so a dead substrate aborts cleanly up front
// (distinct exit) instead of failing partway through. The error names which check
// failed.
func preflight(ctx context.Context, b brokerHealth, j judgeLive) error {
	if err := b.Healthy(ctx); err != nil {
		return fmt.Errorf("%w: broker substrate: %v", errPreflight, err)
	}
	if err := j.Ping(ctx); err != nil {
		return fmt.Errorf("%w: judge liveness: %v", errPreflight, err)
	}
	return nil
}

// reapOrphans sweeps sandbox containers/volumes a crashed prior run leaked — the
// Reaper backstop (#0018). A var so tests can spy without a live docker.
var reapOrphans = func(ctx context.Context) (containers, volumes int, err error) {
	return repro.NewReaper().Reap(ctx)
}

// logSkippedPoison reports records the resilient run-loader skipped (#0053): a
// poison/unparseable file does not abort the run, but each one is surfaced
// (slog + prose) so it is never silently dropped.
func logSkippedPoison(logger *slog.Logger, out io.Writer, stage string, skipped []string) {
	for _, s := range skipped {
		if logger != nil {
			logger.Warn("skipped unparseable record", "stage", stage, "detail", s)
		}
		_, _ = fmt.Fprintf(out, "%s: skipped unparseable record — %s\n", stage, s)
	}
}

// startupReap wires the Reaper into the loop start (#0052): before the corpus
// walk, sweep any orphans a crashed prior run left so they don't accumulate. It
// is skipped in a dry-run (-effect writes nothing, so it must delete nothing
// either) and is best-effort — a sweep error is reported, never fatal (a healthy
// substrate already passed preflight; cleanup hiccups shouldn't abort the run).
// For belt-and-suspenders, also run `twiceshy` … with a periodic out-of-band
// sweep (the Reaper is idempotent and safe on a schedule, see repro.Reaper).
func startupReap(ctx context.Context, stage string, dryRun bool, logger *slog.Logger, out io.Writer) {
	if dryRun {
		return
	}
	c, v, err := reapOrphans(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("orphan sweep failed", "stage", stage, "error", err.Error())
		}
		_, _ = fmt.Fprintf(out, "%s: orphan sweep failed: %v (continuing)\n", stage, err)
		return
	}
	if c+v > 0 {
		if logger != nil {
			logger.Info("reaped orphaned sandbox resources", "stage", stage, "containers", c, "volumes", v)
		}
		_, _ = fmt.Fprintf(out, "%s: reaped %d orphaned container(s), %d volume(s)\n", stage, c, v)
	}
}

// parseFlags parses args, leaving usage/errors on stderr (flag's default — never
// stdout). It returns flag.ErrHelp for `-h` and errUsage for a real flag error,
// so main can exit 0 vs 2 without re-printing what flag already showed.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return flag.ErrHelp
		}
		return errUsage
	}
	return nil
}

func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: twiceshy <index|serve|healthcheck|ingest|learned|draft|promote|repromote|adapt|intake-reports|intake-records|intake-issues|retro-intake|screen|report|pack|doctor|eval|corpus-quality|pilot-report|rights-audit|usage-flush|gold-add|judge-eval|prospect|corpus-merge-check|corpus-pr-paths|nextid|token> [flags]")
	}
	switch args[0] {
	case "index":
		return runIndex(ctx, args[1:], out)
	case "serve":
		return runServe(ctx, args[1:], out, getenv)
	case "healthcheck":
		return runHealthcheck(ctx, args[1:], out)
	case "pack":
		return runPack(args[1:], out)
	case "ingest":
		return runIngest(ctx, args[1:], out, getenv)
	case "learned":
		return runLearned(ctx, args[1:], out, getenv)
	case "draft":
		return runDraft(ctx, args[1:], out, getenv)
	case "promote":
		return runPromote(ctx, args[1:], out, getenv)
	case "repromote":
		return runRepromote(ctx, args[1:], out, getenv)
	case "adapt":
		return runAdapt(ctx, args[1:], out, getenv)
	case "intake-reports":
		return runIntakeReports(args[1:], out, getenv)
	case "intake-records":
		return runIntakeRecords(args[1:], out, getenv)
	case "intake-issues":
		return runIntakeIssues(args[1:], out)
	case "retro-intake":
		return runRetroIntake(ctx, args[1:], out, getenv)
	case "screen":
		return runScreen(args[1:], os.Stdin, out)
	case "report":
		return runReport(args[1:], out)
	case "doctor":
		return runDoctor(ctx, args[1:], out)
	case "eval":
		return runEval(ctx, args[1:], out, getenv)
	case "corpus-quality":
		return runCorpusQuality(args[1:], out)
	case "pilot-report":
		return runPilotReport(args[1:], out)
	case "rights-audit":
		return runRightsAudit(args[1:], out)
	case "usage-flush":
		return runUsageFlush(ctx, args[1:], out)
	case "gold-add":
		return runGoldAdd(ctx, args[1:], out)
	case "judge-eval":
		return runJudgeEval(ctx, args[1:], out, getenv)
	case "prospect":
		return runProspect(ctx, args[1:], out, getenv)
	case "self-audit":
		return runSelfAudit(args[1:], out)
	case "similarity":
		return runSimilarity(args[1:], out)
	case "author":
		return runAuthor(args[1:], out)
	case "corpus-merge-check":
		return runCorpusMergeCheck(ctx, args[1:], out)
	case "corpus-pr-paths":
		return runCorpusPRPaths(ctx, args[1:], out)
	case "nextid":
		return runNextID(ctx, args[1:], out, getenv)
	case "idf-build":
		return runIdfBuild(args[1:], out)
	case "token":
		return runToken(ctx, args[1:], out, getenv)
	default:
		return fmt.Errorf("unknown subcommand %q (want index, serve, healthcheck, ingest, learned, draft, promote, repromote, adapt, intake-reports, intake-records, intake-issues, retro-intake, screen, report, pack, doctor, eval, corpus-quality, pilot-report, rights-audit, usage-flush, gold-add, judge-eval, prospect, self-audit, similarity, author, corpus-merge-check, corpus-pr-paths, nextid, token, or idf-build)", args[0])
	}
}

type commonFlags struct {
	corpus     string
	db         string
	repo       string
	embedURL   string
	embedModel string
}

func addCommonFlags(fs *flag.FlagSet) *commonFlags {
	var c commonFlags
	fs.StringVar(&c.corpus, "corpus", ".", "corpus root (the directory containing experience/)")
	fs.StringVar(&c.db, "db", "twiceshy.db", "path of the derived SQLite index")
	fs.StringVar(&c.repo, "repo", "", "corpus repository identifier for app-scoped fingerprints")
	fs.StringVar(&c.embedURL, "embed-url", "", "Ollama endpoint for dense (pull-only) retrieval, e.g. http://192.168.50.150:11434; empty disables dense (ADR-0009)")
	fs.StringVar(&c.embedModel, "embed-model", "nomic-embed-text", "embedding model name for -embed-url")
	return &c
}

// embedderFor builds the pull-only dense embedder from the flags, or nil when
// -embed-url is unset (dense disabled; retrieval stays embedding-free).
func embedderFor(c *commonFlags) index.Embedder {
	if c.embedURL == "" {
		return nil
	}
	return index.NewOllamaEmbedder(c.embedURL, c.embedModel)
}

// loadAndRebuild (re)loads the corpus and rebuilds ix in place, returning the
// record count. When resilient is set (the long-running serve read path) it uses
// the tolerant reader (LoadCorpusForServe): an additive unknown field or a single
// unparseable record is logged + skipped, never fatal — a server must stay up and
// serve the good records, not crash-loop on one it cannot parse (#0064). One-shot
// commands (index, etc.) pass false: strict load, fail loud, the operator/CI sees it.
//
// This is the body of both the startup build and every SIGHUP hot-reload (#0060):
// ix.Rebuild is a single transaction, so concurrent readers keep the prior snapshot
// until it commits (no torn reads), and a failed load rolls back leaving the live
// index intact. Under resilient, a dense re-embed failure (after the record swap
// commits) degrades pull-only dense rather than failing — the new records stay live.
func loadAndRebuild(ctx context.Context, c *commonFlags, ix *index.Index, resilient bool) (int, error) {
	var (
		recs []*record.Record
		err  error
	)
	if resilient {
		var skipped []string
		var critical []string
		recs, skipped, critical, err = record.LoadCorpusForServe(c.corpus)
		if err != nil {
			return 0, fmt.Errorf("loading corpus: %w", err)
		}
		for _, s := range skipped {
			slog.Warn("serve: skipped an unloadable record, serving the rest", "record", s)
		}
		if len(skipped) > 0 {
			slog.Warn("serve: corpus had unloadable records", "skipped", len(skipped))
		}
		// #0080 covers alert metrics and corpus-CI checks; this is the
		// engine-side loud log ADR-0021 §2 requires.
		for _, s := range critical {
			slog.Error("serve: CRITICAL: unsupported schema_version, serving the rest", "record", s)
		}
		if len(critical) > 0 {
			slog.Error("serve: CRITICAL: corpus had unsupported schema_version records", "critical", len(critical))
		}
	} else if recs, err = record.LoadCorpus(c.corpus); err != nil {
		return 0, fmt.Errorf("loading corpus: %w", err)
	}
	if err := ix.Rebuild(ctx, recs, c.repo); err != nil {
		return 0, err
	}
	// Dense retrieval (pull-only) — populate embeddings when configured; cached
	// by content hash so a rebuild only re-embeds changed records (ADR-0009).
	if emb := embedderFor(c); emb != nil {
		if err := ix.EmbedCorpus(ctx, recs, emb); err != nil {
			if !resilient {
				return 0, fmt.Errorf("embedding corpus: %w", err)
			}
			// Serve path: the record swap already committed (ix.Rebuild above), so
			// the new corpus is live on the embedding-free hot path. A re-embed
			// failure only degrades dense (pull-only) — keep serving the new
			// records and report the committed count, rather than fail the reload
			// with a stale count and a misleading "serving prior corpus" alert.
			slog.Warn("serve: corpus (re)loaded but dense re-embed failed; dense retrieval degraded", "err", err)
		}
	}
	return len(recs), nil
}

// buildIndex opens the derived index and loads + rebuilds it from the corpus.
// Rebuild-on-start keeps the index trivially consistent with the records — the
// index is never the source of truth.
func buildIndex(ctx context.Context, c *commonFlags, resilient bool) (*index.Index, int, error) {
	ix, err := index.Open(c.db)
	if err != nil {
		return nil, 0, err
	}
	n, err := loadAndRebuild(ctx, c, ix, resilient)
	if err != nil {
		_ = ix.Close()
		return nil, 0, err
	}
	return ix, n, nil
}

func runIndex(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	c := addCommonFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, n, err := buildIndex(ctx, c, false) // one-shot index build: strict, fail loud for the operator/CI
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	_, _ = fmt.Fprintf(out, "indexed %d records into %s\n", n, c.db)
	return nil
}

// telemetryMaxBytes caps the active gate-decision log before it rotates (#0067);
// one prior generation is kept, so on-disk telemetry is bounded to ~2x this.
const telemetryMaxBytes = 64 << 20 // 64 MiB

func runServe(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	c := addCommonFlags(fs)
	addr := fs.String("addr", ":8722", "listen address")
	reportQueue := fs.String("report-queue", "", "directory report_outcome enqueues outcome reports into for `intake-reports` to materialize (ADR-0013 §E1); empty = legacy markdown-to-PR")
	recordQueue := fs.String("record-queue", "", "directory record_experience enqueues contribution drafts into for `intake-records` to materialize (#0139, ADR-0030 phase 2); empty = return PR-ready markdown")
	retroQueue := fs.String("retro-queue", "", "directory POST /retro spools session transcripts into for `retro-intake` to analyze (ADR-0018, #0065); empty disables the /retro endpoint")
	issueQueue := fs.String("issue-queue", "", "directory report_issue enqueues agent-submitted issues into for `intake-issues` to materialize (#0066); empty = return PR-ready markdown")
	telemetryLog := fs.String("telemetry-log", "", "append per-query gate-decision telemetry to this rotating JSONL file (#0067); empty = disabled")
	telemetryQueryText := fs.Bool("telemetry-query-text", false, "also capture raw query text (truncated 256 bytes) on gate-decision telemetry lines (#0109, single-tenant deployments only); default off, no-op without -telemetry-log")
	idBaseRef := fs.String("base", getenv("TWICESHY_ID_BASE_REF"), "base git ref for merge-safe live write id allocation (env: TWICESHY_ID_BASE_REF)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	token := getenv("TWICESHY_TOKEN")
	if token == "" {
		return errors.New("TWICESHY_TOKEN must be set; the server has no unauthenticated mode")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	// A fatal serve exit (the crash-loop cause: a corpus the index can't build, a
	// bind failure) fires to TWICESHY_ALERT_URL so a restart loop is never silent
	// again; unset = no-op. Pairs with the /healthz + /readyz probes.
	alerter := notify.New(getenv("TWICESHY_ALERT_URL"), getenv("NTFY_TOKEN"), logger)

	ix, n, err := buildIndex(ctx, c, true) // serve = tolerant reader: one bad record never crash-loops the service
	if err != nil {
		alerter.Alert(ctx, "serve-fatal", fmt.Sprintf("serve could not build the index: %v", err))
		return err
	}
	defer func() { _ = ix.Close() }()

	// Per-query gate-decision telemetry (#0067): opt-in via -telemetry-log. Salt the
	// query hash with the bearer token (a per-deployment secret) unless overridden,
	// so a hash can't be dictionary-attacked. Off the hot path; closed on shutdown.
	var tele *telemetry.Recorder
	if *telemetryLog != "" {
		salt := telemetrySalt(getenv("TWICESHY_TELEMETRY_SALT"), token)
		tele, err = telemetry.NewRecorder(telemetry.Config{Path: *telemetryLog, MaxBytes: telemetryMaxBytes, Salt: []byte(salt), Log: logger})
		if err != nil {
			alerter.Alert(ctx, "serve-fatal", fmt.Sprintf("telemetry init failed: %v", err))
			return err
		}
		defer func() { _ = tele.Close() }()
	}

	// TWICESHY_SIGNUP=1 turns on the public POST /signup self-serve alpha token
	// endpoint (#0127); default off, like TWICESHY_PAUSE — the LAN instance never
	// sets it.
	signupEnabled := guard.Truthy(getenv("TWICESHY_SIGNUP"))
	// TWICESHY_DEMO=1 turns on the public GET /demo-search endpoint;
	// default off, like TWICESHY_PAUSE — the LAN instance never sets it.
	demoEnabled := guard.Truthy(getenv("TWICESHY_DEMO"))
	// TWICESHY_TRUSTED_PROXIES (#0131): comma-separated CIDRs (bare IPs accepted
	// as /32 or /128) of reverse proxies allowed to set X-Forwarded-For for the
	// signup per-IP cap; unset = RemoteAddr only. Invalid input fails fast here
	// rather than silently trusting nothing (or the wrong network) at runtime.
	trustedProxies, err := server.ParseTrustedProxies(getenv("TWICESHY_TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("TWICESHY_TRUSTED_PROXIES: %w", err)
	}
	handler, err := server.New(server.Config{Index: ix, RecordCount: n, Token: token, TokenStore: ix, TokenIssuer: ix, SignupEnabled: signupEnabled, DemoEnabled: demoEnabled, TrustedProxies: trustedProxies, Repo: c.repo, Embedder: embedderFor(c), ReportQueue: *reportQueue, RetroQueue: *retroQueue, IssueQueue: *issueQueue, RecordQueue: *recordQueue, Logger: logger, Corpus: c.corpus, IDBaseRef: *idBaseRef, IDFloorResolver: serveIDFloorResolver(c.corpus, getenv), Telemetry: tele, TelemetryQueryText: *telemetryQueryText})
	if err != nil {
		alerter.Alert(ctx, "serve-fatal", fmt.Sprintf("serve could not build the handler: %v", err))
		return err
	}

	// SIGHUP hot-reloads the corpus in place — no restart, no dropped listener
	// (#0060). The corpus-sync timer signals instead of `docker restart`, so a
	// corpus update never blips the service. A reload that fails to load/rebuild
	// keeps the prior good index serving (ix.Rebuild rolls back) and alerts.
	// Registered before the address is reported so an early SIGHUP can't terminate.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	// reloadCtx + reloadWG drain an in-flight rebuild before the deferred
	// ix.Close() runs, so a SIGHUP that coincides with shutdown can't run a
	// Rebuild transaction on a closed DB. stopReload fires on every exit path
	// (graceful shutdown AND a serve error, where ctx itself is not cancelled),
	// and the deferred Wait runs before ix.Close() (defers are LIFO).
	reloadCtx, stopReload := context.WithCancel(ctx)
	var reloadWG sync.WaitGroup
	reloadWG.Add(1)
	go func() {
		defer reloadWG.Done()
		for {
			select {
			case <-reloadCtx.Done():
				return
			case <-hup:
				rn, err := loadAndRebuild(reloadCtx, c, ix, true)
				if err != nil {
					logger.Error("serve: SIGHUP reload failed; serving prior corpus", "err", err)
					alerter.Alert(reloadCtx, "serve-reload-failed", fmt.Sprintf("SIGHUP reload failed, still serving prior corpus: %v", err))
					continue
				}
				handler.SetRecordCount(rn)
				logger.Info("serve: hot-reloaded corpus on SIGHUP", "records", rn)
			}
		}
	}()
	defer func() { stopReload(); reloadWG.Wait() }()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "indexed %d records; listening on %s\n", n, ln.Addr())

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Reap idle keep-alive connections so a client that opens and abandons
		// them can't accumulate file handles indefinitely.
		IdleTimeout: 120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		alerter.Alert(ctx, "serve-fatal", fmt.Sprintf("serve exited unexpectedly: %v", err))
		return err
	}
	return nil
}

// runHealthcheck is the container HEALTHCHECK / external-probe entrypoint: it GETs
// /healthz on the local serve port and exits non-zero if it is not 200. Distroless
// has no curl, so the binary probes itself; this is what lets Docker detect the
// crash-loop the 5h outage hid.
func runHealthcheck(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	addr := fs.String("addr", ":8722", "serve address to probe (host optional; defaults to 127.0.0.1)")
	path := fs.String("path", "/healthz", "health path to probe (/healthz or /readyz)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	url := "http://" + host + *path
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck: %s unreachable: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: %s returned HTTP %d", url, resp.StatusCode)
	}
	_, _ = fmt.Fprintln(out, "ok")
	return nil
}

// importSource resolves a CLI source selector to its adapter.
func importSource(name, ecosystem string) (ingest.Source, error) {
	switch name {
	case "go":
		return ingest.NewGoSource(), nil
	case "osv":
		return ingest.NewOSVSource(), nil
	case "py":
		return ingest.NewPySource(), nil
	case "osv-live":
		// ecosystem ("" ignored → Go) lets one importer cover a whole stack:
		// npm (React/React Native), PyPI (Python), Go — one run per ecosystem.
		return ingest.NewOSVLiveSource(ingest.WithEcosystem(ecosystem)), nil
	case "eol-live":
		// endoflife.date → deprecation records for end-of-life runtimes (#0023);
		// the default product set covers the common runtimes (unknown ones 404→skip).
		return ingest.NewEOLLiveSource(), nil
	case "npm-deprecation":
		// npm registry → deprecation records for deprecated packages (#0073), the
		// non-OSV web watcher; the default package set is curated (unknown ones 404→skip).
		return ingest.NewNpmLiveSource(), nil
	case "node-breaking":
		// Node.js's own changelogs → trap records for SEMVER-MAJOR breaking changes
		// (#0115); the default major-line target set is curated (unknown ones 404→skip).
		return ingest.NewNodeBreakingSource(), nil
	case "wtf":
		// wtfjs/wtfpython README gotchas → trap records for JS/Python quirks (#0134);
		// WTFPL prose, born quarantined.
		return ingest.NewWtfSource(), nil
	default:
		return nil, fmt.Errorf("unknown ingest source %q (want: go, osv, osv-live, eol-live, npm-deprecation, node-breaking, wtf, py)", name)
	}
}

// runIngest imports quarantined records from a license-clean source (#0007).
// Records are deduped against the corpus (via ingest.Prepare) and within the
// batch, then written to disk for a human to open as a PR — git is the trust
// boundary, so nothing is born validated and nothing reaches the push channel.
func runIngest(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	// The source is the first positional; flags follow it. Go's flag package
	// stops at the first non-flag arg, so pull the source off the front before
	// parsing (otherwise `ingest go -corpus X` would leave -corpus unparsed).
	if len(args) < 1 {
		return errors.New("usage: twiceshy ingest <source> [flags] (sources: go, osv, osv-live, eol-live, npm-deprecation, node-breaking, wtf, py)")
	}
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	c := addCommonFlags(fs)
	dryRun := fs.Bool("dry-run", false, "classify and report, but write no files")
	limit := fs.Int("limit", 0, "max new records to write this run (0 = unlimited); bounds a scheduled import")
	author := fs.String("author", "twiceshy-importer", "provenance author recorded on imported records")
	ecosystem := fs.String("ecosystem", "", "OSV ecosystem for osv-live (e.g. npm, PyPI, Go); empty = Go")
	base := fs.String("base", "", "base git ref for merge-safe id allocation")
	openPRs := fs.Bool("open-prs", false, "also allocate ids above records on open corpus PRs (Forgejo API, #0121)")
	if err := parseFlags(fs, args[1:]); err != nil {
		return err
	}
	src, err := importSource(args[0], *ecosystem)
	if err != nil {
		return err
	}

	ix, _, err := buildIndex(ctx, c, false)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	drafts, err := src.Drafts(ctx)
	if err != nil {
		return err
	}

	// The scan is skipped when nothing will be written — a dry run or an empty
	// draft set must not acquire a network dependency (or burn API calls) for
	// an id it never uses.
	floors, err := openPRFloors(ctx, c.corpus, *openPRs && !*dryRun && len(drafts) > 0, getenv)
	if err != nil {
		return err
	}
	id, err := ingest.NextIDWithBase(ctx, ix, c.corpus, *base, floors...)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02")

	_, err = ingest.ImportBatch(ctx, ix, c.repo, c.corpus, src.Name(), drafts, id, *author, now, *dryRun, *limit, writeRecord, out)
	return err
}

// safeJoin joins rel under base and verifies the result stays within base —
// defense in depth (#0013) against a record path that escapes the corpus/output
// root. Record paths are already derived (buildPath/slugify), so this is a
// belt-and-suspenders guard. The error names only rel, never the absolute base.
func safeJoin(base, rel string) (string, error) {
	clean := filepath.FromSlash(rel)
	dst := filepath.Join(base, clean)
	rp, err := filepath.Rel(filepath.Clean(base), dst)
	if filepath.IsAbs(clean) || err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing path that escapes the output root: %q", rel)
	}
	return dst, nil
}

// writeRecord marshals a record and writes it under the corpus at its path,
// creating the year directory. Persistence is a CLI concern (ADR-0008).
func writeRecord(corpus string, rec *record.Record) error {
	md, err := record.Marshal(rec)
	if err != nil {
		return err
	}
	dst, err := safeJoin(corpus, rec.Path)
	if err != nil {
		return err
	}
	return writeFileAtomic(dst, md, 0o644)
}

// writeFileAtomic writes data to a temp file in the destination directory and
// renames it into place, so a crash or ENOSPC mid-write can never leave a
// truncated, unparseable record where a valid one was (rename is atomic within a
// directory). The draft command rewrites EXISTING records in place to attach a
// proven repro, so a plain truncate-then-write would risk corrupting a
// known-good record file on a partial write.
func writeFileAtomic(dst string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // harmless no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

// runDraft runs the deterministic drafter pipeline (ADR-0011 §8) over the corpus:
// for each quarantined record without a repro, it drafts a candidate repro, gates
// it in the gVisor broker (fail-pre / pass-post, offline), and attaches the proof
// into the record's guard — still quarantined; promotion stays the human PR step
// (#0020). The execute phase runs untrusted code, so this needs docker + the runsc
// runtime (the brain); a bare checkout can use -dry-run to list candidates.
func runDraft(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	dryRun := fs.Bool("dry-run", false, "list the quarantined candidate records; run no gate, write nothing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}

	if *dryRun {
		n := 0
		for _, rec := range recs {
			if runpkg.IsCandidate(rec) {
				n++
				_, _ = fmt.Fprintf(out, "  candidate %s %s\n", rec.ID, rec.Path)
			}
		}
		_, _ = fmt.Fprintf(out, "draft (dry-run): %d quarantined candidate(s); the drafter templates the supported subset and the broker gate proves each\n", n)
		return nil
	}

	// DefaultLimits are the SAME caps the revalidate doctor (#0020) runs these
	// repros under, so a repro proven here holds identically when the doctor
	// re-checks it — no draft-vs-revalidate cap divergence.
	b := repro.NewBroker([]string{repro.PinnedGoImage})
	rv := repro.NewRevalidator(b, *corpus)
	p := drafter.NewPipeline(rv, *corpus, draftersFrom(getenv)...)

	st, err := runpkg.DraftCorpus(ctx, *corpus, recs, p, writeRecord, out)
	if err != nil {
		return err
	}
	runpkg.WriteDraftSummary(out, st)
	return nil
}

// draftersFrom builds the drafter chain for `twiceshy draft`: the deterministic
// template drafter always, plus the model drafter (#0026 slice 3) when
// TWICESHY_DRAFTER_URL is configured (off-pool Ollama, e.g. qwen2.5-coder on VM
// 101). Deterministic is tried first; the model covers what templates can't. With
// no env the run is deterministic-only, so a bare checkout needs no model.
func draftersFrom(getenv func(string) string) []drafter.Drafter {
	ds := []drafter.Drafter{drafter.NewGoDeprecationDrafter()}
	if url := strings.TrimSpace(getenv("TWICESHY_DRAFTER_URL")); url != "" {
		model := strings.TrimSpace(getenv("TWICESHY_DRAFTER_MODEL"))
		if model == "" {
			model = "qwen2.5-coder:14b"
		}
		ds = append(ds, drafter.NewModelDrafter(url, model))
	}
	return ds
}

// defaultMaxActions is the default anomaly-halt threshold (ADR-0013 §7/§D1, #0033):
// in an UNBOUNDED run, more auto-promotions/demotions than this halts the loop (the
// "judge approving everything" backstop). When a throughput cap (-max-promotions)
// is set, the cap governs and this count-anomaly is moot (#0084).
const defaultMaxActions = 25

// defaultMaxPromotions is the default intended throughput cap (#0084): 0 = off, so
// behaviour is unchanged until an operator opts in with -max-promotions. The
// scheduled driver sets it to a clean per-run batch size (with -max-actions raised
// or disabled) so a normal batch stops cleanly instead of tripping the anomaly halt.
const defaultMaxPromotions = 0

// defaultMaxActionRate is the default approval-rate anomaly baseline (#0085): 0 = off,
// so behaviour is unchanged until an operator opts in (like the throughput cap). When
// enabled (e.g. 0.6), a capped run promoting/demoting more than this fraction of the
// records it judged is flagged as a likely compromised judge — the spike detector
// that SURVIVES a cap, where the raw-count anomaly (-max-actions) goes moot.
const defaultMaxActionRate = 0

// defaultMinSample is the default minimum judged records before the rate anomaly can
// fire, so a tiny batch (e.g. 3/3) is never flagged on too little signal (#0085).
const defaultMinSample = 10

// guardrailsFrom builds the safety limits for a promote/adapt run: the emergency
// stop from TWICESHY_PAUSE, the throughput cap (clean stop), and the anomaly +
// budget backstops.
func guardrailsFrom(getenv func(string) string, maxActions, maxPromotions, maxRuns int, maxActionRate float64, minSample int) guard.Guardrails {
	return guard.Guardrails{
		Paused:        guard.Truthy(getenv("TWICESHY_PAUSE")),
		MaxActions:    maxActions,
		MaxPromotions: maxPromotions,
		MaxActionRate: maxActionRate,
		MinSample:     minSample,
		MaxRuns:       maxRuns,
	}
}

// wrapFrontierFallback wraps the advisory panel's primary frontier judge with a
// Gemini→Sonnet fallback (#0086) when fbURL is set; otherwise it returns primary
// unchanged. The fallback fires only on a primary ERROR (free-tier Gemini exhausting
// its daily quota → 429), never on a primary reject — so off-pool on the happy path
// without the daily-quota stall a straight swap would cause.
func wrapFrontierFallback(primary judge.Judge, fbURL, fbModel, drafterModel string, votes int) (judge.Judge, error) {
	if fbURL == "" {
		return primary, nil
	}
	fb, err := judge.NewModelJudge(judge.Config{
		Endpoint: fbURL, Model: fbModel, DrafterModel: drafterModel,
		System: judge.AdvisorySystemV2, Advisory: true,
	})
	if err != nil {
		return nil, fmt.Errorf("configuring advisory panel fallback judge: %w", err)
	}
	return judge.NewFallback(primary, judge.NewMajority(judge.NewTiming(fb), votes)), nil
}

func buildPromoterOptions(getenv func(string) string, judgeURL, judgeModel, drafterModel string, votes int) ([]promote.Option, error) {
	promoterOpts := []promote.Option{}
	if panelURL := getenv("TWICESHY_PANEL_JUDGE_URL"); panelURL != "" {
		panelModel := getenv("TWICESHY_PANEL_JUDGE_MODEL")
		aj1, err := judge.NewModelJudge(judge.Config{
			Endpoint: judgeURL, Model: judgeModel, DrafterModel: drafterModel,
			System: judge.AdvisorySystemV2, Advisory: true,
		})
		if err != nil {
			return nil, fmt.Errorf("configuring advisory panel primary judge: %w", err)
		}
		aj2, err := judge.NewModelJudge(judge.Config{
			Endpoint: panelURL, Model: panelModel, DrafterModel: drafterModel,
			System: judge.AdvisorySystemV2, Advisory: true,
		})
		if err != nil {
			return nil, fmt.Errorf("configuring advisory panel secondary judge: %w", err)
		}
		// Frontier seat = the panel's diverse second family. Default: just the
		// PANEL_JUDGE model. Hybrid (#0086): when TWICESHY_PANEL_JUDGE_FALLBACK_URL is
		// set, wrap it so a primary FAILURE (e.g. a free-tier Gemini exhausting its
		// daily quota → 429) falls back to a pooled secondary (Sonnet) instead of
		// fail-safe-skipping the record. A primary REJECT does NOT fall back (it is a
		// real verdict, not a failure). The panel member keeps the primary's family
		// label for the construction-time diversity check; the runtime verdict.Model
		// records whichever model actually answered.
		frontier, err := wrapFrontierFallback(
			judge.NewMajority(judge.NewTiming(aj2), votes),
			getenv("TWICESHY_PANEL_JUDGE_FALLBACK_URL"),
			getenv("TWICESHY_PANEL_JUDGE_FALLBACK_MODEL"), drafterModel, votes)
		if err != nil {
			return nil, err
		}
		panel, err := judge.NewPanel(
			judge.PanelMember{Model: judgeModel, Judge: judge.NewMajority(judge.NewTiming(aj1), votes)},
			judge.PanelMember{Model: panelModel, Judge: frontier},
		)
		if err != nil {
			return nil, fmt.Errorf("configuring advisory panel: %w", err)
		}
		promoterOpts = append(promoterOpts, promote.WithAdvisoryPanel(panel))
		// Born-stale gate (#0071, companion to #302), paired with the panel since it
		// only fires on the advisory path: never promote an advisory whose runtime is
		// already EOL — it would trip the (validated-scoped) D2 staleness guard the
		// instant it became validated, the very thing that stuck ~36 validate PRs.
		// Uses the public endoflife.date source (TWICESHY_EOL_URL overrides — e.g. a
		// test/offline stub). Fails open (a source outage ⇒ no flag ⇒ promotion
		// proceeds), with the deterministic D2 guard test as the backstop.
		staleGate := doctor.NewStaleness(doctor.NewEndOfLifeSource(getenv("TWICESHY_EOL_URL")), time.Now().UTC())
		promoterOpts = append(promoterOpts, promote.WithStalenessGate(staleGate.WouldFlag))
	}
	// Prose-class panel (ADR-0020): a cross-family panel for no-repro, no-source lessons.
	// Members are gpt-oss (the off-pool local judge) + an operator-designated frontier
	// family on TWICESHY_PROSE_PANEL_JUDGE_URL (agy) — the gemini FREE tier is excluded
	// for prose (privacy, ADR-0016 §5), and the ADR-0013 §6 local denylist stays enforced
	// (neither member is a denylisted family). The prompt foregrounds poison and rejects
	// on uncertainty; the mandatory content-screen + fail-safe panel are in promote.
	if prosePanelURL := getenv("TWICESHY_PROSE_PANEL_JUDGE_URL"); prosePanelURL != "" {
		prosePanelModel := getenv("TWICESHY_PROSE_PANEL_JUDGE_MODEL")
		pj1, err := judge.NewModelJudge(judge.Config{
			Endpoint: judgeURL, Model: judgeModel, DrafterModel: drafterModel,
			System: judge.ProsePanelSystemV2, Prose: true,
		})
		if err != nil {
			return nil, fmt.Errorf("configuring prose panel primary judge: %w", err)
		}
		pj2, err := judge.NewModelJudge(judge.Config{
			Endpoint: prosePanelURL, Model: prosePanelModel, DrafterModel: drafterModel,
			System: judge.ProsePanelSystemV2, Prose: true,
		})
		if err != nil {
			return nil, fmt.Errorf("configuring prose panel secondary judge: %w", err)
		}
		prosePanel, err := judge.NewPanel(
			judge.PanelMember{Model: judgeModel, Judge: judge.NewMajority(judge.NewTiming(pj1), votes)},
			judge.PanelMember{Model: prosePanelModel, Judge: judge.NewMajority(judge.NewTiming(pj2), votes)},
		)
		if err != nil {
			return nil, fmt.Errorf("configuring prose panel: %w", err)
		}
		promoterOpts = append(promoterOpts, promote.WithProsePanel(prosePanel))
		// The born-stale gate (valid.until) guards the prose path too; add it if the
		// advisory block above didn't already wire the (idempotent) same gate.
		if getenv("TWICESHY_PANEL_JUDGE_URL") == "" {
			staleGate := doctor.NewStaleness(doctor.NewEndOfLifeSource(getenv("TWICESHY_EOL_URL")), time.Now().UTC())
			promoterOpts = append(promoterOpts, promote.WithStalenessGate(staleGate.WouldFlag))
		}
	}
	return promoterOpts, nil
}

// newRunLogger builds the structured loop logger for one promote/adapt run: JSON
// to stderr (the read path's slog.NewJSONHandler pattern, internal/server),
// scoped to runID so a night's events are greppable by run. stdout stays the
// human prose channel (or the -json manifest) — structured logs never pollute it.
func newRunLogger(runID string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("run_id", runID)
}

// surfaceJudgeStats logs aggregate judge latency and verdict distribution when the
// run made at least one judge call. nil is omitted from the run manifest.
func surfaceJudgeStats(runLog *slog.Logger, tj *judge.TimingJudge) *judge.JudgeStats {
	stats := tj.Stats()
	if stats.Calls == 0 {
		return nil
	}
	runLog.Info("judge stats",
		"calls", stats.Calls,
		"approvals", stats.Approvals,
		"rejections", stats.Rejections,
		"p50_ms", stats.P50ms,
		"p95_ms", stats.P95ms,
	)
	return &stats
}

// newRunID is a sortable, filesystem-safe id for one promote/adapt run.
func newRunID() string {
	return "run-" + time.Now().UTC().Format("20060102T150405Z")
}

// loopLockName is the corpus-local single-flight lockfile shared by promote and
// adapt, so the two mutating commands are mutually exclusive (ADR-0013 §A2).
const loopLockName = ".twiceshy-loop.lock"

// acquireLoopLock takes the single-flight lock for a mutating run, mapping
// contention to a clear, non-zero-exit error (a second overlapping run skips
// rather than double-writing).
func acquireLoopLock(corpus string) (*lock.Lock, error) {
	path := filepath.Join(corpus, loopLockName)
	lk, err := lock.Acquire(path)
	if errors.Is(err, lock.ErrHeld) {
		return nil, fmt.Errorf("another promote/adapt run is in progress (lock %s held) — skipping this run: %w", path, lock.ErrHeld)
	}
	if err != nil {
		return nil, fmt.Errorf("acquiring run lock %s: %w", path, err)
	}
	return lk, nil
}

// runPromote is the positive direction of ADR-0013 (#0029): for each quarantined
// execution-provable record, a holding broker attestation PLUS a judge PASS flips
// it to validated with no human approver, recording the attestation + verdict in
// provenance. The execute phase runs untrusted repros, so a real run needs docker
// + the runsc runtime (the brain) AND a judge endpoint (TWICESHY_JUDGE_URL, off
// the Anthropic pool); a bare checkout can use -dry-run to list candidates.
func runPromote(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	judgeModel := fs.String("judge-model", "", "diverse frontier judge model id, e.g. gemini-2.5-pro (must differ from -drafter-model)")
	drafterModel := fs.String("drafter-model", "", "the model that drafted records; the judge must not share its family (anti-monoculture)")
	dryRun := fs.Bool("dry-run", false, "list the execution-provable promotion candidates; run no gate/judge, write nothing")
	effect := fs.Bool("effect", false, "with the gate+judge run but write NOTHING, print the would-be status delta per record (effect preview)")
	asJSON := fs.Bool("json", false, "emit a machine-readable run manifest (every record's transition) to stdout instead of prose; the daily audit reads this")
	maxActions := fs.Int("max-actions", defaultMaxActions, "anomaly-halt backstop for UNBOUNDED runs: promotions per run above which the loop halts (0 = off; moot when -max-promotions is set)")
	maxPromotions := fs.Int("max-promotions", defaultMaxPromotions, "throughput cap: stop CLEANLY after this many promotions (a mergeable batch; re-run to continue). 0 = unlimited (#0084)")
	maxActionRate := fs.Float64("max-action-rate", defaultMaxActionRate, "approval-rate anomaly: flag a run whose promoted/judged fraction exceeds this (survives a throughput cap, unlike -max-actions). 0 = off (#0085)")
	minSample := fs.Int("min-sample", defaultMinSample, "minimum judged records before -max-action-rate can fire, so a tiny run isn't flagged (#0085)")
	holdCooldown := fs.Duration("hold-cooldown", defaultHoldCooldown, "skip re-judging a record held within this window — stops the held backlog re-judging itself every run. 0 = off (#0084)")
	maxRuns := fs.Int("max-runs", 0, "budget cap: max records processed (broker/judge runs) per invocation (0 = unlimited)")
	votes := fs.Int("votes", judge.DefaultVotes, "judge each record this many times and promote on majority-approve only — closes the model's single-shot non-determinism (ADR-0013 §F1; min 1)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	recs, skipped, err := record.LoadCorpusResilient(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}

	if *dryRun && !*effect {
		logSkippedPoison(nil, out, "promote", skipped)
		n := 0
		for _, rec := range recs {
			if ok, _ := promote.Promotable(rec); ok {
				n++
				_, _ = fmt.Fprintf(out, "  candidate %s %s\n", rec.ID, rec.Path)
			}
		}
		_, _ = fmt.Fprintf(out, "promote (dry-run): %d promotable candidate(s); proof-path needs attestation+judge, advisory-path and prose-path need their panels\n", n)
		return nil
	}

	// Single-flight (ADR-0013 §A2): only one mutating run at a time. A second
	// overlapping run (cron tick + manual, or two ticks) exits here, before any
	// judge/broker setup or write, rather than double-writing the corpus.
	lk, err := acquireLoopLock(*corpus)
	if err != nil {
		return err
	}
	defer func() { _ = lk.Release() }()

	// Fail-safe: no judge configured → nothing is ever auto-promoted (ADR-0013 §6).
	judgeURL := getenv("TWICESHY_JUDGE_URL")
	if judgeURL == "" {
		return errors.New("TWICESHY_JUDGE_URL must be set: auto-promotion requires a diverse-model judge (it is never bypassed)")
	}
	// System + think are the measured A/B winner (internal/judgeeval, repeat=5):
	// the prose prompt at think=false had 0 false-approve / 0 false-reject, beating
	// the rubric (over-rejects) and think=true (adds false-approves, slower). Pinned
	// here so the validated prompt lives in version control, not the untracked shim.
	// ProseSystemV2 supersedes that measured V1 text with the added usefulness
	// check (#0110); the false-approve/false-reject figures above are V1's — V2
	// awaits its own live A/B re-measurement (gated behind an endpoint, never CI).
	j, err := judge.NewModelJudge(judge.Config{
		Endpoint: judgeURL, Model: *judgeModel, DrafterModel: *drafterModel,
		System: judge.ProseSystemV2, Think: false,
	})
	if err != nil {
		return fmt.Errorf("configuring judge: %w", err)
	}

	// The SAME broker caps the revalidate doctor (#0020) and draft use, so a repro
	// proven here holds identically when re-checked.
	b := repro.NewBroker([]string{repro.PinnedGoImage})
	rv := repro.NewRevalidator(b, *corpus)
	// Production majority voting (ADR-0013 §F1): wrap the model judge so each
	// record is judged -votes times and promotes on majority-approve only — closes
	// the measured ~0.7% single-shot false-approve (exp-0046). The raw j keeps its
	// Ping for preflight; only the gate sees the voting wrapper. TimingJudge sits
	// inside Majority so each inner HTTP call is timed, not the N-vote group.
	tj := judge.NewTiming(j)
	promoterOpts, err := buildPromoterOptions(getenv, judgeURL, *judgeModel, *drafterModel, *votes)
	if err != nil {
		return err
	}
	p := promote.NewPromoter(rv, judge.NewMajority(tj, *votes), *corpus, promoterOpts...)

	// Preflight (ADR-0013 §A3): abort before walking the corpus if the sandbox
	// substrate or the judge endpoint is down, rather than failing mid-run.
	if err := preflight(ctx, b, j); err != nil {
		return err
	}

	g := guardrailsFrom(getenv, *maxActions, *maxPromotions, *maxRuns, *maxActionRate, *minSample)
	runID := newRunID()
	runLog := newRunLogger(runID)
	// Guardrail trips fire to TWICESHY_ALERT_URL (ntfy) when set; unset = no-op.
	alerter := notify.New(getenv("TWICESHY_ALERT_URL"), getenv("NTFY_TOKEN"), runLog)
	persist := writeRecord
	if *effect {
		persist = func(string, *record.Record) error { return nil }
	}
	now := time.Now()
	holds := loadHoldLedger(*corpus, *holdCooldown)
	return runpkg.RunStage(ctx, out, getenv, runLog, "promote", runID, *asJSON, *effect,
		func() *judge.JudgeStats { return surfaceJudgeStats(runLog, tj) },
		func(proseOut io.Writer) (runpkg.PromoteStats, []promote.RecordAction, error) {
			// Sweep a crashed prior run's leaked sandbox resources before the walk (#0052).
			startupReap(ctx, "promote", *effect, runLog, proseOut)
			logSkippedPoison(runLog, proseOut, "promote", skipped)
			// Hold cooldown (#0084): drop records the panel declined within the window so a
			// scheduled run doesn't re-run the costly judge on the same held backlog every
			// time. The ledger is operational state under <corpus>/runs/, alongside the
			// journals; a nil ledger (cooldown 0) keeps every record.
			cooledRecs, cooled := filterCooldown(recs, holds, now)
			if cooled > 0 {
				_, _ = fmt.Fprintf(proseOut, "promote: %d record(s) in hold-cooldown — skipped (re-judged at most once per %s)\n", cooled, *holdCooldown)
				runLog.Info("hold cooldown", "skipped", cooled, "cooldown", holdCooldown.String())
			}
			return runpkg.PromoteCorpus(ctx, *corpus, cooledRecs, p, persist, g, runLog, alerter, proseOut, runpkg.JournalPathForRun(*corpus, "promote", *effect))
		},
		func(_ runpkg.PromoteStats, actions []promote.RecordAction, _ error) error {
			// Fold this run's outcomes into the cooldown ledger: a held record starts its
			// cooldown; a promoted one is cleared. Done after the effect short-circuit so a
			// preview run never mutates the ledger.
			noteOutcomes(holds, actions, now)
			if serr := holds.save(now); serr != nil {
				runLog.Warn("hold ledger save failed", "err", serr.Error())
			}
			return nil
		},
		runpkg.PromoteManifest,
		runpkg.WritePromoteSummary)
}

// runRepromote is the reversal path of ADR-0013 (#0048): for one stale or
// disputed execution-provable record, a holding broker attestation PLUS a judge
// PASS restores it to validated — clearing valid.until and the demotion block.
// Like promote it needs docker + runsc (the brain) AND a judge endpoint
// (TWICESHY_JUDGE_URL); a bare checkout can use -dry-run to preview eligibility.
func runRepromote(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("repromote", flag.ContinueOnError)
	id := fs.String("id", "", "record id to restore (required, exp-NNNN)")
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	judgeModel := fs.String("judge-model", "", "diverse frontier judge model id, e.g. gemini-2.5-pro (must differ from -drafter-model)")
	drafterModel := fs.String("drafter-model", "", "the model that drafted records; the judge must not share its family (anti-monoculture)")
	dryRun := fs.Bool("dry-run", false, "report whether the record is re-promotable; run no gate/judge, write nothing")
	votes := fs.Int("votes", judge.DefaultVotes, "judge this many times and re-promote on majority-approve only (ADR-0013 §F1; min 1)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("repromote requires -id <exp-NNNN>")
	}
	if !record.ValidID(*id) {
		return fmt.Errorf("invalid record id %q (want exp-NNNN)", *id)
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	var rec *record.Record
	for _, r := range recs {
		if r.ID == *id {
			rec = r
			break
		}
	}
	if rec == nil {
		return fmt.Errorf("record %s not found in corpus", *id)
	}

	if *dryRun {
		ok, reason := promote.RepromoteEligible(rec)
		if ok {
			_, _ = fmt.Fprintf(out, "repromote (dry-run): %s %s is re-promotable (needs holding attestation + judge PASS)\n", rec.ID, rec.Path)
		} else {
			_, _ = fmt.Fprintf(out, "repromote (dry-run): %s %s is not re-promotable: %s\n", rec.ID, rec.Path, reason)
		}
		return nil
	}

	lk, err := acquireLoopLock(*corpus)
	if err != nil {
		return err
	}
	defer func() { _ = lk.Release() }()

	judgeURL := getenv("TWICESHY_JUDGE_URL")
	if judgeURL == "" {
		return errors.New("TWICESHY_JUDGE_URL must be set: re-promotion requires a diverse-model judge (it is never bypassed)")
	}
	j, err := judge.NewModelJudge(judge.Config{
		Endpoint: judgeURL, Model: *judgeModel, DrafterModel: *drafterModel,
		System: judge.ProseSystemV2, Think: false,
	})
	if err != nil {
		return fmt.Errorf("configuring judge: %w", err)
	}

	b := repro.NewBroker([]string{repro.PinnedGoImage})
	rv := repro.NewRevalidator(b, *corpus)
	tj := judge.NewTiming(j)
	p := promote.NewPromoter(rv, judge.NewMajority(tj, *votes), *corpus)

	if err := preflight(ctx, b, j); err != nil {
		return err
	}

	origStatus := rec.Status
	outcome, err := p.Repromote(ctx, rec)
	if err != nil {
		return err
	}
	if !outcome.Promoted {
		_, _ = fmt.Fprintf(out, "repromote: held %s — %s\n", rec.ID, outcome.Reason)
		return nil
	}
	if err := writeRecord(*corpus, rec); err != nil {
		return fmt.Errorf("writing record %s: %w", rec.ID, err)
	}
	_, _ = fmt.Fprintf(out, "repromote: restored %s %s -> validated\n", rec.ID, origStatus)
	return nil
}

// runAdapt is the negative direction of ADR-0013 (#0032): for each quarantined
// outcome report, re-run the disputed record's repro plus the report's counter
// through the broker; a reproduced failure + a judge PASS demotes the record to
// stale, while independent non-reproducing reports accumulate and past a
// threshold flag it disputed (escalate). Needs docker+runsc and a judge endpoint
// (TWICESHY_JUDGE_URL); a bare checkout can use -dry-run.
func runAdapt(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("adapt", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	judgeModel := fs.String("judge-model", "", "diverse frontier judge model id (must differ from -drafter-model)")
	drafterModel := fs.String("drafter-model", "", "the model that drafted records; the judge must not share its family")
	dryRun := fs.Bool("dry-run", false, "list the outcome reports and the records they dispute; run no gate/judge, write nothing")
	effect := fs.Bool("effect", false, "with the gate+judge run but write NOTHING, print the would-be status delta per record (effect preview)")
	asJSON := fs.Bool("json", false, "emit a machine-readable run manifest (every record's transition) to stdout instead of prose; the daily audit reads this")
	maxActions := fs.Int("max-actions", defaultMaxActions, "anomaly-halt backstop for UNBOUNDED runs: demotions per run above which the loop halts (0 = off; moot when -max-promotions is set)")
	maxPromotions := fs.Int("max-promotions", defaultMaxPromotions, "throughput cap: stop CLEANLY after this many demote/dispute actions (re-run to continue). 0 = unlimited (#0084)")
	maxActionRate := fs.Float64("max-action-rate", defaultMaxActionRate, "action-rate anomaly: flag a run whose demote-dispute/judged fraction exceeds this (survives a throughput cap, unlike -max-actions). 0 = off (#0085)")
	minSample := fs.Int("min-sample", defaultMinSample, "minimum judged records before -max-action-rate can fire, so a tiny run isn't flagged (#0085)")
	maxRuns := fs.Int("max-runs", 0, "budget cap: max reports processed (broker/judge runs) per invocation (0 = unlimited)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	recs, skipped, err := record.LoadCorpusResilient(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}

	if *dryRun && !*effect {
		n := 0
		for _, rec := range recs {
			if d := runpkg.ReportDisputes(rec); d != "" {
				n++
				_, _ = fmt.Fprintf(out, "  report %s disputes %s\n", rec.ID, d)
			}
		}
		_, _ = fmt.Fprintf(out, "adapt (dry-run): %d outcome report(s); each re-runs the disputed record + the counter and is judge-gated\n", n)
		return nil
	}

	// Single-flight (ADR-0013 §A2): the same lock as promote, so adapt and promote
	// (and two adapt runs) are mutually exclusive — no overlapping double-write.
	lk, err := acquireLoopLock(*corpus)
	if err != nil {
		return err
	}
	defer func() { _ = lk.Release() }()

	// Fail-safe: no judge configured → nothing is ever auto-demoted.
	judgeURL := getenv("TWICESHY_JUDGE_URL")
	if judgeURL == "" {
		return errors.New("TWICESHY_JUDGE_URL must be set: the counter-evidence gate requires a diverse-model judge")
	}
	// System + think are the measured A/B winner (internal/judgeeval, repeat=5):
	// the prose prompt at think=false had 0 false-approve / 0 false-reject, beating
	// the rubric (over-rejects) and think=true (adds false-approves, slower). Pinned
	// here so the validated prompt lives in version control, not the untracked shim.
	// ProseSystemV2 supersedes that measured V1 text with the added usefulness
	// check (#0110); the false-approve/false-reject figures above are V1's — V2
	// awaits its own live A/B re-measurement (gated behind an endpoint, never CI).
	j, err := judge.NewModelJudge(judge.Config{
		Endpoint: judgeURL, Model: *judgeModel, DrafterModel: *drafterModel,
		System: judge.ProseSystemV2, Think: false,
	})
	if err != nil {
		return fmt.Errorf("configuring judge: %w", err)
	}

	b := repro.NewBroker([]string{repro.PinnedGoImage})
	rv := repro.NewRevalidator(b, *corpus)
	runner := brokerCounterRunner{rv: rv}
	tj := judge.NewTiming(j)
	adapter := promote.NewAdapter(tj)

	// Preflight (ADR-0013 §A3): abort before walking the corpus if the sandbox
	// substrate or the judge endpoint is down, rather than failing mid-run.
	if err := preflight(ctx, b, j); err != nil {
		return err
	}

	g := guardrailsFrom(getenv, *maxActions, *maxPromotions, *maxRuns, *maxActionRate, *minSample)
	runID := newRunID()
	runLog := newRunLogger(runID)
	// Guardrail trips fire to TWICESHY_ALERT_URL (ntfy) when set; unset = no-op.
	alerter := notify.New(getenv("TWICESHY_ALERT_URL"), getenv("NTFY_TOKEN"), runLog)
	persist := writeRecord
	if *effect {
		persist = func(string, *record.Record) error { return nil }
	}
	return runpkg.RunStage(ctx, out, getenv, runLog, "adapt", runID, *asJSON, *effect,
		func() *judge.JudgeStats { return surfaceJudgeStats(runLog, tj) },
		func(proseOut io.Writer) (runpkg.AdaptStats, []promote.RecordAction, error) {
			// Sweep a crashed prior run's leaked sandbox resources before the walk (#0052).
			startupReap(ctx, "adapt", *effect, runLog, proseOut)
			logSkippedPoison(runLog, proseOut, "adapt", skipped)
			return runpkg.AdaptCorpus(ctx, *corpus, recs, runner, adapter, persist, g, runLog, alerter, proseOut, runpkg.JournalPathForRun(*corpus, "adapt", *effect))
		},
		nil,
		runpkg.AdaptManifest,
		runpkg.WriteAdaptSummary)
}

// brokerCounterRunner re-runs the original's repro in the sandbox and, when the
// report carries its own runnable counter-repro, runs that too. Synthesizing a
// counter-repro from free-text evidence is a drafter-level concern (ADR-0013 §8)
// and is deferred: a prose-only report yields an inconclusive counter, so the
// demote path then relies on the original's own repro having broken, while the
// non-reproducing accumulation → disputed path still applies.
type brokerCounterRunner struct {
	rv *repro.Revalidator
}

func (b brokerCounterRunner) Run(ctx context.Context, original, report *record.Record) (promote.CounterEvidence, error) {
	_, atts, err := b.rv.RunWithAttestations(ctx, []*record.Record{original})
	if err != nil {
		return promote.CounterEvidence{}, err
	}
	ev := promote.CounterEvidence{Counter: repro.Attestation{RecordID: report.ID, Inconclusive: true}, CounterRepro: reportEvidence(report)}
	if len(atts) > 0 {
		ev.Original = atts[0]
	}
	if report.Guard != nil && record.HasPositiveRepro(report) {
		_, catts, err := b.rv.RunWithAttestations(ctx, []*record.Record{report})
		if err != nil {
			return promote.CounterEvidence{}, err
		}
		if len(catts) > 0 {
			ev.Counter = catts[0]
		}
	}
	return ev, nil
}

// reportEvidence pulls the human evidence out of a report as the judge's
// context (the CounterRepro). It is prose, not a runnable script — synthesizing
// a runnable counter-repro from it is deferred (ADR-0013 §8), so a prose-only
// report's counter stays inconclusive and can only accumulate toward `disputed`,
// never demote. The judge reads it only when the original's own repro broke.
func reportEvidence(report *record.Record) string {
	if report.Resolution != nil && len(report.Resolution.DeadEnds) > 0 {
		return report.Resolution.DeadEnds[0].WhyItFailed
	}
	return report.Body
}

func nextIDForCorpus(ctx context.Context, corpus, base string, floors ...int) (string, error) {
	tmp, err := os.CreateTemp("", "twiceshy-nextid-*.db")
	if err != nil {
		return "", err
	}
	db := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(db) }()
	ix, err := index.Open(db)
	if err != nil {
		return "", err
	}
	defer func() { _ = ix.Close() }()
	return ingest.NextIDWithBase(ctx, ix, corpus, base, floors...)
}

func openPRFloors(ctx context.Context, corpusRoot string, openPRs bool, getenv func(string) string) ([]int, error) {
	if !openPRs {
		return nil, nil
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	maxID, err := forgejoOpenPRMaxID(ctx, corpusRoot, getenv, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return []int{maxID}, nil
}

// serveIDFloorResolver is the live-write counterpart to openPRFloors. Config
// resolution happens inside the cached callback so env-first Forgejo settings
// work in containers without a usable git origin. The server caches calls for
// its short TTL and degrades loudly to base/local allocation on any error.
func serveIDFloorResolver(corpusRoot string, getenv func(string) string) server.IDFloorResolver {
	return func(ctx context.Context) (int, error) {
		return forgejoOpenPRMaxID(ctx, corpusRoot, getenv, 5*time.Second)
	}
}

func forgejoOpenPRMaxID(ctx context.Context, corpusRoot string, getenv func(string) string, timeout time.Duration) (int, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	api, token, err := ingest.ForgejoAPIFromOrigin(ctx, corpusRoot, getenv)
	if err != nil {
		return 0, err
	}
	client := &http.Client{
		Timeout: timeout,
		// A redirect would strip the Authorization header cross-host and
		// resurface as a misleading 401; an API root that redirects is a
		// misconfig — surface the 3xx itself via the non-2xx check instead.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return ingest.OpenPRMaxID(ctx, client, api, token)
}

// runIntakeReports drains the report queue (ADR-0013 §E1, #0042): each queued
// outcome report becomes a quarantined counter-record written under experience/,
// so the next `adapt` adjudicates it with no human paste-PR step. Ids are
// allocated against the live corpus sequentially within the batch, so reports
// queued before this drain never collide. A malformed queue entry is logged and
// removed (it cannot wedge the nightly drain); a write failure aborts so the
// entry is retried next run.
func runIntakeReports(args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("intake-reports", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	queue := fs.String("queue", "", "report queue directory written by `serve -report-queue` (required)")
	base := fs.String("base", "", "base git ref for merge-safe id allocation")
	openPRs := fs.Bool("open-prs", false, "also allocate ids above records on open corpus PRs (Forgejo API, #0121)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *queue == "" {
		return errors.New("intake-reports requires -queue <dir> (the directory serve enqueues reports into)")
	}

	if _, err := record.LoadCorpus(*corpus); err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	files, err := spool.List(*queue)
	if err != nil {
		return fmt.Errorf("listing report queue: %w", err)
	}

	// Idle ticks (empty queue) skip the scan — no network dependency for an
	// id the drain never uses.
	floors, err := openPRFloors(context.Background(), *corpus, *openPRs && len(files) > 0, getenv)
	if err != nil {
		return fmt.Errorf("getting open PR floors: %w", err)
	}
	id, err := nextIDForCorpus(context.Background(), *corpus, *base, floors...)
	if err != nil {
		return fmt.Errorf("allocating next id: %w", err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	intaken, skipped := 0, 0
	for _, f := range files {
		rep, err := spool.Read(f)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  skip %s: unreadable queue entry (%v)\n", filepath.Base(f), err)
			_ = spool.Remove(f)
			skipped++
			continue
		}
		meta := ingest.Meta{ID: id, Author: rep.Author, Now: today}
		if rep.Session != "" {
			s := rep.Session
			meta.Session = &s
		}
		rec, err := ingest.BuildReport(ingest.ReportInput{RecordID: rep.RecordID, Outcome: rep.Outcome, Evidence: rep.Evidence}, meta)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  skip %s: invalid report (%v)\n", filepath.Base(f), err)
			_ = spool.Remove(f)
			skipped++
			continue
		}
		if err := writeRecord(*corpus, rec); err != nil {
			// A write failure is environmental — leave the entry queued for retry.
			return fmt.Errorf("writing counter-record for %s: %w", rep.RecordID, err)
		}
		_ = spool.Remove(f)
		id = ingest.BumpID(id)
		intaken++
		_, _ = fmt.Fprintf(out, "  intake %s -> %s (disputes %s)\n", filepath.Base(f), rec.ID, rep.RecordID)
	}
	_, _ = fmt.Fprintf(out, "intake-reports: materialized %d report(s) into experience/, %d skipped\n", intaken, skipped)
	return nil
}

// runReport enqueues an outcome dispute into the report intake queue without the
// server (#0044): the daily audit (or an operator) files a disagreement, then
// `intake-reports` materializes it and `adapt` adjudicates.
func runReport(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	id := fs.String("id", "", "disputed record id (required, exp-NNNN)")
	outcome := fs.String("outcome", "audit-disagreement", "short outcome label")
	evidence := fs.String("evidence", "", "reason / failing detail")
	queue := fs.String("queue", "", "report queue directory (required; same as intake-reports -queue)")
	author := fs.String("author", "daily-audit", "provenance author on the counter-record")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("report requires -id <exp-NNNN>")
	}
	if !record.ValidID(*id) {
		return fmt.Errorf("invalid record id %q (want exp-NNNN)", *id)
	}
	if *queue == "" {
		return errors.New("report requires -queue <dir> (the directory intake-reports drains)")
	}
	r := spool.Report{
		RecordID:   *id,
		Outcome:    *outcome,
		Evidence:   *evidence,
		Author:     *author,
		ReportedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	path, err := spool.Enqueue(*queue, r)
	if err != nil {
		return fmt.Errorf("enqueue report: %w", err)
	}
	_, _ = fmt.Fprintf(out, "report: queued dispute against %s (%s)\n", *id, path)
	return nil
}

// runPack builds a distributable experience pack (#0007, ADR-0002 §4). It
// selects validated records, optionally enforces commercial-pack license
// cleanliness (pack.BuildManifest), and writes the included records plus a
// MANIFEST.json and ATTRIBUTION.md to -out. Pure selection lives in
// internal/pack; this edge does the I/O.
func runPack(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	outDir := fs.String("out", "", "output directory for the built pack")
	commercial := fs.Bool("commercial", false, "build a commercial pack: exclude copyleft/contract-encumbered records")
	includeQ := fs.Bool("include-quarantined", false, "include not-yet-validated records (inspection only)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *outDir == "" {
		return errors.New("pack requires -out <dir>")
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	m := pack.BuildManifest(recs, *commercial, *includeQ)

	byID := make(map[string]*record.Record, len(recs))
	for _, r := range recs {
		byID[r.ID] = r
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	for _, id := range m.Included {
		r := byID[id]
		md, err := record.Marshal(r)
		if err != nil {
			return err
		}
		dst, err := safeJoin(*outDir, r.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, md, 0o644); err != nil {
			return err
		}
	}

	mj, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "MANIFEST.json"), append(mj, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "ATTRIBUTION.md"), pack.NoticeDocument(m), 0o644); err != nil {
		return err
	}

	kind := "open"
	if *commercial {
		kind = "commercial"
	}
	_, _ = fmt.Fprintf(out, "pack (%s): included %d, excluded %d, source/license notices %d -> %s\n",
		kind, len(m.Included), len(m.Excluded), len(m.Attribution), *outDir)
	return nil
}

// runDoctor runs a store-hygiene doctor over the corpus and prints its proposed
// deltas. Doctors are report-only (ADR-0001 §7, ADR-0010); they never mutate
// the corpus — a human applies the proposal via PR.
func runDoctor(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: twiceshy doctor <name> [flags] (doctors: staleness)")
	}
	name := args[0]
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	eolURL := fs.String("endoflife-url", doctor.DefaultEOLBase, "endoflife.date API base; empty runs only the valid.until check")
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	attest := fs.Bool("attest", false, "(revalidate) also emit the structured attestations as JSON")
	if err := parseFlags(fs, args[1:]); err != nil {
		return err
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}

	// revalidate runs untrusted repros in the gVisor broker, so it has a distinct
	// path: it needs a corpus root + Docker/runsc, and it emits attestations the
	// reviewer reads before flipping `validated` in the PR. (Report-only.)
	if name == "revalidate" {
		return runRevalidate(ctx, *corpus, recs, *asJSON, *attest, out)
	}

	var d doctor.Doctor
	switch name {
	case "staleness":
		var eol doctor.EOLSource
		if *eolURL != "" {
			eol = doctor.NewEndOfLifeSource(*eolURL)
		}
		d = doctor.NewStaleness(eol, time.Now().UTC())
	default:
		return fmt.Errorf("unknown doctor %q (want: staleness, revalidate)", name)
	}

	rep, err := d.Run(ctx, recs)
	if err != nil {
		return err
	}

	if *asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
		return nil
	}
	printReport(out, rep)
	return nil
}

// runRevalidate runs the execution-validation harness (#0020) over the corpus:
// each record's repro test-set runs in the gVisor broker, and the doctor proposes
// promotion/demotion plus a structured attestation. Report-only.
func runRevalidate(ctx context.Context, corpus string, recs []*record.Record, asJSON, attest bool, out io.Writer) error {
	images := make([]string, 0, len(repro.DefaultGoMatrix))
	for _, e := range repro.DefaultGoMatrix {
		images = append(images, e.Image)
	}
	rv := repro.NewRevalidator(repro.NewBroker(images), corpus)
	rep, atts, err := rv.RunWithAttestations(ctx, recs)
	if err != nil {
		return err
	}
	if asJSON {
		payload := struct {
			Report       doctor.Report       `json:"report"`
			Attestations []repro.Attestation `json:"attestations"`
		}{rep, atts}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
		return nil
	}
	printReport(out, rep)
	if attest {
		b, err := json.MarshalIndent(atts, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "\nattestations:\n%s\n", string(b))
	}
	return nil
}

// usageEqual compares two usage pointers for value equality.
func usageEqual(a, b *record.Usage) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Retrieved != b.Retrieved || a.Pushed != b.Pushed || a.ConfirmedHelpful != b.ConfirmedHelpful {
		return false
	}
	switch {
	case a.LastHit == nil && b.LastHit == nil:
		return true
	case a.LastHit == nil || b.LastHit == nil:
		return false
	default:
		return *a.LastHit == *b.LastHit
	}
}

// runUsageFlush materializes SQLite usage counters into each record's
// provenance.usage in the markdown corpus (delta-only, idempotent).
func runUsageFlush(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("usage-flush", flag.ContinueOnError)
	c := addCommonFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, err := index.Open(c.db)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	recs, err := record.LoadCorpus(c.corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	usage, err := ix.AllUsage(ctx)
	if err != nil {
		return err
	}

	updated := 0
	for _, r := range recs {
		u, ok := usage[r.ID]
		if !ok {
			continue
		}
		if usageEqual(r.Provenance.Usage, &u) {
			continue
		}
		r.Provenance.Usage = &u
		if err := writeRecord(c.corpus, r); err != nil {
			return err
		}
		updated++
	}
	_, _ = fmt.Fprintf(out, "usage-flush: updated %d of %d record(s) from %s\n", updated, len(recs), c.db)
	return nil
}

// runGoldAdd turns an audit-miss record into one gold.yaml case stanza (#0058).
func runGoldAdd(_ context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("gold-add", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	recordPath := fs.String("record", "", "corpus-relative or absolute path to the audit-miss record markdown")
	id := fs.String("id", "", "gold case id (e.g. G42)")
	mode := fs.String("mode", "", "gold case mode (approve, poison, scope, meaning, license)")
	rationale := fs.String("rationale", "", "ground-truth rationale for the label")
	checks := fs.String("checks", "", "comma-separated want_failing_checks (reject cases only)")
	goldFile := fs.String("gold-file", "internal/judgeeval/gold.yaml", "path to gold.yaml (for -append)")
	appendFile := fs.Bool("append", false, "append the stanza to -gold-file instead of printing")
	advisoryAudit := fs.String("advisory-audit", "", "bulk: regenerate advisory-gold.yaml from a Sonnet advisory-audit JSON (advisory-class gold cases, no repro, #0074)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *advisoryAudit != "" {
		return runGoldAddAdvisory(*corpus, *advisoryAudit, *goldFile, out)
	}
	if *recordPath == "" || *id == "" || *mode == "" || *rationale == "" {
		return errors.New("gold-add: -record, -id, -mode, and -rationale are required")
	}

	rec, err := loadRecordForGoldAdd(*corpus, *recordPath)
	if err != nil {
		return fmt.Errorf("gold-add: loading record: %w", err)
	}
	repros, err := loadRecordRepros(*corpus, rec)
	if err != nil {
		return err
	}
	if len(repros) == 0 {
		return fmt.Errorf("gold-add: %s has no guard.repro or guard.repros — a gold case needs at least one repro", rec.Path)
	}

	var checkList []string
	if strings.TrimSpace(*checks) != "" {
		for _, c := range strings.Split(*checks, ",") {
			if c = strings.TrimSpace(c); c != "" {
				checkList = append(checkList, c)
			}
		}
	}

	stanza, err := judgeeval.GoldCaseStanza(judgeeval.GoldStanzaInput{
		ID:        *id,
		Mode:      *mode,
		Rationale: *rationale,
		Checks:    checkList,
		Record:    rec,
		Repros:    repros,
	})
	if err != nil {
		return fmt.Errorf("gold-add: %w", err)
	}

	if *appendFile {
		f, err := os.OpenFile(*goldFile, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("gold-add: opening %s: %w", *goldFile, err)
		}
		defer func() { _ = f.Close() }()
		if _, err := f.WriteString("\n" + stanza); err != nil {
			return fmt.Errorf("gold-add: appending to %s: %w", *goldFile, err)
		}
		_, _ = fmt.Fprintf(out, "gold-add: appended case %s to %s — re-run judge-eval to re-measure\n", *id, *goldFile)
		return nil
	}
	_, _ = fmt.Fprintln(out, stanza)
	_, _ = fmt.Fprintf(out, "\n# paste under cases: in %s, then re-run judge-eval to re-measure\n", *goldFile)
	return nil
}

// runGoldAddAdvisory bulk-regenerates the advisory gold set (#0074): it reads a Sonnet
// advisory-audit JSON, resolves each audited record from the corpus, and writes the 85
// verdicts as advisory-class gold cases (no repro) into advisory-gold.yaml, which
// LoadGold merges with the prose gold.yaml. The whole file is rewritten deterministically
// (idempotent), so re-running on an updated audit refreshes the embed.
func runGoldAddAdvisory(corpus, auditPath, goldFile string, out io.Writer) error {
	data, err := os.ReadFile(auditPath)
	if err != nil {
		return fmt.Errorf("gold-add: reading advisory audit: %w", err)
	}
	var audit judgeeval.AdvisoryAudit
	if err := json.Unmarshal(data, &audit); err != nil {
		return fmt.Errorf("gold-add: parsing advisory audit %s: %w", auditPath, err)
	}
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		return fmt.Errorf("gold-add: loading corpus: %w", err)
	}
	byID := make(map[string]*record.Record, len(recs))
	for _, r := range recs {
		byID[r.ID] = r
	}
	doc, err := judgeeval.BuildAdvisoryGold(audit, func(id string) (*record.Record, error) {
		if r, ok := byID[id]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("record %s not in corpus", id)
	})
	if err != nil {
		return fmt.Errorf("gold-add: %w", err)
	}
	// The advisory cases live in their own embed so the prose gold.yaml stays readable;
	// redirect off the gold.yaml default unless the operator named an explicit target.
	target := goldFile
	if target == "internal/judgeeval/gold.yaml" {
		target = "internal/judgeeval/advisory-gold.yaml"
	}
	if err := os.WriteFile(target, []byte(doc), 0o644); err != nil {
		return fmt.Errorf("gold-add: writing %s: %w", target, err)
	}
	_, _ = fmt.Fprintf(out, "gold-add: wrote %d advisory gold case(s) to %s — re-run judge-eval to measure\n", len(audit.Approved)+len(audit.Rejected), target)
	return nil
}

func loadRecordForGoldAdd(corpus, recordPath string) (*record.Record, error) {
	if !filepath.IsAbs(recordPath) {
		return record.ParseFile(corpus, recordPath)
	}
	src, err := os.ReadFile(recordPath)
	if err != nil {
		return nil, err
	}
	rel := recordPath
	if r, err := filepath.Rel(corpus, recordPath); err == nil {
		r = filepath.ToSlash(r)
		if !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	return record.Parse(rel, src)
}

const maxReproContentBytes = 64 << 10

func loadRecordRepros(corpus string, rec *record.Record) ([]judge.ReproArtifact, error) {
	if rec.Guard == nil {
		return nil, nil
	}
	var arts []judge.ReproArtifact
	add := func(rp, kind, label string) error {
		content, err := readReproContent(corpus, rp)
		if err != nil {
			return fmt.Errorf("gold-add: repro %s: %w", rp, err)
		}
		arts = append(arts, judge.ReproArtifact{Path: rp, Kind: kind, Label: label, Content: content})
		return nil
	}
	if rec.Guard.Repro != nil && *rec.Guard.Repro != "" {
		if err := add(*rec.Guard.Repro, "positive", ""); err != nil {
			return nil, err
		}
	}
	for _, rp := range rec.Guard.Repros {
		if err := add(rp.Path, rp.Kind, rp.Label); err != nil {
			return nil, err
		}
	}
	return arts, nil
}

func readReproContent(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repro path %q escapes the corpus root", rel)
	}
	abs := filepath.Join(root, clean)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "repro.sh")
	}
	f, err := os.Open(abs) //nolint:gosec // abs is rooted at the corpus and escape-checked above
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxReproContentBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// runEval runs the retrieval-effectiveness eval (#0005) over the corpus: it
// drives the validated-only pull path with queries taken from each behavioral
// record's error signatures + summary, and reports recall@k / MRR / near-miss.
// It is the store's evidence gate — does the corpus surface the right trap?
func runEval(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	c := addCommonFlags(fs)
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	push := fs.Bool("push", false, "run the push-precision eval (off-domain prompts must inject nothing) instead of pull recall")
	usage := fs.Bool("usage", false, "run the usage-judge precision/recall eval over the gold set")
	usageCases := fs.String("usage-cases", "", "path to a JSON []UsageCase file for -usage; replaces the built-in synthetic gold set (real-traffic transcripts stay outside the repo)")
	model := fs.String("analyzer-model", "", "off-pool model id for -usage (default: TWICESHY_RETRO_MODEL, else TWICESHY_JUDGE_MODEL)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *usage {
		return runEvalUsage(ctx, getenv, out, *asJSON, *model, *usageCases)
	}
	recs, err := record.LoadCorpus(c.corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	ix, err := index.Open(c.db)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	if err := ix.Rebuild(ctx, recs, c.repo); err != nil {
		return err
	}
	if *push {
		return runEvalPush(ctx, ix, out, *asJSON)
	}
	cases := eval.Cases(recs)
	rep, err := eval.Run(ctx, ix, cases, index.MaxK)
	if err != nil {
		return err
	}
	if *asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
		return nil
	}
	_, _ = fmt.Fprintf(out, "eval: %d cases over the validated corpus (k=%d)\n", rep.Cases, rep.K)
	_, _ = fmt.Fprintf(out, "  recall@k:      %.1f%% (%d/%d found)\n", rep.RecallAtK*100, rep.Found, rep.Cases)
	_, _ = fmt.Fprintf(out, "  MRR:           %.3f\n", rep.MRR)
	_, _ = fmt.Fprintf(out, "  near-miss:     %.1f%% (wrong card on top)\n", rep.NearMissRate*100)
	for _, r := range rep.Results {
		if !r.Found || r.NearMiss() {
			status := "MISS"
			if r.Found {
				status = fmt.Sprintf("rank %d", r.Rank)
			}
			_, _ = fmt.Fprintf(out, "    [%s] %s (%s) %q -> %v\n", status, r.RecordID, r.Source, truncate(r.Query, 50), r.Returned)
		}
	}
	return nil
}

// runEvalPush reports the push channel's precision (off-domain prompts must inject
// nothing) and recall (genuine traps must surface) — the #0005 measurement that
// gates the push channel. Returns a non-zero error on any false injection so a
// script/CI can fail on a precision regression.
func runEvalPush(ctx context.Context, ix *index.Index, out io.Writer, asJSON bool) error {
	cases := append(eval.PushNegatives(), eval.PushPositives()...)
	rep, err := eval.RunPush(ctx, ix, cases)
	if err != nil {
		return err
	}
	if asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
	} else {
		_, _ = fmt.Fprintf(out, "push eval over the validated corpus\n")
		_, _ = fmt.Fprintf(out, "  precision: %.1f%% (%d/%d off-domain prompts injected — want 0)\n",
			rep.Precision()*100, rep.FalseInjections, rep.Negatives)
		_, _ = fmt.Fprintf(out, "  recall:    %.1f%% (%d/%d traps surfaced)\n",
			rep.Recall()*100, rep.Recalled, rep.Positives)
		for _, l := range rep.Leaks {
			_, _ = fmt.Fprintf(out, "    [LEAK] %s\n", l)
		}
		for _, m := range rep.Misses {
			_, _ = fmt.Fprintf(out, "    [MISS] %s\n", m)
		}
	}
	if rep.FalseInjections > 0 {
		return fmt.Errorf("push precision regression: %d/%d off-domain prompts injected a card", rep.FalseInjections, rep.Negatives)
	}
	return nil
}

// runEvalUsage reports how accurately the usage judge labels served cards as
// used vs ignored over the hand-labeled gold set (#0069 acceptance 3) — the
// built-in synthetic set, or a real-traffic []UsageCase JSON via casesPath.
func runEvalUsage(ctx context.Context, getenv func(string) string, out io.Writer, asJSON bool, model, casesPath string) error {
	cfg, err := modelConfigFromEnv(getenv, model, 0)
	if err != nil {
		return err
	}
	judge, err := retro.NewModelUsageJudge(cfg)
	if err != nil {
		return err
	}
	cases := eval.UsageGold()
	if casesPath != "" {
		if cases, err = eval.LoadUsageCases(casesPath); err != nil {
			return err
		}
	}
	rep, err := eval.RunUsage(ctx, judge, cases)
	if err != nil {
		return err
	}
	if asJSON {
		payload := struct {
			eval.UsageReport
			Precision float64 `json:"precision"`
			Recall    float64 `json:"recall"`
		}{rep, rep.Precision(), rep.Recall()}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
		return nil
	}
	_, _ = fmt.Fprintf(out, "usage eval over the gold set (%d cases)\n", rep.Cases)
	_, _ = fmt.Fprintf(out, "  precision: %.1f%% (%d/%d judge-positives correct)\n",
		rep.Precision()*100, rep.TP, rep.TP+rep.FP)
	_, _ = fmt.Fprintf(out, "  recall:    %.1f%% (%d/%d gold-used found)\n",
		rep.Recall()*100, rep.TP, rep.TP+rep.FN)
	_, _ = fmt.Fprintf(out, "  TP/FP/FN:  %d/%d/%d\n", rep.TP, rep.FP, rep.FN)
	for _, m := range rep.Mismatches {
		_, _ = fmt.Fprintf(out, "    [MISMATCH] %s\n", m)
	}
	return nil
}

func truncate(s string, n int) string {
	// n is a character budget; index by rune so a multibyte codepoint is never
	// split into invalid UTF-8.
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// runJudgeEval drives the diverse-model judge against the labelled gold set
// (internal/judgeeval) and A/Bs the prose vs rubric system prompt at think
// off/on, scoring the fail-UNSAFE direction (false-approve rate) so the operator
// can install the winning prompt. It hits the live shim (TWICESHY_JUDGE_URL) — it
// is an offline tuning tool, never part of CI (no network there).
func runJudgeEval(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("judge-eval", flag.ContinueOnError)
	model := fs.String("model", "gpt-oss:20b", "judge model id (the shim's upstream model)")
	drafterModel := fs.String("drafter-model", "", "drafter model for the anti-monoculture check; empty skips it")
	repeat := fs.Int("repeat", 1, "samples per case; the majority decision is scored (smooths boundary cases)")
	confirm := fs.Bool("confirm", false, "adaptive sampling: sample every case -repeat times, then re-sample only the boundary (flipped) cases up to 3×-repeat — same headline at ~3× fewer judge calls (#0057)")
	timeout := fs.Int("timeout", 90, "per-call HTTP timeout in seconds (raise for think=true)")
	configs := fs.String("configs", "all", "comma list of configs to run, or all: "+judgeeval.ConfigNames())
	asJSON := fs.Bool("json", false, "emit the full report as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	url := getenv("TWICESHY_JUDGE_URL")
	if url == "" {
		return errors.New("TWICESHY_JUDGE_URL must be set: judge-eval drives the live judge shim")
	}
	selected, err := judgeeval.SelectConfigs(*configs)
	if err != nil {
		return err
	}
	cases, err := judgeeval.LoadGold()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Duration(*timeout) * time.Second}

	var results []judgeeval.NamedResult
	for _, cf := range selected {
		j, err := judge.NewModelJudge(judge.Config{
			Endpoint: url, Model: *model, DrafterModel: *drafterModel,
			System: cf.System, Think: cf.Think, Client: client,
		})
		if err != nil {
			return fmt.Errorf("configuring judge for %s: %w", cf.Name, err)
		}
		if !*asJSON {
			if *confirm {
				_, _ = fmt.Fprintf(out, "running %s (%d cases × %d confirm, up to %d) …\n", cf.Name, len(cases), *repeat, 3*(*repeat))
			} else {
				_, _ = fmt.Fprintf(out, "running %s (%d cases × %d) …\n", cf.Name, len(cases), *repeat)
			}
		}
		var rep judgeeval.Result
		if *confirm {
			rep, err = judgeeval.RunConfirm(ctx, j, cases, *repeat, 3*(*repeat))
		} else {
			rep, err = judgeeval.Run(ctx, j, cases, *repeat)
		}
		if err != nil {
			return fmt.Errorf("running %s: %w", cf.Name, err)
		}
		results = append(results, judgeeval.NamedResult{Name: cf.Name, Result: rep})
	}

	winner := -1
	for i := range results {
		if winner < 0 || judgeeval.Better(results[i].Result, results[winner].Result) {
			winner = i
		}
	}

	if *asJSON {
		payload := struct {
			Configs []judgeeval.NamedResult `json:"configs"`
			Winner  string                  `json:"winner"`
		}{results, judgeeval.ResultName(results, winner)}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(b))
		return nil
	}

	_, _ = fmt.Fprintf(out, "\njudge-eval: %d gold cases, repeat=%d, model=%s\n", len(cases), *repeat, *model)
	if *confirm {
		uniform := 3 * (*repeat) * len(cases)
		for _, nr := range results {
			_, _ = fmt.Fprintf(out, "  %s: %d judge calls (confirm) vs %d uniform\n", nr.Name, nr.Result.JudgeCalls, uniform)
		}
	}
	_, _ = fmt.Fprintf(out, "%-16s %12s %12s %8s %9s  %s %6s\n", "config", "false-appr", "false-rej", "errors", "accuracy", "check-recall", "flips")
	for i, nr := range results {
		r := nr.Result
		mark := "  "
		if i == winner {
			mark = "★ "
		}
		_, _ = fmt.Fprintf(out, "%s%-14s %5d %5.0f%% %5d %5.0f%% %8d %8.0f%% %10.0f%% %6d\n",
			mark, nr.Name,
			r.FalseApproves, r.FalseApproveRate*100,
			r.FalseRejects, r.FalseRejectRate*100,
			r.Errors, r.Accuracy*100, r.CheckRecall*100, r.Flips)
	}
	_, _ = fmt.Fprintf(out, "\nwinner: %s (lowest false-approve, then false-reject, then errors)\n", judgeeval.ResultName(results, winner))

	// Detail for the winner: which gold cases slipped, so the failure is legible.
	w := results[winner].Result
	judgeeval.PrintMisses(out, "FALSE-APPROVE (fail-unsafe — would auto-promote)", w.Outcomes, func(o judgeeval.Outcome) bool { return o.FalseApprove })
	judgeeval.PrintMisses(out, "false-reject (over-conservative — good record blocked)", w.Outcomes, func(o judgeeval.Outcome) bool { return o.FalseReject })
	judgeeval.PrintMisses(out, "errors (no verdict)", w.Outcomes, func(o judgeeval.Outcome) bool { return o.Errored })
	judgeeval.PrintMisses(out, "flipped (judge disagreed with itself across samples)", w.Outcomes, func(o judgeeval.Outcome) bool { return o.Flipped })
	return nil
}

func printReport(out io.Writer, rep doctor.Report) {
	_, _ = fmt.Fprintf(out, "doctor %s: %d finding(s)\n", rep.Doctor, len(rep.Findings))
	for _, f := range rep.Findings {
		_, _ = fmt.Fprintf(out, "  %s (%s)\n    issue:    %s\n    proposal: %s\n", f.RecordID, f.Path, f.Issue, f.Proposal)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

// Command twiceshy is the experience service binary (ADR-0001 §9): one Go
// process serving the Phase 1 read path, plus corpus tooling.
//
//	twiceshy index  -corpus <dir> -db <file>          rebuild the derived index
//	twiceshy serve  -corpus <dir> -db <file> -addr …  rebuild, then serve MCP
//	twiceshy ingest <source> -corpus <dir> -db <file> import quarantined records
//	twiceshy draft  -corpus <dir>                     draft+gate+attach repros (needs docker+runsc)
//	twiceshy pack   -corpus <dir> -out <dir>          build a distributable pack
//	twiceshy doctor <name> -corpus <dir>              run a doctor (staleness | revalidate)
//	twiceshy eval   -corpus <dir> -db <file>          report retrieval recall@k / MRR
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
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/eval"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
	"github.com/dotts-h/twiceshy/internal/server"
)

// errUsage marks a flag parse error whose specifics the flag package already
// printed to stderr; main maps it to exit code 2 without re-printing (no double
// message), distinct from `-h` (flag.ErrHelp → exit 0, not an error).
var errUsage = errors.New("invalid flags")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch err := run(ctx, os.Args[1:], os.Stdout, os.Getenv); {
	case err == nil, errors.Is(err, flag.ErrHelp): // -h: usage already on stderr; success
	case errors.Is(err, errUsage):
		os.Exit(2) // flag already printed the details
	default:
		fmt.Fprintln(os.Stderr, "twiceshy:", err)
		os.Exit(1)
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
		return errors.New("usage: twiceshy <index|serve|ingest|draft|pack|doctor|eval> [flags]")
	}
	switch args[0] {
	case "index":
		return runIndex(ctx, args[1:], out)
	case "serve":
		return runServe(ctx, args[1:], out, getenv)
	case "pack":
		return runPack(args[1:], out)
	case "ingest":
		return runIngest(ctx, args[1:], out)
	case "draft":
		return runDraft(ctx, args[1:], out)
	case "doctor":
		return runDoctor(ctx, args[1:], out)
	case "eval":
		return runEval(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown subcommand %q (want index, serve, ingest, draft, pack, doctor, or eval)", args[0])
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

// buildIndex loads + validates the corpus and rebuilds the derived index.
// Rebuild-on-start keeps the index trivially consistent with the records —
// the index is never the source of truth.
func buildIndex(ctx context.Context, c *commonFlags) (*index.Index, int, error) {
	recs, err := record.LoadCorpus(c.corpus)
	if err != nil {
		return nil, 0, fmt.Errorf("loading corpus: %w", err)
	}
	ix, err := index.Open(c.db)
	if err != nil {
		return nil, 0, err
	}
	if err := ix.Rebuild(ctx, recs, c.repo); err != nil {
		_ = ix.Close()
		return nil, 0, err
	}
	// Dense retrieval (pull-only) — populate embeddings when configured; cached
	// by content hash so a rebuild only re-embeds changed records (ADR-0009).
	if emb := embedderFor(c); emb != nil {
		if err := ix.EmbedCorpus(ctx, recs, emb); err != nil {
			_ = ix.Close()
			return nil, 0, fmt.Errorf("embedding corpus: %w", err)
		}
	}
	return ix, len(recs), nil
}

func runIndex(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	c := addCommonFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, n, err := buildIndex(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	_, _ = fmt.Fprintf(out, "indexed %d records into %s\n", n, c.db)
	return nil
}

func runServe(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	c := addCommonFlags(fs)
	addr := fs.String("addr", ":8722", "listen address")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	token := getenv("TWICESHY_TOKEN")
	if token == "" {
		return errors.New("TWICESHY_TOKEN must be set; the server has no unauthenticated mode")
	}

	ix, n, err := buildIndex(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	handler, err := server.New(server.Config{Index: ix, Token: token, Repo: c.repo, Embedder: embedderFor(c)})
	if err != nil {
		return err
	}

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
		return err
	}
	return nil
}

// importSource resolves a CLI source selector to its adapter.
func importSource(name string) (ingest.Source, error) {
	switch name {
	case "go":
		return ingest.NewGoSource(), nil
	case "osv":
		return ingest.NewOSVSource(), nil
	case "py":
		return ingest.NewPySource(), nil
	case "osv-live":
		return ingest.NewOSVLiveSource(), nil
	default:
		return nil, fmt.Errorf("unknown ingest source %q (want: go, osv, osv-live, py)", name)
	}
}

// runIngest imports quarantined records from a license-clean source (#0007).
// Records are deduped against the corpus (via ingest.Prepare) and within the
// batch, then written to disk for a human to open as a PR — git is the trust
// boundary, so nothing is born validated and nothing reaches the push channel.
func runIngest(ctx context.Context, args []string, out io.Writer) error {
	// The source is the first positional; flags follow it. Go's flag package
	// stops at the first non-flag arg, so pull the source off the front before
	// parsing (otherwise `ingest go -corpus X` would leave -corpus unparsed).
	if len(args) < 1 {
		return errors.New("usage: twiceshy ingest <source> [flags] (sources: go, osv, osv-live, py)")
	}
	src, err := importSource(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	c := addCommonFlags(fs)
	dryRun := fs.Bool("dry-run", false, "classify and report, but write no files")
	limit := fs.Int("limit", 0, "max new records to write this run (0 = unlimited); bounds a scheduled import")
	author := fs.String("author", "twiceshy-importer", "provenance author recorded on imported records")
	if err := parseFlags(fs, args[1:]); err != nil {
		return err
	}

	ix, _, err := buildIndex(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	drafts, err := src.Drafts(ctx)
	if err != nil {
		return err
	}

	id, err := ix.NextID(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02")

	var created, skipped, flagged int
	seen := map[string]bool{} // within-batch dedup, keyed by the primary signal
	for _, d := range drafts {
		key := batchKey(d)
		if seen[key] {
			skipped++
			continue
		}
		outcome, err := ingest.Prepare(ctx, ix, c.repo, d,
			ingest.Meta{ID: id, Author: *author, Now: now, IncludeQuarantined: true})
		if err != nil {
			return fmt.Errorf("ingest %q: %w", d.Title, err)
		}
		if outcome.Record == nil { // Known — already in the corpus
			skipped++
			continue
		}
		seen[key] = true
		rec := outcome.Record
		flag := ""
		if len(rec.Provenance.SecurityFlags) > 0 {
			flagged++
			flag = fmt.Sprintf("  [FLAGGED: %s]", strings.Join(rec.Provenance.SecurityFlags, ", "))
		}
		if *dryRun {
			_, _ = fmt.Fprintf(out, "  would create %s %s%s\n", rec.ID, rec.Path, flag)
		} else {
			if err := writeRecord(c.corpus, rec); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "  created %s %s%s\n", rec.ID, rec.Path, flag)
		}
		created++
		id = bumpID(id)
		if *limit > 0 && created >= *limit {
			break // bound a scheduled import so it grows the corpus gradually (0022)
		}
	}

	verb := "created"
	if *dryRun {
		verb = "would create"
	}
	_, _ = fmt.Fprintf(out, "ingest %s: %s %d records, skipped %d (known), flagged %d (quarantined+documented)\n",
		src.Name(), verb, created, skipped, flagged)
	return nil
}

// batchKey is a draft's primary dedup signal for within-batch deduplication:
// its first error signature, else its title.
func batchKey(d ingest.Draft) string {
	if d.Symptom != nil {
		for _, sig := range d.Symptom.ErrorSignatures {
			if s := strings.TrimSpace(sig); s != "" {
				return s
			}
		}
	}
	return d.Title
}

// bumpID returns the next sequential exp-NNNN id. The index is not rebuilt
// mid-batch, so ids are advanced locally as records are created.
func bumpID(id string) string {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "exp-"))
	return fmt.Sprintf("exp-%04d", n+1)
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

// pipelineRunner is the seam the draft command drives: drafter.Pipeline.Run
// satisfies it. Abstracting it lets the corpus walk + selection + persistence be
// unit-tested without Docker/runsc (the broker is the part that needs them).
type pipelineRunner interface {
	Run(ctx context.Context, rec *record.Record) (drafter.Outcome, error)
}

// draftStats summarizes a draft run.
type draftStats struct {
	attached    int // a drafted repro held under the gate and was attached
	rejected    int // a drafted repro did not hold (auto-rejected, files removed)
	unsupported int // no template covered the record (left for the model drafter)
	skipped     int // a quarantined record already carried a positive proof (idempotent re-run)
}

// runDraft runs the deterministic drafter pipeline (ADR-0011 §8) over the corpus:
// for each quarantined record without a repro, it drafts a candidate repro, gates
// it in the gVisor broker (fail-pre / pass-post, offline), and attaches the proof
// into the record's guard — still quarantined; promotion stays the human PR step
// (#0020). The execute phase runs untrusted code, so this needs docker + the runsc
// runtime (the brain); a bare checkout can use -dry-run to list candidates.
func runDraft(ctx context.Context, args []string, out io.Writer) error {
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
			if isCandidate(rec) {
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
	p := drafter.NewPipeline(drafter.NewGoDeprecationDrafter(), rv, *corpus)

	st, err := draftCorpus(ctx, *corpus, recs, p, writeRecord, out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "draft: attached %d, rejected %d, unsupported %d, skipped %d (already proven)\n",
		st.attached, st.rejected, st.unsupported, st.skipped)
	return nil
}

// isCandidate reports whether the drafter should attempt rec: a quarantined
// record that does not already carry a positive (fail-to-pass) proof. The same
// predicate drives the -dry-run listing and the real walk, so the preview can
// never diverge from what the gate actually touches. A record with only a
// negative (dead-end) repro is still a candidate — it lacks a positive proof.
func isCandidate(rec *record.Record) bool {
	return rec.Status == "quarantined" && !record.HasPositiveRepro(rec)
}

// draftCorpus is the testable core of `twiceshy draft`: it walks the candidate
// records, runs each through the pipeline, and persists the record whose drafted
// repro held (the pipeline already wrote/removed the repro files and mutated the
// guard in place). run and persist are injected so the walk is exercised without
// a sandbox. A gate error aborts; records attached before it stay written (each
// is an independently-valid proven repro, and a re-run resumes — already-proven
// records are skipped).
func draftCorpus(ctx context.Context, corpus string, recs []*record.Record, run pipelineRunner, persist func(string, *record.Record) error, out io.Writer) (draftStats, error) {
	var st draftStats
	for _, rec := range recs {
		if !isCandidate(rec) {
			if rec.Status == "quarantined" {
				st.skipped++ // already carries a positive proof — re-running attaches nothing new
			}
			continue
		}
		outcome, err := run.Run(ctx, rec)
		if err != nil {
			return st, fmt.Errorf("draft %s: %w", rec.ID, err)
		}
		if !outcome.Drafted {
			st.unsupported++
			continue
		}
		if !outcome.Attached {
			st.rejected++
			_, _ = fmt.Fprintf(out, "  rejected %s (%s)\n", rec.ID, outcome.Reason)
			continue
		}
		if err := persist(corpus, rec); err != nil {
			// The drafter wrote the repro dir and the gate proved it, but the record
			// that references it never landed — remove the now-orphan repro so a
			// failed persist leaves no dangling files in the corpus.
			removeRepro(corpus, outcome.ReproPath)
			return st, fmt.Errorf("persist %s: %w", rec.ID, err)
		}
		st.attached++
		_, _ = fmt.Fprintf(out, "  attached %s -> %s\n", rec.ID, outcome.ReproPath)
	}
	return st, nil
}

// removeRepro best-effort deletes a drafted repro directory under the corpus,
// used to roll back a proven-but-unpersisted draft.
func removeRepro(corpus, reproPath string) {
	if reproPath == "" {
		return
	}
	if dst, err := safeJoin(corpus, reproPath); err == nil {
		_ = os.RemoveAll(dst)
	}
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
	if err := os.WriteFile(filepath.Join(*outDir, "ATTRIBUTION.md"), attributionDoc(m), 0o644); err != nil {
		return err
	}

	kind := "open"
	if *commercial {
		kind = "commercial"
	}
	_, _ = fmt.Fprintf(out, "pack (%s): included %d, excluded %d, attribution %d -> %s\n",
		kind, len(m.Included), len(m.Excluded), len(m.Attribution), *outDir)
	return nil
}

// attributionDoc renders the pack's ATTRIBUTION.md from its manifest.
func attributionDoc(m pack.Manifest) []byte {
	var b strings.Builder
	b.WriteString("# Attribution\n\n")
	if len(m.Attribution) == 0 {
		b.WriteString("No records in this pack require attribution.\n")
		return []byte(b.String())
	}
	b.WriteString("This pack includes records distilled from the following attributed sources:\n\n")
	for _, a := range m.Attribution {
		_, _ = fmt.Fprintf(&b, "- `%s` — %s — %s\n", a.ID, a.SourceLicense, a.SourceURL)
	}
	return []byte(b.String())
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

// runEval runs the retrieval-effectiveness eval (#0005) over the corpus: it
// drives the validated-only pull path with queries taken from each behavioral
// record's error signatures + summary, and reports recall@k / MRR / near-miss.
// It is the store's evidence gate — does the corpus surface the right trap?
func runEval(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	c := addCommonFlags(fs)
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func printReport(out io.Writer, rep doctor.Report) {
	_, _ = fmt.Fprintf(out, "doctor %s: %d finding(s)\n", rep.Doctor, len(rep.Findings))
	for _, f := range rep.Findings {
		_, _ = fmt.Fprintf(out, "  %s (%s)\n    issue:    %s\n    proposal: %s\n", f.RecordID, f.Path, f.Issue, f.Proposal)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only

// Command twiceshy is the experience service binary (ADR-0001 §9): one Go
// process serving the Phase 1 read path, plus corpus tooling.
//
//	twiceshy index  -corpus <dir> -db <file>          rebuild the derived index
//	twiceshy serve  -corpus <dir> -db <file> -addr …  rebuild, then serve MCP
//	twiceshy ingest <source> -corpus <dir> -db <file> import quarantined records
//	twiceshy pack   -corpus <dir> -out <dir>          build a distributable pack
//
// serve requires the bearer token in TWICESHY_TOKEN.
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

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
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
		return errors.New("usage: twiceshy <index|serve|ingest|pack> [flags]")
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
	default:
		return fmt.Errorf("unknown subcommand %q (want index, serve, ingest, or pack)", args[0])
	}
}

type commonFlags struct {
	corpus string
	db     string
	repo   string
}

func addCommonFlags(fs *flag.FlagSet) *commonFlags {
	var c commonFlags
	fs.StringVar(&c.corpus, "corpus", ".", "corpus root (the directory containing experience/)")
	fs.StringVar(&c.db, "db", "twiceshy.db", "path of the derived SQLite index")
	fs.StringVar(&c.repo, "repo", "", "corpus repository identifier for app-scoped fingerprints")
	return &c
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

	handler, err := server.New(server.Config{Index: ix, Token: token, Repo: c.repo})
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
	default:
		return nil, fmt.Errorf("unknown ingest source %q (want: go, osv, py)", name)
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
		return errors.New("usage: twiceshy ingest <source> [flags] (sources: go, osv, py)")
	}
	src, err := importSource(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	c := addCommonFlags(fs)
	dryRun := fs.Bool("dry-run", false, "classify and report, but write no files")
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

	var created, skipped int
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
		if *dryRun {
			_, _ = fmt.Fprintf(out, "  would create %s %s\n", rec.ID, rec.Path)
		} else {
			if err := writeRecord(c.corpus, rec); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "  created %s %s\n", rec.ID, rec.Path)
		}
		created++
		id = bumpID(id)
	}

	verb := "created"
	if *dryRun {
		verb = "would create"
	}
	_, _ = fmt.Fprintf(out, "ingest %s: %s %d records, skipped %d (known)\n", src.Name(), verb, created, skipped)
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

// writeRecord marshals a record and writes it under the corpus at its path,
// creating the year directory. Persistence is a CLI concern (ADR-0008).
func writeRecord(corpus string, rec *record.Record) error {
	md, err := record.Marshal(rec)
	if err != nil {
		return err
	}
	dst := filepath.Join(corpus, filepath.FromSlash(rec.Path))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, md, 0o644)
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
		dst := filepath.Join(*outDir, filepath.FromSlash(r.Path))
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

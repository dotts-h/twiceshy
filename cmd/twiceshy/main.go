// Command twiceshy is the experience service binary (ADR-0001 §9): one Go
// process serving the Phase 1 read path.
//
//	twiceshy index -corpus <dir> -db <file>          rebuild the derived index
//	twiceshy serve -corpus <dir> -db <file> -addr …  rebuild, then serve MCP
//
// serve requires the bearer token in TWICESHY_TOKEN.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "twiceshy:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: twiceshy <index|serve> [flags]")
	}
	switch args[0] {
	case "index":
		return runIndex(ctx, args[1:], out)
	case "serve":
		return runServe(ctx, args[1:], out, getenv)
	default:
		return fmt.Errorf("unknown subcommand %q (want index or serve)", args[0])
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
	fs.SetOutput(out)
	c := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
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
	fs.SetOutput(out)
	c := addCommonFlags(fs)
	addr := fs.String("addr", ":8722", "listen address")
	if err := fs.Parse(args); err != nil {
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

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
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

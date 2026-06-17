// Package index is the derived, always-rebuildable SQLite index over the
// experience corpus (ADR-0001 §1) and the embedding-free retrieval path
// (§3–4): fingerprint-exact first, BM25/FTS5 lexical second, hard cap k≤3,
// relevance floor. Dense retrieval is a later phase and lives elsewhere by
// design — never here, this is the hot path.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	_ "modernc.org/sqlite" // database/sql driver

	"github.com/dotts-h/twiceshy/internal/fingerprint"
	"github.com/dotts-h/twiceshy/internal/record"
)

// MaxK is the hard cap on returned hits (ADR-0001 §3). It is an invariant,
// not a tunable.
const MaxK = 3

// DefaultFloor is the relevance floor applied to every novelty classification
// by default (ADR-0004). Like MaxK it is a locked invariant made concrete, not
// a tunable. The literal is a coarse, corpus-sensitive threshold whose only job
// this phase is to demote a single weak token while a genuine multi-term match
// survives; it is owned by a guarding test and revisited when dense/RRF banding
// lands (ADR-0006). It is not a runtime tunable or server flag.
const DefaultFloor = 2.0e-06

// FloorOff is the explicit floor-off opt-out: a caller wanting raw recall
// (tests, diagnostic/pull callers) sets Query.Floor = FloorOff so the intent
// reads as a choice, never an accidental zero. Any Floor <= 0 disables the
// floor at the Search mechanism; FloorOff names that.
const FloorOff = -1.0

// How a hit was matched, in precedence order.
const (
	MatchedFingerprint = "fingerprint"
	MatchedLexical     = "lexical"
)

// fingerprintScore is the exported score for fingerprint-exact hits: a
// deterministic match outranks any lexical score.
const fingerprintScore = 1000.0

// maxQueryTokens bounds OR-query cost on long pasted error texts.
const maxQueryTokens = 24

// ErrNotFound is returned by Get for an unknown record id.
var ErrNotFound = errors.New("record not found")

// Index wraps the single SQLite file. It is derived state: delete the file
// and Rebuild to recover, never migrate.
type Index struct {
	db *sql.DB
}

// Query is one retrieval request.
type Query struct {
	// Text is the symptom, error message, or topic searched for.
	Text string
	// Repo, when set, additionally matches app-scoped fingerprints for
	// that repository identifier.
	Repo string
	// Ecosystem/Package filter hits by stack fingerprint (applies_to).
	Ecosystem string
	Package   string
	// K is the number of hits wanted; clamped to MaxK.
	K int
	// Floor is the relevance floor for lexical hits, as a positive score:
	// matches scoring below it are dropped entirely — returning nothing is
	// a feature (near-miss defense). <=0 means no floor (pull channel).
	Floor float64
	// IncludeQuarantined surfaces quarantined records, labeled by Status.
	// Push-channel callers must never set this (ADR-0001 §6).
	IncludeQuarantined bool
}

// Hit is one search result. Score is positive, higher-is-better: the
// SQLite bm25() smaller-is-better convention is flipped exactly once, here
// (see experience/2026/0002-fts5-bm25-negative-scores.md).
type Hit struct {
	ID      string
	Kind    string
	Status  string
	Title   string
	Summary string
	Path    string
	Score   float64
	Matched string
}

// Stored is a full record as served by get_experience.
type Stored struct {
	ID       string
	Kind     string
	Status   string
	Title    string
	Summary  string
	Path     string
	Markdown string
}

const ddl = `
CREATE TABLE IF NOT EXISTS records (
  id      TEXT PRIMARY KEY,
  kind    TEXT NOT NULL,
  status  TEXT NOT NULL,
  title   TEXT NOT NULL,
  summary TEXT NOT NULL,
  path    TEXT NOT NULL,
  raw     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS fingerprints (
  fp        TEXT NOT NULL,
  scope     TEXT NOT NULL,
  record_id TEXT NOT NULL,
  PRIMARY KEY (fp, record_id)
);
CREATE TABLE IF NOT EXISTS applies_to (
  record_id TEXT NOT NULL,
  ecosystem TEXT NOT NULL DEFAULT '',
  package   TEXT NOT NULL DEFAULT ''
);
CREATE VIRTUAL TABLE IF NOT EXISTS records_fts USING fts5(
  id UNINDEXED, title, summary, signatures, body
);
`

// Open opens (creating if needed) the index file.
func Open(path string) (*Index, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening index %s: %w", path, err)
	}
	if _, err := db.Exec(ddl); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating index schema: %w", err)
	}
	return &Index{db: db}, nil
}

// Close releases the underlying database.
func (ix *Index) Close() error { return ix.db.Close() }

// Rebuild replaces the whole index from the given records. repo is the
// corpus's repository identifier, used to derive app-scoped fingerprints.
// The index is derived state, so a full wipe-and-reload is the correct
// "migration" for every change.
func (ix *Index) Rebuild(ctx context.Context, recs []*record.Record, repo string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range []string{
		"DELETE FROM records", "DELETE FROM fingerprints",
		"DELETE FROM applies_to", "DELETE FROM records_fts",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("rebuild: %w", err)
		}
	}
	for _, r := range recs {
		if err := insertRecord(ctx, tx, r, repo); err != nil {
			return fmt.Errorf("rebuild %s: %w", r.ID, err)
		}
	}
	return tx.Commit()
}

func insertRecord(ctx context.Context, tx *sql.Tx, r *record.Record, repo string) error {
	summary, sigs := "", []string(nil)
	if r.Symptom != nil {
		summary = r.Symptom.Summary
		sigs = r.Symptom.ErrorSignatures
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO records (id, kind, status, title, summary, path, raw) VALUES (?,?,?,?,?,?,?)",
		r.ID, r.Kind, r.Status, r.Title, summary, r.Path, string(r.Raw)); err != nil {
		return err
	}

	fps := map[string]string{} // fp -> scope; deduplicates within a record
	for _, sig := range sigs {
		fps[fingerprint.Generic(sig)] = "generic"
		fps[fingerprint.App(repo, sig)] = "app"
	}
	if r.Symptom != nil && r.Symptom.Fingerprints != nil {
		for _, fp := range r.Symptom.Fingerprints.App {
			fps[fp] = "app"
		}
		for _, fp := range r.Symptom.Fingerprints.Generic {
			fps[fp] = "generic"
		}
	}
	for fp, scope := range fps {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO fingerprints (fp, scope, record_id) VALUES (?,?,?)",
			fp, scope, r.ID); err != nil {
			return err
		}
	}

	for _, a := range r.AppliesTo {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO applies_to (record_id, ecosystem, package) VALUES (?,?,?)",
			r.ID, a.Ecosystem, a.Package); err != nil {
			return err
		}
	}

	_, err := tx.ExecContext(ctx,
		"INSERT INTO records_fts (id, title, summary, signatures, body) VALUES (?,?,?,?,?)",
		r.ID, r.Title, summary, strings.Join(sigs, "\n"), r.Body)
	return err
}

// Search runs the embedding-free retrieval pipeline: fingerprint-exact
// hits first, then lexical BM25 hits, deduplicated, capped at MaxK.
func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error) {
	k := q.K
	if k <= 0 || k > MaxK {
		k = MaxK
	}

	hits, err := ix.fingerprintHits(ctx, q, k)
	if err != nil {
		return nil, err
	}
	if len(hits) < k {
		seen := make(map[string]bool, len(hits))
		for _, h := range hits {
			seen[h.ID] = true
		}
		lex, err := ix.lexicalHits(ctx, q, k)
		if err != nil {
			return nil, err
		}
		for _, h := range lex {
			if !seen[h.ID] && len(hits) < k {
				hits = append(hits, h)
			}
		}
	}
	return hits, nil
}

func (ix *Index) fingerprintHits(ctx context.Context, q Query, k int) ([]Hit, error) {
	if strings.TrimSpace(q.Text) == "" {
		return nil, nil
	}
	fps := []any{fingerprint.Generic(q.Text)}
	if q.Repo != "" {
		fps = append(fps, fingerprint.App(q.Repo, q.Text))
	}

	var sb strings.Builder
	args := fps
	sb.WriteString(`SELECT DISTINCT r.id, r.kind, r.status, r.title, r.summary, r.path
		FROM fingerprints f JOIN records r ON r.id = f.record_id
		WHERE f.fp IN (?` + strings.Repeat(",?", len(fps)-1) + `)`)
	args = appendStatusFilter(&sb, args, q)
	args = appendStackFilter(&sb, args, q)
	sb.WriteString(" ORDER BY r.id LIMIT ?")
	args = append(args, k)

	rows, err := ix.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("fingerprint search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		h := Hit{Score: fingerprintScore, Matched: MatchedFingerprint}
		if err := rows.Scan(&h.ID, &h.Kind, &h.Status, &h.Title, &h.Summary, &h.Path); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (ix *Index) lexicalHits(ctx context.Context, q Query, k int) ([]Hit, error) {
	match := ftsQuery(q.Text)
	if match == "" {
		return nil, nil
	}

	var sb strings.Builder
	// bm25() is negative, smaller-is-better (exp-0002): order ascending,
	// floor as bm25 <= -Floor, and flip the sign exactly once on the way
	// out so callers only ever see positive higher-is-better scores.
	sb.WriteString(`SELECT r.id, r.kind, r.status, r.title, r.summary, r.path, m.s
		FROM (SELECT id, bm25(records_fts) AS s FROM records_fts WHERE records_fts MATCH ?) m
		JOIN records r ON r.id = m.id
		WHERE 1=1`)
	args := []any{match}
	if q.Floor > 0 {
		sb.WriteString(" AND m.s <= ?")
		args = append(args, -q.Floor)
	}
	args = appendStatusFilter(&sb, args, q)
	args = appendStackFilter(&sb, args, q)
	sb.WriteString(" ORDER BY m.s ASC, r.id LIMIT ?")
	args = append(args, k)

	rows, err := ix.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("lexical search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		var h Hit
		var s float64
		if err := rows.Scan(&h.ID, &h.Kind, &h.Status, &h.Title, &h.Summary, &h.Path, &s); err != nil {
			return nil, err
		}
		h.Score = -s
		h.Matched = MatchedLexical
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func appendStatusFilter(sb *strings.Builder, args []any, q Query) []any {
	if q.IncludeQuarantined {
		sb.WriteString(" AND r.status IN ('validated','quarantined')")
	} else {
		sb.WriteString(" AND r.status = 'validated'")
	}
	return args
}

func appendStackFilter(sb *strings.Builder, args []any, q Query) []any {
	if q.Ecosystem == "" && q.Package == "" {
		return args
	}
	sb.WriteString(` AND EXISTS (
		SELECT 1 FROM applies_to a WHERE a.record_id = r.id`)
	if q.Ecosystem != "" {
		sb.WriteString(" AND lower(a.ecosystem) = lower(?)")
		args = append(args, q.Ecosystem)
	}
	if q.Package != "" {
		sb.WriteString(" AND lower(a.package) = lower(?)")
		args = append(args, q.Package)
	}
	sb.WriteString(")")
	return args
}

// NextID returns the next sequential record id ("exp-NNNN"): one past the
// highest currently indexed. The write path uses it to allocate an id for a
// new quarantined record. An empty corpus yields "exp-0001".
func (ix *Index) NextID(ctx context.Context) (string, error) {
	var max sql.NullInt64
	err := ix.db.QueryRowContext(ctx,
		`SELECT MAX(CAST(substr(id, 5) AS INTEGER)) FROM records WHERE id LIKE 'exp-%'`).Scan(&max)
	if err != nil {
		return "", fmt.Errorf("next id: %w", err)
	}
	return fmt.Sprintf("exp-%04d", max.Int64+1), nil
}

// Get returns one record by id, any status — pull-channel callers asked
// for it explicitly.
func (ix *Index) Get(ctx context.Context, id string) (*Stored, error) {
	var s Stored
	err := ix.db.QueryRowContext(ctx,
		"SELECT id, kind, status, title, summary, path, raw FROM records WHERE id = ?", id).
		Scan(&s.ID, &s.Kind, &s.Status, &s.Title, &s.Summary, &s.Path, &s.Markdown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", id, err)
	}
	return &s, nil
}

// ftsQuery turns untrusted free text into a safe FTS5 query: each token is
// double-quoted (with embedded quotes doubled) so nothing the user typed
// can be parsed as FTS5 syntax, joined with OR for recall on long error
// texts. See experience/2026/0001-fts5-match-raw-user-input.md.
func ftsQuery(text string) string {
	var quoted []string
	for _, tok := range strings.Fields(text) {
		if !hasAlnum(tok) {
			continue // a token with no letters or digits can't match anything
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
		if len(quoted) == maxQueryTokens {
			break
		}
	}
	return strings.Join(quoted, " OR ")
}

func hasAlnum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

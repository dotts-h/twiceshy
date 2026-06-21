// SPDX-License-Identifier: AGPL-3.0-only

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

// Push-channel discriminative gate (ADR-0001 §4: embedding-free; BM25 + fingerprint
// only). A magnitude floor cannot separate off-topic from on-topic on the live
// corpus — BM25 is corpus-relative, so off-topic prose ("buy milk") scores as high
// as a weak genuine hit. Two signals do separate, and BOTH are required:
//
//   - Document frequency: a discriminative token sits in 1..pushMaxDF validated
//     records. Absent tokens (df=0) and corpus-generic tokens (df>pushMaxDF) are out.
//   - Common-word exclusion (pushStopwords): low df in a SMALL curated corpus is not
//     specificity — common dev vocabulary ("http", "request", "version", "cache") is
//     rare here only because the corpus is tiny, and leaked cards into off-domain
//     sessions (the #0005 push-precision eval reproduces it). So a token must also
//     not be a common word. The genuine signals are the rare identifiers a real
//     error query carries (fts5, bm25, servemux, tmpdir, rand.Seed, setup-go).
//
// pushMaxDF is a FIXED ceiling, not a fraction of the corpus: an earlier
// ceil(0.25·nValidated) rule loosened the gate as the corpus grew (3→19 once the
// OSV/GHSA advisories were promoted), which is exactly the leak the eval caught.
// Push-only — leaves DefaultFloor / pull / Assess untouched. Guarded by
// eval.TestPushPrecisionOnLiveCorpus.
const (
	// pushFloor is the positive BM25 floor for push, applied to the
	// discriminative-token subset so an off-topic prose token can't contribute a card.
	pushFloor = 3.0
	// pushMaxDF is the df ceiling: a discriminative token is in 1..pushMaxDF validated
	// records. Fixed (not corpus-scaled) so corpus growth can never loosen the gate.
	pushMaxDF = 3
)

// pushStopwords are common English words and common dev/web/ops/data vocabulary
// that must never count as a discriminative token. The principle (validated by the
// #0005 push-precision eval): a genuine error query is carried by RARE identifiers
// (fts5, bm25, servemux, tmpdir, rand.Seed, setup-go), never by common words —
// which are rare in THIS tiny corpus only by accident of its size, and which leaked
// unrelated cards into off-domain sessions (a Svelte+FastAPI prompt surfacing
// Go/SQLite cards). Stoplisting a common word never silences a genuine query,
// because that query still carries its specific tokens. So this is deliberately
// broad — it is the "common vocabulary" half of the gate, where df is the "rare in
// the corpus" half, and BOTH are required. It must cover common words generally,
// not just ones observed leaking, because the corpus grows: a word at df 0 today
// can reach the [1,pushMaxDF] band tomorrow. Guarded two ways: the behavioral
// eval.TestPushPrecisionOnLiveCorpus (realistic off-domain prompts inject nothing)
// and the mechanical TestPushGateExcludesCommonVocabulary (no common word is ever
// discriminative). Grow it — with both guards — as new common words surface.
var pushStopwords = wordSet(commonWords)

// wordSet splits whitespace-separated words into a lookup set.
func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// commonWords is the stoplist source: English function words, high-frequency
// English content words, and common software/web/ops/data vocabulary (plus the
// inflections a prompt actually uses). None is a discriminative signal; every
// genuine trap query carries a rarer identifier alongside these.
const commonWords = `
a an and any are as at be been being but by can cant could did do does doing done
dont for from had has have how i if in into is it its just like may me might my no
nor not of off on once only or our out over please should so some such than that the
their them then there these they this those to too up us very was we were what when
where which while who why will with would you your yours

about above after again all also always another around because before below best better
between both come comes down during each either else enough even ever every few first
get gets getting give given go going gone good got great here high its keep know last
least left less let lets long made make makes making many more most much must need
needs never new next now old once one only open other our own part put puts ready real
right run runs same see seen set sets show shows side since small start starts still
stop stops sure take takes tell than that thing things think three time times today try
trying turn two use used uses using want wants way ways well went what why work works
working wrong year years bad buy gift mother birthday milk

add added adds api app apps async await backend base bash branch buffer build builds
building cache cached caches call called calls case cases cd change changed changes
check checked checks class classes cli client clients close cloud cluster code column
command commands commit component components compose config configure connection console
container containers content context controller cookie copy create created css data
database day default delete dependency deploy deployed deployment deployments dev develop
development directory disk docker document does download edit element email empty endpoint
endpoints env environment error errors event events example exception export feature
fetch field fields file files filter fix fixed flag folder form format frontend function
functions get getter git global handle handled handler handlers hash header headers hook
hooks host html http https icon id image images implement import index input install
instance integration interface issue item items job json key keys kubernetes label
layer layers level library line lines link links lint list lists load loaded loader local
log logged logging login logs loop main map margin merge message method methods middleware
migration mobile mock mode model models module modules name names namespace network node
nodes null number object objects open option options order output package packages page
pages parse parser password patch path pause permission permissions pipeline pod pods
port post print private process production prop props provider providers proxy public
query queries queue react read reads recipe recipes record refactor reference register
release releases reload render rendered replica replicas repo repository request requests
reset resolve resource resources response responses rest result results return returns
role rollout route router routes row rows run running runtime save scale schema scope
score screen script scroll search secret select send series server servers service
services session set settings setup shell side signal size slow socket source span split
sql ssl stack staging state status stderr stdin stdout step steps storage store stored
stores stream string style submit subscribe svelte sync system table tag tags target task
tasks template test tests text theme thread threads timeout title token tokens transcript
type types ui update updated updates upgrade upload url use user users util value values
variable version versions view views volume web webhook widget worker workflow wrapper
write writable writes yaml

aws gcp azure gke eks terraform ansible helm ingress vpc cdn dns ssh tls nginx redis
kafka grpc graphql websocket ci cd binding bindings fast fastapi vue angular numpy pandas
guard guards helper helpers join joins joined mount mounts mounted pull pulls pulled push
pushes pushed fail fails failed failing date dates software shipping implements unit units
`

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
CREATE TABLE IF NOT EXISTS embeddings (
  record_id TEXT PRIMARY KEY,
  vec       BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS embedding_cache (
  hash TEXT PRIMARY KEY,
  vec  BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS usage (
  record_id         TEXT PRIMARY KEY,
  retrieved         INTEGER NOT NULL DEFAULT 0,
  pushed            INTEGER NOT NULL DEFAULT 0,
  confirmed_helpful INTEGER NOT NULL DEFAULT 0,
  last_hit          TEXT
);
`

// migrations are additive, idempotent in-place schema changes for index files
// that predate a column. The index is derived state (delete + Rebuild recovers
// it), but the usage table accumulates counters that survive Rebuild and only
// flush to provenance periodically — so a live db must gain a new usage column
// in place rather than be dropped. ADD COLUMN is the one safe SQLite migration;
// the "duplicate column" error on an already-migrated db is expected and ignored.
var migrations = []string{
	`ALTER TABLE usage ADD COLUMN pushed INTEGER NOT NULL DEFAULT 0`,
}

// maxOpenConns bounds the SQLite connection pool to the documented
// ≤4-concurrent budget. WAL makes concurrent reads safe; the cap stops an
// unbounded pool from opening a file handle per in-flight request.
const maxOpenConns = 4

// Open opens (creating if needed) the index file.
func Open(path string) (*Index, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening index %s: %w", path, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	if _, err := db.Exec(ddl); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating index schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			_ = db.Close()
			return nil, fmt.Errorf("migrating index schema: %w", err)
		}
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

// floorPolicy applies the default relevance floor when the caller left Floor
// unset (ADR-0004, ADR-0007). An explicit Floor — a positive threshold or the
// FloorOff sentinel — is the caller's deliberate override and passes through.
// This is the single home of "the floor is on by default", shared by every
// injection path (Retrieve, and Assess through it).
func floorPolicy(q Query) Query {
	if q.Floor == 0 {
		q.Floor = DefaultFloor
	}
	return q
}

// Retrieve is the injection-path search: the pull channel (search_experience)
// and, later, the push channel reach the corpus through it, so the relevance
// floor (ADR-0001 §3) is applied as policy and can never be a per-caller
// accident (ADR-0007). For raw, floor-free recall (tests, diagnostics) call
// Search with Floor: FloorOff.
func (ix *Index) Retrieve(ctx context.Context, q Query) ([]Hit, error) {
	return ix.Search(ctx, floorPolicy(q))
}

// RetrievePush is the push-channel retrieval (POST /push). Unlike Retrieve it
// injects NOTHING unless the query carries a discriminative token — a content
// token in 1..maxDF validated records, with stopwords and ecosystem names
// excluded — and then searches only that subset at pushFloor, so off-topic prose
// can never contribute a card. A fingerprint-exact match (a deterministic stack
// signature) always bypasses the gate: it is real context by construction.
// Embedding-free; quarantined records are never surfaced (ADR-0001 §4, §6).
func (ix *Index) RetrievePush(ctx context.Context, q Query) ([]Hit, error) {
	d, err := ix.RetrievePushTraced(ctx, q)
	return d.Served, err
}

// PushDecision is the gate decision RetrievePushTraced makes, for per-query
// telemetry (#0067): which path the gate took and what it served. It is a
// read-only trace — recording it can never influence ranking (ADR-0013 §4).
type PushDecision struct {
	FingerprintBypass bool     // a deterministic stack match bypassed the discriminative gate
	Discriminative    []string // the gate-passing tokens; empty means the gate stayed closed
	Served            []Hit
}

// RetrievePushTraced is RetrievePush with the gate decision exposed. The logic is
// identical to RetrievePush — it just returns what the gate did alongside the
// served hits, computed on the same single pass (no extra work on the hot path).
func (ix *Index) RetrievePushTraced(ctx context.Context, q Query) (PushDecision, error) {
	q.IncludeQuarantined = false // push never surfaces quarantined records

	// 1) fingerprint-exact bypass — a deterministic match is always real context.
	if fp, err := ix.fingerprintHits(ctx, q, MaxK); err != nil {
		return PushDecision{}, err
	} else if len(fp) > 0 {
		q.Floor = pushFloor
		served, err := ix.Retrieve(ctx, q)
		return PushDecision{FingerprintBypass: true, Served: served}, err
	}

	// 2) discriminative-token precondition.
	disc, err := ix.discriminativeTokens(ctx, q.Text)
	if err != nil {
		return PushDecision{}, err
	}
	if len(disc) == 0 {
		return PushDecision{}, nil // generic / off-topic -> inject nothing ("empty is an answer")
	}

	// 3) retrieve + floor on the discriminative subset only.
	pq := q
	pq.Text = strings.Join(disc, " ")
	pq.Floor = pushFloor
	served, err := ix.Retrieve(ctx, pq)
	return PushDecision{Discriminative: disc, Served: served}, err
}

// discriminativeTokens returns the query's content tokens (lowercased, alnum,
// not a stopword, not a corpus ecosystem name) whose validated document frequency
// is in [1, pushMaxDF]. It reuses the ftsQuery tokenization and quoting so df
// counts agree with what the lexical search later matches (exp-0001).
func (ix *Index) discriminativeTokens(ctx context.Context, text string) ([]string, error) {
	eco, err := ix.ecosystemNames(ctx)
	if err != nil {
		return nil, err
	}

	var out []string
	seen := map[string]bool{}
	for _, field := range strings.Fields(strings.ToLower(text)) {
		tok := stripControl(field)
		if tok == "" || !hasAlnum(tok) || pushStopwords[tok] || eco[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		df, err := ix.validatedDF(ctx, tok)
		if err != nil {
			return nil, err
		}
		if df >= 1 && df <= pushMaxDF {
			out = append(out, tok)
		}
		if len(out) >= maxQueryTokens {
			break
		}
	}
	return out, nil
}

// validatedDF counts how many VALIDATED records contain the token in any indexed
// field. Quarantine scope matters: counting the OSV stubs would dilute df and mask
// the discriminative gap, so the count is validated-only.
func (ix *Index) validatedDF(ctx context.Context, tok string) (int, error) {
	var n int
	err := ix.db.QueryRowContext(ctx,
		`SELECT count(*) FROM records_fts m JOIN records r ON r.id = m.id
		 WHERE records_fts MATCH ? AND r.status = 'validated'`, ftsPhrase(tok)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("validated df: %w", err)
	}
	return n, nil
}

// ecosystemNames is the lowercased set of ecosystem labels on validated records.
// A query token equal to one (e.g. "docker") is structural noise, never a
// discriminative signal, so it is stoplisted. Corpus-derived, self-maintaining.
func (ix *Index) ecosystemNames(ctx context.Context) (map[string]bool, error) {
	rows, err := ix.db.QueryContext(ctx,
		`SELECT DISTINCT lower(a.ecosystem) FROM applies_to a
		 JOIN records r ON r.id = a.record_id
		 WHERE r.status = 'validated' AND a.ecosystem != ''`)
	if err != nil {
		return nil, fmt.Errorf("ecosystem names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out[e] = true
	}
	return out, rows.Err()
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
//
// This is a MAX+1 read over the index, NOT an atomic allocation, and it trusts
// the index to be current. Write paths must use ingest.NextID instead, which
// also consults the source-of-truth corpus tree on disk so a live index that
// has drifted behind the committed records cannot hand back an id that already
// exists (#0059). Even ingest.NextID is not atomic: two concurrent
// record_experience calls can be handed the same id. That is acceptable only
// because the write path is propose-only — it returns a draft for a human to
// open as a PR (the trust boundary), where a colliding id is caught in review
// and cheap to re-derive at merge. Reserve/allocate transactionally before
// allowing any unattended (auto-merge / non-PR) write path. See docs/TECH_DEBT.md (M3).
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
		tok = stripControl(tok)
		if !hasAlnum(tok) {
			continue // a token with no letters or digits can't match anything
		}
		quoted = append(quoted, ftsPhrase(tok))
		if len(quoted) == maxQueryTokens {
			break
		}
	}
	return strings.Join(quoted, " OR ")
}

// ftsPhrase quotes one token as an FTS5 phrase literal — embedded quotes doubled,
// wrapped in double quotes — so nothing in the token is parsed as FTS5 syntax
// (exp-0001). Shared by ftsQuery (the OR match) and validatedDF (the push df
// count) so the two tokenize a token identically.
func ftsPhrase(tok string) string {
	return `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
}

// stripControl drops control runes (e.g. a NUL byte) from a token. They match
// nothing in the FTS5 index, and a NUL would terminate the query string mid-token
// — FTS5 then reports "unterminated string" on the dangling open quote (exp-0001).
// Whitespace controls are already removed by strings.Fields; this catches the rest.
func stripControl(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func hasAlnum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

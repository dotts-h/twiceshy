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
	"github.com/dotts-h/twiceshy/internal/idf"
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

// pushEligibleKinds are the kinds an agent can act on mid-prompt (#0107): a trap
// to avoid or a fix to apply. convention/dead-end/workflow records are narrative
// or self-audit material, never decision-relevant at push time — they stay
// reachable via pull (Retrieve/Search), never via push.
var pushEligibleKinds = []string{"trap", "fix"}

// importerOrigins are provenance.source.author values (lowercased) naming a bulk
// import pipeline rather than an agent/human (#0107). ~940/990 validated records
// are importer-origin OSV/deprecation advisories — topical-but-generic self-audit
// material that is never mid-prompt material, so a token living only there must
// never open the push gate.
var importerOrigins = []string{"twiceshy-importer"}

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

// idfMaxDocRatio is the phase-1 ceiling (ADR-0017) above which a token's
// global document-frequency ratio (df/totalDocs, from the embedded idf.Table)
// marks it as "globally common": too generic to ever be a discriminative
// push-gate signal, regardless of how rare it looks in this tiny corpus. The
// literal is deliberately conservative for this first phase — it is not
// derived from measurement yet — and is expected to be recalibrated once
// telemetry from the live gate is available.
const idfMaxDocRatio = 0.10

// idfTableProvider is the small subset of *idf.Table's methods globallyCommonWord
// needs, seamed out so a test can inject a fake table without touching the real
// embedded idf.Global() data.
type idfTableProvider interface {
	Available() bool
	TotalDocs() uint64
	DF(word string) (uint64, bool)
}

// idfProvider is the injectable idfTableProvider seam, defaulting to the
// process-wide embedded table. Tests swap it to exercise globallyCommonWord
// against controlled df/totalDocs values.
var idfProvider idfTableProvider = idf.Global()

// globallyCommonWord reports whether tok is globally common per idfProvider:
// true only when the provider is available, has documents loaded, contains
// tok, and tok's df/totalDocs ratio strictly exceeds idfMaxDocRatio. Every
// other case — an unavailable provider, an empty/default table, or a token
// absent from it — reports false, so a missing table can never be
// misread as "everything is common".
func globallyCommonWord(tok string) bool {
	if idfProvider == nil || !idfProvider.Available() {
		return false
	}
	total := idfProvider.TotalDocs()
	if total == 0 {
		return false
	}
	df, ok := idfProvider.DF(tok)
	if !ok {
		return false
	}
	return float64(df)/float64(total) > idfMaxDocRatio
}

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
port post print private process production prop props provider providers proxy public python
query queries queue react read reads recipe recipes record refactor reference register
release releases reload render rendered replica replicas repo repository request requests
reset resolve resource resources response responses rest result results return returns
role rollout route router routes row rows run running runtime save scale schema scope
score screen script scroll search secret select send series server servers service
services session set settings setup shell side signal size slow socket source span split
sql ssl stack staging state status stderr stdin stdout step steps storage store stored
stores stream string style submit subscribe svelte sync system table tag tags target task
tasks template test tests text theme thread threads timeout title token tokens transcript transition
type types ui update updated updates upgrade upload url use user users util value values
variable version versions view views volume web webhook widget worker workflow wrapper
write writable writes yaml

aws gcp azure gke eks terraform ansible helm ingress vpc cdn dns ssh tls nginx redis
kafka grpc graphql websocket ci cd binding bindings fast fastapi vue angular numpy pandas
guard guards helper helpers join joins joined mount mounts mounted pull pulls pulled push
pushes pushed fail fails failed failing date dates software shipping implements unit units

animation append approve array artifact assert asset auth benchmark body book boolean border
breakpoint bucket bump bundle button changelog cleanup clone color computed coverage dataframe
debug decode decorator desktop dict dinner directive django dropdown early encode extract fixture
flask flex float font framework grid hour inline insert integer jwt keystroke lambda latency layout
leak leap lifecycle logout memory microservice minute modal mongo month movie music mysql navbar
nesting oauth optimize padding param payload performance pool postgres profile pydantic range
reactive rebase redux ref registry rename responsive review rollback rollup second shadow
sidebar slice sprint sqlite standup tablet throughput ticket timestamp timezone tooltip topic
trace transaction travel tuple unmount utility viewport vite warning watcher weather webpack
weekend yield
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
	// ErrorTrigger is set by the /push edge when the client's trigger field is
	// "error" — a verbatim error/log line the error-pull hook (#0087) already
	// singled out, not a raw prompt. It relaxes the two-token corroboration rule
	// (#0108) back to the single-token gate: the text itself is already
	// high-precision, unlike an arbitrary prompt.
	ErrorTrigger bool
	// PushEligibleOnly restricts lexical/fingerprint matching to the push-eligible
	// subset (#0107): validated records of an agent-actionable kind (trap/fix)
	// whose provenance is not a bulk-import pipeline. Set only by the push
	// channel's discriminative-subset retrieval; pull (Retrieve/RetrieveFused)
	// never sets it.
	PushEligibleOnly bool
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
  raw     TEXT NOT NULL,
  origin  TEXT NOT NULL DEFAULT ''
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
CREATE TABLE IF NOT EXISTS tokens (
  id           TEXT PRIMARY KEY,
  secret_hash  BLOB NOT NULL,
  label        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  revoked_at   TEXT,
  daily_quota  INTEGER NOT NULL DEFAULT 1000,
  rate_per_min INTEGER NOT NULL DEFAULT 60
);
CREATE TABLE IF NOT EXISTS token_usage (
  token_id TEXT NOT NULL,
  day      TEXT NOT NULL,
  calls    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (token_id, day)
);
CREATE TABLE IF NOT EXISTS tenant_usage (
  token_id TEXT NOT NULL,
  day      TEXT NOT NULL,
  tool     TEXT NOT NULL,
  calls    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (token_id, day, tool)
);
CREATE TABLE IF NOT EXISTS contribution_usage (
  token_id TEXT NOT NULL,
  day      TEXT NOT NULL,
  tool     TEXT NOT NULL,
  calls    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (token_id, day, tool)
);
CREATE TABLE IF NOT EXISTS organizations (
  id         TEXT PRIMARY KEY,
  label      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspaces (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL,
  label           TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS token_entitlements (
  token_id        TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL,
  workspace_id    TEXT NOT NULL,
  plan            TEXT NOT NULL
);
`

// migrations are additive, idempotent in-place schema changes for index files
// that predate a column. The index is derived state (delete + Rebuild recovers
// it), but the usage table accumulates counters that survive Rebuild and only
// flush to provenance periodically — so a live db must gain a new usage column
// in place rather than be dropped. ADD COLUMN is the one safe SQLite migration;
// the "duplicate column" error on an already-migrated db is expected and ignored.
//
// `records.origin` (#0107) is different: records IS dropped and reloaded by
// Rebuild, so a fresh Rebuild alone would add the column for free — but Open
// runs before the caller's first Rebuild, and a live deploy may read `records`
// (e.g. via NextID) before that happens, so the column must exist immediately.
var migrations = []string{
	`ALTER TABLE usage ADD COLUMN pushed INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE records ADD COLUMN origin TEXT NOT NULL DEFAULT ''`,
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
	origin := strings.ToLower(r.Provenance.Source.Author)
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO records (id, kind, status, title, summary, path, raw, origin) VALUES (?,?,?,?,?,?,?,?)",
		r.ID, r.Kind, r.Status, r.Title, summary, r.Path, string(r.Raw), origin); err != nil {
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
	decision, err := ix.RetrievePushTraced(ctx, q)
	return decision.Served, err
}

// PushDecision is the gate decision RetrievePushTraced makes, for per-query
// telemetry (#0067): which path the gate took and what it served. It is a
// read-only trace — recording it can never influence ranking (ADR-0013 §4).
type PushDecision struct {
	FingerprintBypass bool     // a deterministic stack match bypassed the discriminative gate
	Discriminative    []string // the gate-passing tokens; empty means the gate stayed closed
	IdfFiltered       int      // eligible tokens dropped by the global-IDF check (globallyCommonWord, ADR-0017)
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
		// Serve ONLY the fingerprint-exact hits. The bypass exists because a deterministic
		// stack signature is real context by construction (ADR-0015); it does NOT license
		// admitting BM25 fill from the full query (Retrieve->Search would append it to reach k).
		// Fingerprint hits carry a fixed score above any floor, so no floor step is needed.
		return PushDecision{FingerprintBypass: true, Served: fp}, nil
	}

	// 2) discriminative-token precondition, computed over the ELIGIBLE subset
	// (#0107): a token whose df is nonzero only among importer-origin advisories
	// or a non-trap/fix kind must never open the gate.
	disc, idfFiltered, err := ix.discriminativeTokens(ctx, q.Text)
	if err != nil {
		return PushDecision{}, err
	}
	if len(disc) == 0 {
		return PushDecision{}, nil // generic / off-topic -> inject nothing ("empty is an answer")
	}

	// 2b) corroboration precondition (#0108): a prompt-triggered query needs TWO
	// independent discriminative tokens. A single rare token is exactly the
	// false-positive class the specimen prompt reproduced ("llm" alone served an
	// unrelated card). An error-triggered query — a verbatim stack/log line the
	// error-pull hook already singled out — keeps the single-token gate: the text
	// itself is already high-precision. The gate decision still records disc for
	// telemetry (#0067) even though nothing is served.
	if !q.ErrorTrigger && len(disc) < minCorroboratingTokens {
		return PushDecision{Discriminative: disc, IdfFiltered: idfFiltered}, nil
	}

	// 3) retrieve + floor on the discriminative subset, restricted to eligible
	// records only (#0107) — an OR-joined disc token can still lexically match an
	// ineligible record; the eligibility predicate must apply here too, not just
	// at the df stage.
	pq := q
	pq.Text = strings.Join(disc, " ")
	pq.Floor = pushFloor
	pq.PushEligibleOnly = true
	served, err := ix.Retrieve(ctx, pq)
	if err != nil {
		return PushDecision{}, err
	}

	// 4) per-record corroboration (#0108), prompt trigger only: drop any served
	// hit that does not lexically carry >=2 DISTINCT discriminative tokens. Two
	// tokens that each live in a DIFFERENT record (the "application"+"llm"
	// specimen) pass the OR-joined search above; corroboration is what actually
	// rejects both — neither record carries both tokens.
	if !q.ErrorTrigger {
		served, err = ix.corroborated(ctx, served, disc)
		if err != nil {
			return PushDecision{}, err
		}
	}

	return PushDecision{Discriminative: disc, IdfFiltered: idfFiltered, Served: served}, nil
}

// discriminativeTokens returns the query's content tokens (lowercased, alnum,
// not a stopword, not a corpus ecosystem name) whose ELIGIBLE document frequency
// (#0107: validated, kind trap/fix, non-importer origin) is in [1, pushMaxDF]. It
// reuses the ftsQuery tokenization and quoting so df counts agree with what the
// lexical search later matches (exp-0001).
func (ix *Index) discriminativeTokens(ctx context.Context, text string) ([]string, int, error) {
	return ix.discriminativeTokensVia(ctx, text, ix.eligibleDF)
}

// minCorroboratingTokens is the ADR-0108 two-token corroboration threshold:
// a prompt-triggered query needs at least this many distinct discriminative
// tokens (RetrievePushTraced step 2b), and a served hit must lexically carry
// at least this many of them (corroborated). Named once so the precondition
// and the per-hit check can never silently drift apart.
const minCorroboratingTokens = 2

// corroborated is the per-hit specificity check for prompt-triggered push
// (#0108): a served hit must lexically MATCH at least minCorroboratingTokens
// of the query's DISTINCT discriminative tokens. Cheap: at most MaxK hits x
// maxQueryTokens tokens, one indexed MATCH+id lookup each (sub-ms).
func (ix *Index) corroborated(ctx context.Context, hits []Hit, disc []string) ([]Hit, error) {
	var out []Hit
	for _, h := range hits {
		n, err := ix.tokenMatchCount(ctx, h.ID, disc)
		if err != nil {
			return nil, err
		}
		if n >= minCorroboratingTokens {
			out = append(out, h)
		}
	}
	return out, nil
}

// tokenMatchCount counts how many of tokens lexically MATCH record id, via the
// same FTS5 phrase quoting the df counts use (ftsPhrase, exp-0001).
func (ix *Index) tokenMatchCount(ctx context.Context, id string, tokens []string) (int, error) {
	n := 0
	for _, tok := range tokens {
		var count int
		err := ix.db.QueryRowContext(ctx,
			`SELECT count(*) FROM records_fts WHERE records_fts MATCH ? AND id = ?`,
			ftsPhrase(tok), id).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("corroboration match: %w", err)
		}
		if count > 0 {
			n++
		}
	}
	return n, nil
}

// discriminativeTokensVia is discriminativeTokens with the per-token validated-DF
// lookup injected, so the serial-round-trip bound below can be asserted in a test.
// The second return value counts tokens dropped by the global-IDF check
// (globallyCommonWord) ALONE: a token that passes every local check (df in
// [1, pushMaxDF]) but is globally common per idfProvider is excluded from the
// slice and counted here, separate from tokens that never reach that check.
func (ix *Index) discriminativeTokensVia(ctx context.Context, text string, df func(context.Context, string) (int, error)) ([]string, int, error) {
	eco, err := ix.ecosystemNames(ctx)
	if err != nil {
		return nil, 0, err
	}

	var out []string
	var globallyDropped, scanned int
	seen := map[string]bool{}
	for _, field := range strings.Fields(strings.ToLower(text)) {
		tok := stripControl(field)
		if tok == "" || !hasAlnum(tok) || pushStopwords[tok] || eco[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		n, err := df(ctx, tok)
		if err != nil {
			return nil, 0, err
		}
		if n >= 1 && n <= pushMaxDF {
			if globallyCommonWord(tok) {
				globallyDropped++
			} else {
				out = append(out, tok)
			}
		}
		if len(out) >= maxQueryTokens {
			break
		}
		// Bound the SQLite round-trips themselves: a long off-topic push query is
		// mostly non-discriminative tokens that never grow out, so without this an
		// authenticated client could force one DF query per distinct token.
		scanned++
		if scanned >= maxQueryTokens {
			break
		}
	}
	return out, globallyDropped, nil
}

// validatedDFQuery is the shared base count-query for validatedDF and eligibleDF:
// how many VALIDATED records contain a token in any indexed field. eligibleDF
// appends the push-eligibility predicate to this same base via appendEligibleFilter
// so the two queries can never silently drift apart.
const validatedDFQuery = `SELECT count(*) FROM records_fts m JOIN records r ON r.id = m.id
		 WHERE records_fts MATCH ? AND r.status = 'validated'`

// validatedDF counts how many VALIDATED records contain the token in any indexed
// field. Quarantine scope matters: counting the OSV stubs would dilute df and mask
// the discriminative gap, so the count is validated-only.
func (ix *Index) validatedDF(ctx context.Context, tok string) (int, error) {
	var n int
	err := ix.db.QueryRowContext(ctx, validatedDFQuery, ftsPhrase(tok)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("validated df: %w", err)
	}
	return n, nil
}

// eligibleDF is validatedDF further restricted to the push-eligible subset
// (#0107): kind IN pushEligibleKinds AND origin NOT IN importerOrigins. The
// push gate's discriminative-token precondition is computed over THIS df, not
// validatedDF, so a token that lives only in importer-origin advisories or a
// convention/dead-end/workflow record never opens the gate — the query still
// finds those records via pull (Retrieve), just never via push.
func (ix *Index) eligibleDF(ctx context.Context, tok string) (int, error) {
	var sb strings.Builder
	sb.WriteString(validatedDFQuery)
	args := appendEligibleFilter(&sb, []any{ftsPhrase(tok)})
	var n int
	if err := ix.db.QueryRowContext(ctx, sb.String(), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("eligible df: %w", err)
	}
	return n, nil
}

// appendEligibleFilter adds the push-eligibility predicate (kind + origin) to a
// WHERE clause already scoped to validated records. Shared by eligibleDF and the
// Query.PushEligibleOnly path in fingerprintHits/lexicalHits so both agree on
// exactly what "eligible" means (origin is stored lowercased at index time, see
// insertRecord).
func appendEligibleFilter(sb *strings.Builder, args []any) []any {
	sb.WriteString(" AND r.kind IN (" + placeholders(len(pushEligibleKinds)) + ")")
	for _, k := range pushEligibleKinds {
		args = append(args, k)
	}
	sb.WriteString(" AND r.origin NOT IN (" + placeholders(len(importerOrigins)) + ")")
	for _, o := range importerOrigins {
		args = append(args, o)
	}
	// Alpha-tier tenants (#0128, ADR-0030 phase 2): an "alpha:<token_id>" origin
	// is excluded from push EVEN AFTER validation — defense in depth over the
	// quarantine floor, since a low-trust submission must never reach the
	// mid-prompt injection channel regardless of what promoted it.
	sb.WriteString(" AND r.origin NOT LIKE ?")
	args = append(args, record.AlphaOriginPrefix+"%")
	return args
}

// placeholders renders n "?" SQL parameter placeholders, comma-joined.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
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
		WHERE f.fp IN (` + placeholders(len(fps)) + `)`)
	args = appendSearchFilters(&sb, args, q)
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
	args = appendSearchFilters(&sb, args, q)
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

// appendSearchFilters appends the status, stack (ecosystem/package), and —
// when requested — push-eligibility predicates shared by fingerprintHits and
// lexicalHits, in the fixed order both already agreed on.
func appendSearchFilters(sb *strings.Builder, args []any, q Query) []any {
	appendStatusFilter(sb, q)
	args = appendStackFilter(sb, args, q)
	if q.PushEligibleOnly {
		args = appendEligibleFilter(sb, args)
	}
	return args
}

func appendStatusFilter(sb *strings.Builder, q Query) {
	if q.IncludeQuarantined {
		sb.WriteString(" AND r.status IN ('validated','quarantined')")
	} else {
		sb.WriteString(" AND r.status = 'validated'")
	}
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

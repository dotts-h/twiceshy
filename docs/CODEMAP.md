# CODEMAP — generated, do not edit by hand

> Regenerate with `scripts/codemap.sh`. A per-directory index of source
> files and their top-level declarations, so a session learns the layout
> from this one file instead of opening source to find a symbol. The source
> is the source of truth — if this looks stale, re-run the script.

_Last generated: 2026-06-17 (UTC)._

## cmd/twiceshy

### main.go (141 LOC)
- L28: `func main()`
- L37: `func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L51: `type commonFlags struct`
- L57: `func addCommonFlags(fs *flag.FlagSet) *commonFlags`
- L68: `func buildIndex(ctx context.Context, c *commonFlags) (*index.Index, int, error)`
- L84: `func runIndex(ctx context.Context, args []string, out io.Writer) error`
- L100: `func runServe(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`

## docs/design/examples

### repro-go-ioutil-deprecation.sh (74 LOC)
- _(no top-level declarations matched)_

## experience/repro

### 0001-fts5-raw-match.sh (36 LOC)
- _(no top-level declarations matched)_

## internal/fingerprint

### dedup.go (43 LOC)
- L5: `type DedupResult struct`
- L13: `func Dedup(repo string, sigs []string, known map[string]bool) DedupResult`

### fingerprint.go (58 LOC)
- L33: `func Normalize(s string) string`
- L45: `func Generic(signature string) string`
- L51: `func App(repo, signature string) string`
- L55: `func hash(input string) string`

## internal/index

### assess.go (60 LOC)
- L6: `type Novelty string`
- L18: `type Assessment struct`
- L27: `func (ix *Index) Assess(ctx context.Context, q Query) (Assessment, error)`

### index.go (437 LOC)
- L58: `type Index struct`
- L63: `type Query struct`
- L86: `type Hit struct`
- L98: `type Stored struct`
- L135: `func Open(path string) (*Index, error)`
- L149: `func (ix *Index) Close() error { return ix.db.Close() }`
- L155: `func (ix *Index) Rebuild(ctx context.Context, recs []*record.Record, repo string) error`
- L178: `func insertRecord(ctx context.Context, tx *sql.Tx, r *record.Record, repo string) error`
- L230: `func floorPolicy(q Query) Query`
- L242: `func (ix *Index) Retrieve(ctx context.Context, q Query) ([]Hit, error)`
- L248: `func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error)`
- L276: `func (ix *Index) fingerprintHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L312: `func (ix *Index) lexicalHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L356: `func appendStatusFilter(sb *strings.Builder, args []any, q Query) []any`
- L365: `func appendStackFilter(sb *strings.Builder, args []any, q Query) []any`
- L386: `func (ix *Index) NextID(ctx context.Context) (string, error)`
- L398: `func (ix *Index) Get(ctx context.Context, id string) (*Stored, error)`
- L416: `func ftsQuery(text string) string`
- L430: `func hasAlnum(s string) bool`

## internal/ingest

### prepare.go (168 LOC)
- L14: `type Draft struct`
- L25: `type Meta struct`
- L33: `type Outcome struct`
- L52: `func Prepare(ctx context.Context, ix *index.Index, repo string, d Draft, m Meta) (Outcome, error)`
- L112: `func probes(d Draft) []string`
- L135: `func buildPath(now, id, title string) string`
- L150: `func slugify(title string) string`

## internal/record

### marshal.go (27 LOC)
- L13: `func Marshal(r *Record) ([]byte, error)`

### record.go (429 LOC)
- L33: `type Record struct`
- L53: `type Symptom struct`
- L61: `type Fingerprints struct`
- L66: `type AppliesTo struct`
- L73: `type VersionRange struct`
- L78: `type Resolution struct`
- L84: `type DeadEnd struct`
- L89: `type Guard struct`
- L94: `type Provenance struct`
- L103: `type Source struct`
- L109: `type Validity struct`
- L114: `type Usage struct`
- L130: `func Parse(path string, src []byte) (*Record, error)`
- L153: `func ParseFile(root, rel string) (*Record, error)`
- L164: `func LoadCorpus(root string) ([]*Record, error)`
- L221: `func splitFrontmatter(src []byte) (front []byte, body string, err error)`
- L243: `func Validate(r *Record) error { return r.validate() }`
- L245: `func (r *Record) validate() error`
- L280: `func (r *Record) validatePath(fail func(string, ...any))`
- L294: `func (r *Record) validateSymptom(fail func(string, ...any))`
- L319: `func (r *Record) validateAppliesTo(fail func(string, ...any))`
- L327: `func (r *Record) validateResolution(fail func(string, ...any))`
- L353: `func (r *Record) validateGuard(fail func(string, ...any))`
- L363: `func (r *Record) validateProvenance(fail func(string, ...any))`
- L409: `func checkDate(fail func(string, ...any), field, v string) time.Time`
- L422: `func contains(xs []string, x string) bool`

## internal/server

### record.go (149 LOC)
- L27: `type RecordArgs struct`
- L43: `type RecordResult struct`
- L54: `func (h *handlers) record(ctx context.Context, _ *mcp.CallToolRequest, args RecordArgs) (*mcp.CallToolResult, RecordResult, error)`

### server.go (183 LOC)
- L25: `type Config struct`
- L55: `func New(cfg Config) (http.Handler, error)`
- L78: `type handlers struct`
- L84: `type SearchArgs struct`
- L93: `type SearchHit struct`
- L104: `type SearchResult struct`
- L108: `func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error)`
- L139: `type GetArgs struct`
- L144: `type GetResult struct`
- L153: `func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, GetResult, error)`
- L170: `func bearerAuth(token string, next http.Handler) http.Handler`

## scripts

### apply-branch-protection.sh (106 LOC)
- _(no top-level declarations matched)_

### check-workflows.sh (141 LOC)
- _(no top-level declarations matched)_

### codemap.sh (72 LOC)
- L14: `decl_pattern()`
- L28: `list_files()`

### new-issue.sh (79 LOC)
- _(no top-level declarations matched)_

### next-issue.sh (163 LOC)
- _(no top-level declarations matched)_

### start-fresh.sh (87 LOC)
- L26: `note() { printf '  %s\n' "$*"; }`
- L27: `fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }`

### sync-forgejo.sh (165 LOC)
- _(no top-level declarations matched)_


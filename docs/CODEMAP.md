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

### index.go (389 LOC)
- L44: `type Index struct`
- L49: `type Query struct`
- L72: `type Hit struct`
- L84: `type Stored struct`
- L121: `func Open(path string) (*Index, error)`
- L135: `func (ix *Index) Close() error { return ix.db.Close() }`
- L141: `func (ix *Index) Rebuild(ctx context.Context, recs []*record.Record, repo string) error`
- L164: `func insertRecord(ctx context.Context, tx *sql.Tx, r *record.Record, repo string) error`
- L213: `func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error)`
- L241: `func (ix *Index) fingerprintHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L277: `func (ix *Index) lexicalHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L321: `func appendStatusFilter(sb *strings.Builder, args []any, q Query) []any`
- L330: `func appendStackFilter(sb *strings.Builder, args []any, q Query) []any`
- L350: `func (ix *Index) Get(ctx context.Context, id string) (*Stored, error)`
- L368: `func ftsQuery(text string) string`
- L382: `func hasAlnum(s string) bool`

## internal/record

### record.go (420 LOC)
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
- L239: `func (r *Record) validate() error`
- L271: `func (r *Record) validatePath(fail func(string, ...any))`
- L285: `func (r *Record) validateSymptom(fail func(string, ...any))`
- L310: `func (r *Record) validateAppliesTo(fail func(string, ...any))`
- L318: `func (r *Record) validateResolution(fail func(string, ...any))`
- L344: `func (r *Record) validateGuard(fail func(string, ...any))`
- L354: `func (r *Record) validateProvenance(fail func(string, ...any))`
- L400: `func checkDate(fail func(string, ...any), field, v string) time.Time`
- L413: `func contains(xs []string, x string) bool`

## internal/server

### server.go (181 LOC)
- L24: `type Config struct`
- L54: `func New(cfg Config) (http.Handler, error)`
- L76: `type handlers struct`
- L82: `type SearchArgs struct`
- L91: `type SearchHit struct`
- L102: `type SearchResult struct`
- L106: `func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error)`
- L137: `type GetArgs struct`
- L142: `type GetResult struct`
- L151: `func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, GetResult, error)`
- L168: `func bearerAuth(token string, next http.Handler) http.Handler`


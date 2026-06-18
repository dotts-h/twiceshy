# CODEMAP — generated, do not edit by hand

> Regenerate with `scripts/codemap.sh`. A per-directory index of source
> files and their top-level declarations, so a session learns the layout
> from this one file instead of opening source to find a symbol. The source
> is the source of truth — if this looks stale, re-run the script.

_Last generated: 2026-06-18 (UTC)._

## cmd/twiceshy

### main.go (494 LOC)
- L47: `func main()`
- L63: `func parseFlags(fs *flag.FlagSet, args []string) error`
- L73: `func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L93: `type commonFlags struct`
- L101: `func addCommonFlags(fs *flag.FlagSet) *commonFlags`
- L113: `func embedderFor(c *commonFlags) index.Embedder`
- L123: `func buildIndex(ctx context.Context, c *commonFlags) (*index.Index, int, error)`
- L147: `func runIndex(ctx context.Context, args []string, out io.Writer) error`
- L162: `func runServe(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L211: `func importSource(name string) (ingest.Source, error)`
- L228: `func runIngest(ctx context.Context, args []string, out io.Writer) error`
- L311: `func batchKey(d ingest.Draft) string`
- L324: `func bumpID(id string) string`
- L333: `func safeJoin(base, rel string) (string, error)`
- L345: `func writeRecord(corpus string, rec *record.Record) error`
- L365: `func runPack(args []string, out io.Writer) error`
- L430: `func attributionDoc(m pack.Manifest) []byte`
- L447: `func runDoctor(ctx context.Context, args []string, out io.Writer) error`

## docs/design/examples

### repro-go-ioutil-deprecation.sh (74 LOC)
- _(no top-level declarations matched)_

## experience/repro

### 0001-fts5-raw-match.sh (36 LOC)
- _(no top-level declarations matched)_

## internal/doctor

### doctor.go (52 LOC)
- L20: `type Finding struct`
- L28: `type Report struct`
- L34: `type Doctor interface`
- L42: `type Cycle struct`
- L50: `type EOLSource interface`

### endoflife.go (93 LOC)
- L17: `type endoflifeSource struct`
- L24: `func NewEndOfLifeSource(base string) EOLSource`
- L33: `type eolField struct`
- L38: `func (e *eolField) UnmarshalJSON(b []byte) error`
- L54: `func (e eolField) normalized() string`
- L64: `func (s endoflifeSource) Cycles(ctx context.Context, product string) ([]Cycle, error)`

### staleness.go (131 LOC)
- L27: `type Staleness struct`
- L34: `func NewStaleness(eol EOLSource, now time.Time) *Staleness`
- L46: `func (*Staleness) Name() string { return "staleness" }`
- L52: `func majorMinor(v string) string`
- L63: `func parseDate(s string) (time.Time, bool)`
- L68: `func (s *Staleness) Run(ctx context.Context, recs []*record.Record) (Report, error)`
- L98: `func (s *Staleness) staleByEOL(ctx context.Context, r *record.Record, cache map[string][]Cycle) *Finding`

## internal/fingerprint

### fingerprint.go (60 LOC)
- L35: `func Normalize(s string) string`
- L47: `func Generic(signature string) string`
- L53: `func App(repo, signature string) string`
- L57: `func hash(input string) string`

## internal/index

### assess.go (62 LOC)
- L8: `type Novelty string`
- L20: `type Assessment struct`
- L29: `func (ix *Index) Assess(ctx context.Context, q Query) (Assessment, error)`

### dense.go (364 LOC)
- L34: `type Embedder interface`
- L57: `type OllamaEmbedder struct`
- L65: `func NewOllamaEmbedder(endpoint, model string) *OllamaEmbedder`
- L77: `func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error)`
- L112: `func embedText(r *record.Record) string`
- L142: `func (ix *Index) EmbedCorpus(ctx context.Context, recs []*record.Record, emb Embedder) error`
- L190: `func (ix *Index) RetrieveFused(ctx context.Context, q Query, emb Embedder) ([]Hit, error)`
- L247: `func (ix *Index) denseHits(ctx context.Context, qvec []float32, q Query, k int) ([]Hit, error)`
- L297: `func rrfFuse(lists ...[]Hit) []Hit`
- L327: `func cosine(a, b []float32) float64`
- L345: `func encodeVec(v []float32) []byte`
- L353: `func decodeVec(b []byte) []float32`
- L361: `func hashText(s string) string`

### index.go (477 LOC)
- L60: `type Index struct`
- L65: `type Query struct`
- L88: `type Hit struct`
- L100: `type Stored struct`
- L150: `func Open(path string) (*Index, error)`
- L165: `func (ix *Index) Close() error { return ix.db.Close() }`
- L171: `func (ix *Index) Rebuild(ctx context.Context, recs []*record.Record, repo string) error`
- L194: `func insertRecord(ctx context.Context, tx *sql.Tx, r *record.Record, repo string) error`
- L246: `func floorPolicy(q Query) Query`
- L258: `func (ix *Index) Retrieve(ctx context.Context, q Query) ([]Hit, error)`
- L264: `func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error)`
- L292: `func (ix *Index) fingerprintHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L328: `func (ix *Index) lexicalHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L372: `func appendStatusFilter(sb *strings.Builder, args []any, q Query) []any`
- L381: `func appendStackFilter(sb *strings.Builder, args []any, q Query) []any`
- L409: `func (ix *Index) NextID(ctx context.Context) (string, error)`
- L421: `func (ix *Index) Get(ctx context.Context, id string) (*Stored, error)`
- L439: `func ftsQuery(text string) string`
- L458: `func stripControl(s string) string`
- L470: `func hasAlnum(s string) bool`

## internal/ingest

### deprecation.go (70 LOC)
- L17: `type deprecation struct`
- L31: `type deprecationSource struct`
- L37: `func (s deprecationSource) Name() string { return s.name }`
- L39: `func (s deprecationSource) Drafts(_ context.Context) ([]Draft, error)`

### goadapter.go (17 LOC)
- L15: `func NewGoSource() Source`

### osvadapter.go (112 LOC)
- L28: `type osvAffected struct`
- L37: `type osvAdvisory struct`
- L49: `type osvSource struct{}`
- L52: `func NewOSVSource() Source { return osvSource{} }`
- L54: `func (osvSource) Name() string { return "osv" }`
- L59: `func (osvSource) Drafts(_ context.Context) ([]Draft, error)`
- L100: `func versionRange(introduced, fixed string) *record.VersionRange`

### prepare.go (242 LOC)
- L17: `type Draft struct`
- L34: `type Meta struct`
- L51: `type Outcome struct`
- L70: `func Prepare(ctx context.Context, ix *index.Index, repo string, d Draft, m Meta) (Outcome, error)`
- L142: `func scanTexts(r *record.Record) []string`
- L186: `func probes(d Draft) []string`
- L209: `func buildPath(now, id, title string) string`
- L224: `func slugify(title string) string`

### pyadapter.go (17 LOC)
- L15: `func NewPySource() Source`

### source.go (17 LOC)
- L12: `type Source interface`

## internal/pack

### pack.go (139 LOC)
- L28: `type Eligibility struct`
- L55: `func Classify(sourceLicense string) Eligibility`
- L88: `type AttributionEntry struct`
- L95: `type Excluded struct`
- L102: `type Manifest struct`
- L114: `func BuildManifest(recs []*record.Record, commercial, includeQuarantined bool) Manifest`

## internal/record

### marshal.go (29 LOC)
- L15: `func Marshal(r *Record) ([]byte, error)`

### record.go (460 LOC)
- L35: `type Record struct`
- L55: `type Symptom struct`
- L63: `type Fingerprints struct`
- L68: `type AppliesTo struct`
- L75: `type VersionRange struct`
- L80: `type Resolution struct`
- L86: `type DeadEnd struct`
- L91: `type Guard struct`
- L96: `type Provenance struct`
- L120: `type Source struct`
- L126: `type Validity struct`
- L131: `type Usage struct`
- L152: `func Parse(path string, src []byte) (*Record, error)`
- L175: `func ParseFile(root, rel string) (*Record, error)`
- L186: `func LoadCorpus(root string) ([]*Record, error)`
- L243: `func splitFrontmatter(src []byte) (front []byte, body string, err error)`
- L265: `func Validate(r *Record) error { return r.validate() }`
- L267: `func (r *Record) validate() error`
- L302: `func (r *Record) validatePath(fail func(string, ...any))`
- L316: `func (r *Record) validateSymptom(fail func(string, ...any))`
- L341: `func (r *Record) validateAppliesTo(fail func(string, ...any))`
- L349: `func (r *Record) validateResolution(fail func(string, ...any))`
- L375: `func (r *Record) validateGuard(fail func(string, ...any))`
- L385: `func (r *Record) validateProvenance(fail func(string, ...any))`
- L440: `func checkDate(fail func(string, ...any), field, v string) time.Time`
- L453: `func contains(xs []string, x string) bool`

## internal/screen

### screen.go (135 LOC)
- L19: `type Finding struct`
- L25: `type sigRule struct`
- L67: `func Scan(texts ...string) []Finding`
- L101: `func Flags(fs []Finding) []string`
- L111: `func mask(s string) string`
- L120: `func shannon(s string) float64`

## internal/server

### middleware.go (99 LOC)
- L31: `func withMaxBytes(n int64, next http.Handler) http.Handler`
- L41: `func withTimeout(d time.Duration, next http.Handler) http.Handler`
- L51: `type tokenBucket struct`
- L60: `func newTokenBucket(perSecond, burst float64) *tokenBucket`
- L70: `func (b *tokenBucket) allow() bool`
- L90: `func withRateLimit(b *tokenBucket, next http.Handler) http.Handler`

### record.go (193 LOC)
- L31: `type RecordArgs struct`
- L47: `type RecordResult struct`
- L67: `func validateRecordSize(args RecordArgs) error`
- L88: `func (h *handlers) record(ctx context.Context, _ *mcp.CallToolRequest, args RecordArgs) (*mcp.CallToolResult, RecordResult, error)`

### server.go (209 LOC)
- L27: `type Config struct`
- L61: `func New(cfg Config) (http.Handler, error)`
- L91: `type handlers struct`
- L98: `type SearchArgs struct`
- L107: `type SearchHit struct`
- L118: `type SearchResult struct`
- L128: `func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error)`
- L165: `type GetArgs struct`
- L170: `type GetResult struct`
- L179: `func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, GetResult, error)`
- L196: `func bearerAuth(token string, next http.Handler) http.Handler`

## scripts

### apply-branch-protection.sh (113 LOC)
- _(no top-level declarations matched)_

### check-workflows.sh (164 LOC)
- _(no top-level declarations matched)_

### codemap.sh (72 LOC)
- L14: `decl_pattern()`
- L28: `list_files()`

### new-issue.sh (78 LOC)
- _(no top-level declarations matched)_

### next-issue.sh (163 LOC)
- _(no top-level declarations matched)_

### start-fresh.sh (87 LOC)
- L26: `note() { printf '  %s\n' "$*"; }`
- L27: `fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }`

### sync-forgejo.sh (164 LOC)
- _(no top-level declarations matched)_


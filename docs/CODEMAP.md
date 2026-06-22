# CODEMAP — generated, do not edit by hand

> Regenerate with `scripts/codemap.sh`. A per-directory index of source
> files and their top-level declarations, so a session learns the layout
> from this one file instead of opening source to find a symbol. The source
> is the source of truth — if this looks stale, re-run the script.

_Last generated: 2026-06-22 (UTC)._

## cmd/twiceshy

### main.go (2527 LOC)
- L79: `func main()`
- L96: `func exitCode(err error) int`
- L114: `type brokerHealth interface`
- L117: `type judgeLive interface`
- L125: `func preflight(ctx context.Context, b brokerHealth, j judgeLive) error`
- L151: `func logSkippedPoison(logger *slog.Logger, out io.Writer, stage string, skipped []string)`
- L160: `func startupReap(ctx context.Context, stage string, dryRun bool, logger *slog.Logger, out io.Writer)`
- L183: `func parseFlags(fs *flag.FlagSet, args []string) error`
- L193: `func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L241: `type commonFlags struct`
- L249: `func addCommonFlags(fs *flag.FlagSet) *commonFlags`
- L261: `func embedderFor(c *commonFlags) index.Embedder`
- L280: `func loadAndRebuild(ctx context.Context, c *commonFlags, ix *index.Index, resilient bool) (int, error)`
- L324: `func buildIndex(ctx context.Context, c *commonFlags, resilient bool) (*index.Index, int, error)`
- L337: `func runIndex(ctx context.Context, args []string, out io.Writer) error`
- L356: `func runServe(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L474: `func runHealthcheck(ctx context.Context, args []string, out io.Writer) error`
- L505: `func importSource(name, ecosystem string) (ingest.Source, error)`
- L526: `func runIngest(ctx context.Context, args []string, out io.Writer) error`
- L614: `func batchKey(d ingest.Draft) string`
- L627: `func bumpID(id string) string`
- L636: `func safeJoin(base, rel string) (string, error)`
- L648: `func writeRecord(corpus string, rec *record.Record) error`
- L666: `func writeFileAtomic(dst string, data []byte, perm os.FileMode) error`
- L693: `type pipelineRunner interface`
- L698: `type draftStats struct`
- L711: `func runDraft(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L757: `func draftersFrom(getenv func(string) string) []drafter.Drafter`
- L774: `func isCandidate(rec *record.Record) bool`
- L785: `func draftCorpus(ctx context.Context, corpus string, recs []*record.Record, run pipelineRunner, persist func(string, *record.Record) error, out io.Writer) (draftStats, error)`
- L822: `func removeRepro(corpus, reproPath string)`
- L839: `func guardrailsFrom(getenv func(string) string, maxActions, maxRuns int) guard.Guardrails`
- L849: `func loopLogger(logger *slog.Logger) *slog.Logger`
- L858: `func loopAlerter(alerter notify.Alerter) notify.Alerter`
- L869: `func newRunLogger(runID string) *slog.Logger`
- L875: `func surfaceJudgeStats(runLog *slog.Logger, tj *judge.TimingJudge) *judge.JudgeStats`
- L891: `func newRunID() string`
- L902: `func acquireLoopLock(corpus string) (*lock.Lock, error)`
- L917: `type recordPromoter interface`
- L922: `type promoteStats struct`

### retro.go (211 LOC)
- L28: `func runRetroIntake(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error`
- L67: `func analyzerFromEnv(getenv func(string) string, model string, maxTraps int) (retro.Analyzer, error)`
- L88: `type retroOpts struct`
- L99: `func drainRetro(ctx context.Context, analyzer retro.Analyzer, ix *index.Index, repo, corpus, queue string, opts retroOpts, out io.Writer) error`
- L199: `func candidateDraft(c retro.Candidate) ingest.Draft`

### screen.go (38 LOC)
- L21: `func runScreen(args []string, in io.Reader, out io.Writer) error`

### selfaudit.go (59 LOC)
- L20: `func runSelfAudit(args []string, out io.Writer) error`

## docs/design/examples

### repro-go-ioutil-deprecation.sh (74 LOC)
- _(no top-level declarations matched)_

## experience/repro

### 0001-fts5-raw-match.sh (36 LOC)
- _(no top-level declarations matched)_

### 0017-go-test-noexec-tmpdir.sh (72 LOC)
- _(no top-level declarations matched)_

## experience/repro/exp-0043-io-ioutil/fix

### main.go (7 LOC)
- L5: `func main()`

## experience/repro/exp-0043-io-ioutil

### prepare.sh (4 LOC)
- _(no top-level declarations matched)_

### repro.sh (9 LOC)
- _(no top-level declarations matched)_

## experience/repro/exp-0043-io-ioutil/trap

### main.go (7 LOC)
- L5: `func main()`

## experience/repro/exp-0044-strings/fix

### main.go (10 LOC)
- L8: `func main()`

## experience/repro/exp-0044-strings

### prepare.sh (5 LOC)
- _(no top-level declarations matched)_

### repro.sh (9 LOC)
- _(no top-level declarations matched)_

## experience/repro/exp-0044-strings/trap

### main.go (7 LOC)
- L5: `func main()`

## experience/repro/exp-0045-math-rand/fix

### main.go (7 LOC)
- L5: `func main()`

## experience/repro/exp-0045-math-rand

### prepare.sh (4 LOC)
- _(no top-level declarations matched)_

### repro.sh (9 LOC)
- _(no top-level declarations matched)_

## experience/repro/exp-0045-math-rand/trap

### main.go (7 LOC)
- L5: `func main()`

## hooks

### session-retro.sh (54 LOC)
- L12: `fail_open() { exit 0; } # never block or error the session`

### twiceshy-push.sh (36 LOC)
- L5: `fail_open() { exit 0; }`

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

### staleness.go (197 LOC)
- L35: `type Staleness struct`
- L43: `func NewStaleness(eol EOLSource, now time.Time) *Staleness`
- L56: `func (*Staleness) Name() string { return "staleness" }`
- L62: `func majorMinor(v string) string`
- L81: `func isRuntimePackage(pkg string) bool`
- L93: `func parseDate(s string) (time.Time, bool)`
- L98: `func (s *Staleness) Run(ctx context.Context, recs []*record.Record) (Report, error)`
- L123: `func (s *Staleness) WouldFlag(ctx context.Context, r *record.Record) *Finding`
- L129: `func (s *Staleness) wouldFlag(ctx context.Context, r *record.Record) *Finding`
- L147: `func (s *Staleness) staleByEOL(ctx context.Context, r *record.Record) *Finding`
- L187: `func (s *Staleness) cycles(ctx context.Context, product string) ([]Cycle, bool)`

## internal/drafter

### drafter.go (35 LOC)
- L28: `type Drafter interface`

### go_deprecation.go (235 LOC)
- L29: `type goDeprecation struct`
- L42: `type goRequire struct`
- L78: `type GoDeprecationDrafter struct`
- L83: `func NewGoDeprecationDrafter() *GoDeprecationDrafter`
- L88: `func (*GoDeprecationDrafter) Name() string { return "go-deprecation-template" }`
- L93: `func (d *GoDeprecationDrafter) Draft(_ context.Context, root string, rec *record.Record) (string, error)`
- L115: `func emitGoDeprecationRepro(root string, rec *record.Record, tmpl goDeprecation) (string, error)`
- L146: `func goPackage(rec *record.Record) string`
- L157: `func diagnosticMatches(rec *record.Record, check string) bool`
- L170: `func slug(id, pkg string) string`
- L182: `func goMod(name string) string`
- L189: `func goModWithReqs(name string, reqs []goRequire) string`
- L208: `func prepareScript(warmFixDeps bool) string`
- L221: `func reproScript(check string) string`

### model.go (227 LOC)
- L33: `type ModelDrafter struct`
- L41: `func NewModelDrafter(endpoint, model string) *ModelDrafter`
- L52: `func (d *ModelDrafter) Name() string { return "model-drafter(" + d.model + ")" }`
- L58: `func (d *ModelDrafter) Draft(ctx context.Context, root string, rec *record.Record) (string, error)`
- L92: `func (d *ModelDrafter) complete(ctx context.Context, system, user string) (string, error)`
- L131: `type modelDraftJSON struct`
- L145: `func parseModelDraft(raw string) (goDeprecation, error)`
- L175: `func extractJSONObject(s string) string`
- L201: `func buildModelDraftPrompt(rec *record.Record) string`

### pipeline.go (126 LOC)
- L17: `type Outcome struct`
- L39: `type Pipeline struct`
- L48: `func NewPipeline(rv *repro.Revalidator, root string, drafters ...Drafter) *Pipeline`
- L57: `func (p *Pipeline) Run(ctx context.Context, rec *record.Record) (Outcome, error)`
- L118: `func (p *Pipeline) detach(rec *record.Record, dir string)`

## internal/eval

### eval.go (264 LOC)
- L31: `type Case struct`
- L38: `type Searcher interface`
- L43: `type CaseResult struct`
- L52: `func (r CaseResult) NearMiss() bool`
- L57: `type Report struct`
- L74: `func Cases(recs []*record.Record) []Case`
- L99: `func Run(ctx context.Context, s Searcher, cases []Case, k int) (Report, error)`
- L147: `type PushCase struct`
- L154: `type Pusher interface`
- L164: `func PushNegatives() []PushCase`
- L191: `func PushPositives() []PushCase`
- L203: `type PushReport struct`
- L213: `func (r PushReport) Precision() float64`
- L221: `func (r PushReport) Recall() float64`
- L230: `func RunPush(ctx context.Context, p Pusher, cases []PushCase) (PushReport, error)`

## internal/fingerprint

### fingerprint.go (60 LOC)
- L35: `func Normalize(s string) string`
- L47: `func Generic(signature string) string`
- L53: `func App(repo, signature string) string`
- L57: `func hash(input string) string`

## internal/guard

### guard.go (76 LOC)
- L21: `type Guardrails struct`
- L34: `func (g Guardrails) Engaged() bool { return g.Paused }`
- L37: `func (g Guardrails) Budget() *Budget { return &Budget{g: g} }`
- L41: `type Budget struct`
- L49: `func (b *Budget) AllowRun() bool { return b.g.MaxRuns == 0 || b.runs < b.g.MaxRuns }`
- L52: `func (b *Budget) StartRun() { b.runs++ }`
- L55: `func (b *Budget) Runs() int { return b.runs }`
- L58: `func (b *Budget) CountAction() { b.actions++ }`
- L61: `func (b *Budget) Actions() int { return b.actions }`
- L66: `func (b *Budget) Anomalous() bool { return b.g.MaxActions > 0 && b.actions > b.g.MaxActions }`
- L69: `func Truthy(s string) bool`

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

### index.go (747 LOC)
- L101: `func wordSet(s string) map[string]bool`
- L157: `type types ui update updated updates upgrade upload url use user users util value values`
- L172: `type Index struct`
- L177: `type Query struct`
- L200: `type Hit struct`
- L212: `type Stored struct`
- L279: `func Open(path string) (*Index, error)`
- L300: `func (ix *Index) Close() error { return ix.db.Close() }`
- L306: `func (ix *Index) Rebuild(ctx context.Context, recs []*record.Record, repo string) error`
- L329: `func insertRecord(ctx context.Context, tx *sql.Tx, r *record.Record, repo string) error`
- L381: `func floorPolicy(q Query) Query`
- L393: `func (ix *Index) Retrieve(ctx context.Context, q Query) ([]Hit, error)`
- L404: `func (ix *Index) RetrievePush(ctx context.Context, q Query) ([]Hit, error)`
- L412: `type PushDecision struct`
- L421: `func (ix *Index) RetrievePushTraced(ctx context.Context, q Query) (PushDecision, error)`
- L454: `func (ix *Index) discriminativeTokens(ctx context.Context, text string) ([]string, error)`
- L485: `func (ix *Index) validatedDF(ctx context.Context, tok string) (int, error)`
- L499: `func (ix *Index) ecosystemNames(ctx context.Context) (map[string]bool, error)`
- L522: `func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error)`
- L550: `func (ix *Index) fingerprintHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L586: `func (ix *Index) lexicalHits(ctx context.Context, q Query, k int) ([]Hit, error)`
- L630: `func appendStatusFilter(sb *strings.Builder, args []any, q Query) []any`
- L639: `func appendStackFilter(sb *strings.Builder, args []any, q Query) []any`
- L671: `func (ix *Index) NextID(ctx context.Context) (string, error)`
- L683: `func (ix *Index) Get(ctx context.Context, id string) (*Stored, error)`
- L701: `func ftsQuery(text string) string`
- L720: `func ftsPhrase(tok string) string`
- L728: `func stripControl(s string) string`
- L740: `func hasAlnum(s string) bool`

### usage.go (144 LOC)
- L30: `func (ix *Index) RecordHits(ctx context.Context, ids []string, date string) error`
- L59: `func (ix *Index) RecordPushes(ctx context.Context, ids []string) error`
- L82: `func (ix *Index) ConfirmHelpful(ctx context.Context, id string) error`
- L95: `func (ix *Index) Usage(ctx context.Context, id string) (record.Usage, error)`
- L117: `func (ix *Index) AllUsage(ctx context.Context) (map[string]record.Usage, error)`

## internal/ingest

### deprecation.go (70 LOC)
- L17: `type deprecation struct`
- L31: `type deprecationSource struct`
- L37: `func (s deprecationSource) Name() string { return s.name }`
- L39: `func (s deprecationSource) Drafts(_ context.Context) ([]Draft, error)`

### goadapter.go (17 LOC)
- L15: `func NewGoSource() Source`

### nextid.go (45 LOC)
- L26: `func NextID(ctx context.Context, ix *index.Index, corpusRoot string) (string, error)`

### osvadapter.go (89 LOC)
- L24: `type osvAffected struct`
- L33: `type osvAdvisory struct`
- L45: `type osvSource struct{}`
- L48: `func NewOSVSource() Source { return osvSource{} }`
- L50: `func (osvSource) Name() string { return "osv" }`
- L55: `func (osvSource) Drafts(_ context.Context) ([]Draft, error)`

### osvlive.go (302 LOC)
- L32: `type OSVLiveSource struct`
- L38: `type OSVLiveOption func(*OSVLiveSource)`
- L42: `func WithOSVLiveFetch(fetch func(context.Context) (io.ReadCloser, error)) OSVLiveOption`
- L51: `func WithEcosystem(ecosystem string) OSVLiveOption`
- L61: `func NewOSVLiveSource(opts ...OSVLiveOption) Source`
- L73: `func osvLiveFetcher(ecosystem string) func(context.Context) (io.ReadCloser, error)`
- L93: `func (s *OSVLiveSource) Name() string { return "osv-live" }`
- L97: `func (s *OSVLiveSource) Drafts(ctx context.Context) ([]Draft, error)`
- L120: `func draftsFromZip(zr *zip.Reader, ecosystem string) ([]Draft, error)`
- L146: `type osvLiveRecord struct`
- L156: `type osvLiveAffected struct`
- L161: `type osvLivePackage struct`
- L166: `type osvLiveRange struct`
- L170: `type osvLiveEvent struct`
- L175: `type osvLiveRef struct`
- L180: `func mapOSVLiveRecord(rec osvLiveRecord, ecosystem string) (Draft, bool)`
- L250: `func osvLiveFixText(applies []record.AppliesTo, sourceURL string) string`
- L259: `func osvLiveRangeEvents(events []osvLiveEvent) (introduced, fixed string)`
- L271: `func osvLiveGHSAURL(refs []osvLiveRef) string`
- L280: `func osvLiveBody(id string, applies []record.AppliesTo, sourceURL string) string`

### osvmap.go (56 LOC)
- L16: `type osvDraftInput struct`
- L29: `func buildOSVDraft(in osvDraftInput) Draft`
- L44: `func versionRange(introduced, fixed string) *record.VersionRange`

### prepare.go (248 LOC)
- L17: `type Draft struct`
- L34: `type Meta struct`
- L51: `type Outcome struct`
- L70: `func Prepare(ctx context.Context, ix *index.Index, repo string, d Draft, m Meta) (Outcome, error)`
- L142: `func scanTexts(r *record.Record) []string`
- L192: `func probes(d Draft) []string`
- L215: `func BuildPath(now, id, title string) string`
- L230: `func slugify(title string) string`

### pyadapter.go (17 LOC)
- L15: `func NewPySource() Source`

### report.go (125 LOC)
- L15: `type ReportInput struct`
- L43: `func BuildReport(in ReportInput, m Meta) (*record.Record, error)`
- L119: `func capRunes(s string, n int) string`

### source.go (17 LOC)
- L12: `type Source interface`

## internal/judge

### judge.go (155 LOC)
- L27: `type Decision string`
- L38: `type CheckName string`
- L57: `type Check struct`
- L66: `type Verdict struct`
- L78: `func (v Verdict) Approved() bool`
- L99: `func ApproveVerdict(model string) Verdict`
- L110: `type ReproArtifact struct`
- L120: `type Request struct`
- L134: `type Judge interface`
- L142: `type StubJudge struct`
- L149: `func (s *StubJudge) Judge(_ context.Context, _ Request) (Verdict, error)`

### majority.go (65 LOC)
- L20: `type MajorityJudge struct`
- L28: `func NewMajority(inner Judge, votes int) Judge`
- L39: `func (m MajorityJudge) Judge(ctx context.Context, req Request) (Verdict, error)`

### model.go (351 LOC)
- L45: `func FamilyOf(model string) string`
- L55: `type Config struct`
- L85: `type ModelJudge struct`
- L97: `func NewModelJudge(cfg Config) (*ModelJudge, error)`
- L128: `type wireRequest struct`
- L138: `type wireVerdict struct`
- L152: `func (j *ModelJudge) Ping(ctx context.Context) error`
- L169: `func (j *ModelJudge) Judge(ctx context.Context, req Request) (Verdict, error)`
- L219: `func toVerdict(wv wireVerdict, model string) (Verdict, error)`
- L249: `func BuildPrompt(req Request) string`
- L296: `func BuildAdvisoryPrompt(req Request) string`

### panel.go (97 LOC)
- L15: `type PanelMember struct`
- L24: `type PanelJudge struct`
- L31: `func NewPanel(members ...PanelMember) (Judge, error)`
- L57: `func (p *PanelJudge) PanelMembers() []Verdict`
- L63: `func (p *PanelJudge) Judge(ctx context.Context, req Request) (Verdict, error)`

### system.go (128 LOC)
- _(no top-level declarations matched)_

### timing.go (91 LOC)
- L17: `type JudgeStats struct`
- L27: `type TimingJudge struct`
- L38: `func NewTiming(inner Judge) *TimingJudge`
- L44: `func (t *TimingJudge) Judge(ctx context.Context, req Request) (Verdict, error)`
- L65: `func (t *TimingJudge) Stats() JudgeStats`
- L80: `func percentileMS(durations []time.Duration, p float64) int64`

## internal/judgeeval

### eval.go (320 LOC)
- L19: `func Run(ctx context.Context, caller judge.Judge, cases []Case, repeat int) (Result, error)`
- L33: `func (res *Result) tally(c Case, o Outcome)`
- L62: `func (res *Result) finalize()`
- L80: `func RunConfirm(ctx context.Context, caller judge.Judge, cases []Case, base, total int) (Result, error)`
- L114: `func sampleCase(ctx context.Context, caller judge.Judge, c Case, n int) (approvals, rejects, errs int, approveV, rejectV judge.Verdict, lastErr error)`
- L136: `func scoreCase(ctx context.Context, caller judge.Judge, c Case, repeat int) Outcome`
- L141: `func scoreFromTallies(c Case, approvals, rejects, errs int, approveV, rejectV judge.Verdict, lastErr error, samples int) Outcome`
- L186: `func failingChecks(v judge.Verdict) []judge.CheckName`
- L199: `func caughtExpected(want, got []judge.CheckName) bool`
- L215: `func rejectErrors(os []Outcome) int`
- L226: `type Outcome struct`
- L255: `type Result struct`
- L280: `func (r Result) ByMode() []ModeStat`
- L313: `type ModeStat struct`

### gold.go (227 LOC)
- L34: `type Case struct`
- L48: `func (c Case) Request() judge.Request`
- L62: `func (c Case) ShouldReject() bool { return c.WantDecision == judge.Reject }`
- L67: `type goldFile struct`
- L93: `func LoadGold() ([]Case, error)`
- L191: `func validMode(m string) bool`
- L200: `func parseChecks(where string, names []string) ([]judge.CheckName, []error)`
- L220: `func knownCheck(c judge.CheckName) bool`

### goldadd.go (147 LOC)
- L16: `type GoldStanzaInput struct`
- L32: `func GoldCaseStanza(in GoldStanzaInput) (string, error)`
- L101: `func validateStanzaInput(where string, in GoldStanzaInput) error`

## internal/lock

### lock.go (48 LOC)
- L21: `type Lock struct{ f *os.File }`
- L26: `func Acquire(path string) (*Lock, error)`
- L42: `func (l *Lock) Release() error`

## internal/notify

### notify.go (103 LOC)
- L22: `type Alerter interface`
- L28: `type NopAlerter struct{}`
- L31: `func (NopAlerter) Alert(context.Context, string, string) {}`
- L36: `type HTTPNotifier struct`
- L44: `func New(url string, logger *slog.Logger) Alerter`
- L58: `func (n *HTTPNotifier) Alert(ctx context.Context, event, message string)`
- L81: `func Heartbeat(ctx context.Context, url string, logger *slog.Logger)`

## internal/pack

### pack.go (139 LOC)
- L28: `type Eligibility struct`
- L55: `func Classify(sourceLicense string) Eligibility`
- L88: `type AttributionEntry struct`
- L95: `type Excluded struct`
- L102: `type Manifest struct`
- L114: `func BuildManifest(recs []*record.Record, commercial, includeQuarantined bool) Manifest`

## internal/promote

### adapt.go (169 LOC)
- L22: `type Action string`
- L39: `type CounterEvidence struct`
- L46: `type AdaptOutcome struct`
- L58: `type Adapter struct`
- L64: `type AdaptOption func(*Adapter)`
- L67: `func WithAdaptClock(now func() string) AdaptOption { return func(a *Adapter) { a.now = now } }`
- L70: `func NewAdapter(j judge.Judge, opts ...AdaptOption) *Adapter`
- L83: `func (a *Adapter) Adapt(ctx context.Context, original, report *record.Record, ev CounterEvidence, corroborating int) (AdaptOutcome, error)`
- L130: `func none(reason string) AdaptOutcome { return AdaptOutcome{Action: ActionNone, Reason: reason} }`
- L134: `func (a *Adapter) demote(original, report *record.Record, ev CounterEvidence, verdict judge.Verdict) error`
- L161: `func (a *Adapter) dispute(original *record.Record) error`

### journal.go (77 LOC)
- L15: `type Journal struct`
- L24: `type JournalStop struct`
- L30: `func JournalPath(corpus, stage string) string`
- L35: `func LoadJournal(path string) (*Journal, error)`
- L51: `func (j *Journal) Save(path string) error`
- L71: `func (j *Journal) DoneIDs() map[string]bool`

### manifest.go (56 LOC)
- L17: `type RecordAction struct`
- L34: `type RunManifest struct`
- L49: `func (m RunManifest) WriteJSON(w io.Writer) error`

### promote.go (404 LOC)
- L34: `type Attestor interface`
- L39: `type Promoter struct`
- L49: `type Option func(*Promoter)`
- L53: `func WithReproReader(f func(string) (string, error)) Option`
- L58: `func WithClock(now func() string) Option { return func(p *Promoter) { p.now = now } }`
- L62: `func WithAdvisoryPanel(j judge.Judge) Option`
- L72: `func WithStalenessGate(f func(context.Context, *record.Record) *doctor.Finding) Option`
- L78: `func NewPromoter(attestor Attestor, j judge.Judge, root string, opts ...Option) *Promoter`
- L92: `type Outcome struct`
- L99: `func skip(reason string) (Outcome, error) { return Outcome{Reason: reason}, nil }`
- L102: `func todayUTC() string { return time.Now().UTC().Format("2006-01-02") }`
- L109: `func Eligible(rec *record.Record) (bool, string)`
- L125: `func EligibleAdvisory(rec *record.Record) (bool, string)`
- L141: `func Promotable(rec *record.Record) (bool, string)`
- L153: `func (p *Promoter) Promote(ctx context.Context, rec *record.Record) (Outcome, error)`
- L212: `func (p *Promoter) promoteAdvisory(ctx context.Context, rec *record.Record) (Outcome, error)`
- L251: `type panelMemberRecorder interface`
- L255: `func panelVerdicts(j judge.Judge) []record.PanelVerdict`
- L275: `func RepromoteEligible(rec *record.Record) (bool, string)`
- L292: `func (p *Promoter) Repromote(ctx context.Context, rec *record.Record) (Outcome, error)`
- L350: `func (p *Promoter) reproArtifacts(rec *record.Record) ([]judge.ReproArtifact, error)`
- L379: `func readReproFile(root, rel string) (string, error)`

## internal/record

### advisory.go (43 LOC)
- L14: `func IsAdvisoryClass(rec *Record) bool`
- L35: `func hasVulnIDPrefix(s string) bool`

### marshal.go (29 LOC)
- L15: `func Marshal(r *Record) ([]byte, error)`

### maxid.go (53 LOC)
- L20: `func MaxID(root string) (int, error)`

### record.go (752 LOC)
- L29: `func ValidID(id string) bool { return reID.MatchString(id) }`
- L39: `type Record struct`
- L59: `type Symptom struct`
- L67: `type Fingerprints struct`
- L72: `type AppliesTo struct`
- L79: `type VersionRange struct`
- L84: `type Resolution struct`
- L90: `type DeadEnd struct`
- L101: `type Repro struct`
- L107: `type Guard struct`
- L113: `type Provenance struct`
- L152: `type Source struct`
- L158: `type Validity struct`
- L163: `type Usage struct`
- L178: `type PanelVerdict struct`
- L186: `type Promotion struct`
- L201: `type Demotion struct`
- L225: `func Parse(path string, src []byte) (*Record, error)`
- L236: `func ParseLenient(path string, src []byte) (*Record, error)`
- L240: `func parseRecord(path string, src []byte, knownFields bool) (*Record, error)`
- L263: `func ParseFile(root, rel string) (*Record, error)`
- L268: `func ParseFileLenient(root, rel string) (*Record, error)`
- L272: `func parseFileWith(root, rel string, parse func(string, []byte) (*Record, error)) (*Record, error)`
- L283: `func LoadCorpus(root string) ([]*Record, error)`
- L356: `func LoadCorpusResilient(root string) (recs []*Record, skipped []string, err error)`
- L369: `func LoadCorpusForServe(root string) (recs []*Record, skipped []string, err error)`
- L377: `func walkCorpusSkipping(root string, parseFile func(root, rel string) (*Record, error)) (recs []*Record, skipped []string, err error)`
- L419: `func splitFrontmatter(src []byte) (front []byte, body string, err error)`
- L441: `func Validate(r *Record) error { return r.validate() }`
- L443: `func (r *Record) validate() error`
- L478: `func (r *Record) validatePath(fail func(string, ...any))`
- L492: `func (r *Record) validateSymptom(fail func(string, ...any))`
- L517: `func (r *Record) validateAppliesTo(fail func(string, ...any))`
- L525: `func (r *Record) validateResolution(fail func(string, ...any))`
- L551: `func (r *Record) validateGuard(fail func(string, ...any))`
- L592: `func HasPositiveRepro(r *Record) bool { return r.hasPositiveRepro() }`
- L596: `func (r *Record) hasPositiveRepro() bool`
- L611: `func (r *Record) validateProvenance(fail func(string, ...any))`
- L732: `func checkDate(fail func(string, ...any), field, v string) time.Time`
- L745: `func contains(xs []string, x string) bool`

## internal/repro

### broker.go (554 LOC)
- L55: `type Broker interface`
- L68: `type Job struct`
- L94: `type Result struct`
- L103: `type PhaseResult struct`
- L113: `type Limits struct`
- L151: `type dockerBroker struct`
- L161: `type Option func(*dockerBroker)`
- L164: `func WithLimits(l Limits) Option { return func(b *dockerBroker) { b.limits = l } }`
- L167: `func WithRuntime(rt string) Option { return func(b *dockerBroker) { b.runtime = rt } }`
- L170: `func withRunner(r commandRunner) Option { return func(b *dockerBroker) { b.runner = r } }`
- L173: `func withIDFunc(f func() (string, error)) Option { return func(b *dockerBroker) { b.newID = f } }`
- L176: `func WithLogger(l *slog.Logger) Option`
- L188: `func NewBroker(allowedImages []string, opts ...Option) Broker`
- L215: `func (b *dockerBroker) Healthy(ctx context.Context) error`
- L234: `func (b *dockerBroker) Run(ctx context.Context, job Job) (Result, error)`
- L290: `func (b *dockerBroker) validate(job Job) error`
- L327: `func (b *dockerBroker) populate(ctx context.Context, id, vol string, job Job) error`
- L355: `func (b *dockerBroker) runPhase(ctx context.Context, id, vol, phase, network, user string, job Job, cmd []string) PhaseResult`
- L386: `func (b *dockerBroker) sandboxArgs(id, vol, phase, network, user string, capAdd []string) []string`
- L415: `func (b *dockerBroker) policyArgs(id, vol, phase, network, user string, env map[string]string) []string`
- L427: `func (b *dockerBroker) kill(id string)`
- L437: `func (b *dockerBroker) cleanup(id, vol string)`
- L453: `func (b *dockerBroker) removeContainersByLabel(ctx context.Context, label string)`
- L475: `func (b *dockerBroker) phaseTimeout(job Job) time.Duration`
- L485: `func screenFiles(files map[string][]byte) error`
- L498: `func safeRelPath(p string) error`
- L514: `func makeTar(files map[string][]byte) ([]byte, error)`
- L538: `func sortedKeys[V any](m map[string]V) []string`
- L548: `func randomID() (string, error)`

### reaper.go (49 LOC)
- L15: `type Reaper struct`
- L20: `func NewReaper() *Reaper { return &Reaper{runner: dockerRunner{}} }`
- L25: `func (r *Reaper) Reap(ctx context.Context) (containers, volumes int, err error)`

### revalidate.go (411 LOC)
- L30: `type Revalidator struct`
- L41: `type MatrixEntry struct`
- L53: `type Attestation struct`
- L64: `type MatrixResult struct`
- L72: `type ReproOutcome struct`
- L81: `type RevalOption func(*Revalidator)`
- L84: `func WithMatrix(m []MatrixEntry) RevalOption { return func(r *Revalidator) { r.matrix = m } }`
- L87: `func WithClock(now func() time.Time) RevalOption { return func(r *Revalidator) { r.now = now } }`
- L91: `func NewRevalidator(broker Broker, root string, opts ...RevalOption) *Revalidator`
- L105: `func (*Revalidator) Name() string { return "revalidate" }`
- L110: `func (r *Revalidator) Run(ctx context.Context, recs []*record.Record) (doctor.Report, error)`
- L118: `func (r *Revalidator) RunWithAttestations(ctx context.Context, recs []*record.Record) (doctor.Report, []Attestation, error)`
- L135: `type reproRef struct`
- L140: `func reprosOf(rec *record.Record) []reproRef`
- L156: `func (r *Revalidator) revalidateOne(ctx context.Context, rec *record.Record, repros []reproRef) (Attestation, doctor.Finding)`
- L194: `func (r *Revalidator) runRepro(ctx context.Context, entry MatrixEntry, rp reproRef) ReproOutcome`
- L248: `func (r *Revalidator) stage(p string) (Job, error)`
- L287: `func (r *Revalidator) resolve(p string) (string, error)`
- L297: `func readDirFiles(dir string) (map[string][]byte, error)`
- L330: `func (r *Revalidator) toFinding(rec *record.Record, att Attestation) doctor.Finding`
- L354: `func entryHolds(mr MatrixResult) bool`
- L368: `func brokenRepros(att Attestation) []string`
- L389: `func appliesToSanityCheck(rec *record.Record, reproducedUnder []string) string`
- L402: `func firstLine(stdout, stderr string) string`

### runner.go (100 LOC)
- L20: `type capWriter struct`
- L26: `func newCapWriter(n int) *capWriter { return &capWriter{remaining: n} }`
- L28: `func (w *capWriter) Write(p []byte) (int, error)`
- L43: `func (w *capWriter) String() string`
- L51: `type execResult struct`
- L61: `type commandRunner interface`
- L67: `type dockerRunner struct{}`
- L69: `func (dockerRunner) run(ctx context.Context, stdin []byte, timeout time.Duration, name string, args ...string) (execResult, error)`

## internal/retro

### analyzer.go (114 LOC)
- L22: `type Candidate struct`
- L39: `type Analyzer interface`
- L44: `type StubAnalyzer struct`
- L52: `func (s *StubAnalyzer) Analyze(_ context.Context, transcript string) ([]Candidate, error)`
- L71: `func frameTranscript(transcript string) string`
- L81: `func stripControl(s string) string`
- L96: `func buildPrompt(framedTranscript string, maxTraps int) string`

### model.go (149 LOC)
- L29: `type ModelConfig struct`
- L50: `type ModelAnalyzer struct`
- L59: `func NewModelAnalyzer(cfg ModelConfig) (*ModelAnalyzer, error)`
- L79: `type wireRequest struct`
- L87: `type wireCandidates struct`
- L91: `type wireCandidate struct`
- L105: `func (a *ModelAnalyzer) Analyze(ctx context.Context, transcript string) ([]Candidate, error)`

## internal/screen

### screen.go (167 LOC)
- L19: `type Finding struct`
- L25: `type sigRule struct`
- L70: `func Scan(texts ...string) []Finding`
- L108: `func ExecutionHazards(findings []Finding) []Finding`
- L122: `func HasSecret(findings []Finding) bool`
- L133: `func Flags(fs []Finding) []string`
- L143: `func mask(s string) string`
- L152: `func shannon(s string) float64`

## internal/selfaudit

### selfaudit.go (232 LOC)
- L20: `type Dep struct`
- L26: `type Hit struct`
- L40: `func ParseGoMod(r io.Reader) ([]Dep, error)`
- L74: `func parseRequire(s string) (Dep, bool)`
- L82: `func stripComment(s string) string`
- L93: `func Audit(deps []Dep, recs []*record.Record) []Hit`
- L119: `func affected(v string, vr *record.VersionRange) bool`
- L142: `func advisoryID(rec *record.Record) string`
- L157: `func rangeStr(vr *record.VersionRange) (introduced, fixed string)`
- L179: `func cmpVer(a, b string) int`
- L212: `func parseVer(v string) (parts []int, hasPre bool)`

## internal/server

### confirm.go (65 LOC)
- L24: `type ConfirmArgs struct`
- L31: `type ConfirmResult struct`
- L39: `func (h *handlers) confirmHelpful(ctx context.Context, _ *mcp.CallToolRequest, args ConfirmArgs) (*mcp.CallToolResult, ConfirmResult, error)`

### issue.go (175 LOC)
- L38: `type IssueArgs struct`
- L48: `type IssueResult struct`
- L59: `func (h *handlers) reportIssue(_ context.Context, _ *mcp.CallToolRequest, args IssueArgs) (*mcp.CallToolResult, IssueResult, error)`
- L133: `func validateIssueSize(args IssueArgs) error`
- L146: `func renderIssueMarkdown(title, description, category, relatedID, author, session string, now time.Time, flags []string) string`

### logging.go (48 LOC)
- L14: `func errorClass(err error) string`
- L41: `func clientError(class string) bool`

### middleware.go (172 LOC)
- L34: `func withMaxBytes(n int64, next http.Handler) http.Handler`
- L44: `func withTimeout(d time.Duration, next http.Handler) http.Handler`
- L54: `type tokenBucket struct`
- L63: `func newTokenBucket(perSecond, burst float64) *tokenBucket`
- L73: `func (b *tokenBucket) allow() bool`
- L93: `func withRateLimit(logger *slog.Logger, b *tokenBucket, next http.Handler) http.Handler`
- L109: `type responseRecorder struct`
- L114: `func (r *responseRecorder) WriteHeader(code int)`
- L119: `func (r *responseRecorder) Write(b []byte) (int, error)`
- L126: `func (r *responseRecorder) Flush()`
- L132: `func (r *responseRecorder) Unwrap() http.ResponseWriter`
- L136: `func newRequestID() string`
- L145: `func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler`

### push.go (155 LOC)
- L21: `type PushArgs struct`
- L31: `type PushResult struct`
- L37: `func (h *handlers) pushHTTP(w http.ResponseWriter, r *http.Request)`
- L139: `func (h *handlers) recordPushDecision(query string, d index.PushDecision)`

### record.go (216 LOC)
- L32: `type RecordArgs struct`
- L48: `type RecordResult struct`
- L68: `func validateRecordSize(args RecordArgs) error`
- L89: `func (h *handlers) record(ctx context.Context, _ *mcp.CallToolRequest, args RecordArgs) (*mcp.CallToolResult, RecordResult, error)`
- L208: `func (h *handlers) logRecordOK(tool string, start time.Time, result RecordResult)`

### render.go (176 LOC)
- L25: `func sanitizeForTransport(s string) string`
- L47: `func capText(s string, max int) string`
- L59: `func neutralizeEndDelimiter(body string) string`
- L69: `func renderEnvelope(recordType, trust, id, body string) string`
- L86: `func renderGetExperience(status, id, markdown string) string`
- L92: `func RenderTrapCard(rec *record.Record) string`
- L110: `func formatAppliesTo(items []record.AppliesTo) string`
- L152: `func RenderPushContext(cards []string) string`
- L159: `func renderSearchResults(hits []SearchHit) string`

### report.go (166 LOC)
- L34: `type ReportArgs struct`
- L43: `type ReportResult struct`
- L54: `func (h *handlers) reportOutcome(ctx context.Context, _ *mcp.CallToolRequest, args ReportArgs) (*mcp.CallToolResult, ReportResult, error)`
- L158: `func validateReportSize(args ReportArgs) error`

### retro.go (114 LOC)
- L20: `type RetroArgs struct`
- L29: `type RetroResult struct`
- L40: `func (h *handlers) retroHTTP(w http.ResponseWriter, r *http.Request)`

### server.go (415 LOC)
- L32: `type Config struct`
- L98: `type Server struct`
- L105: `func (s *Server) SetRecordCount(n int) { s.h.recordCount.Store(int64(n)) }`
- L108: `func New(cfg Config) (*Server, error)`
- L170: `func (h *handlers) healthz(w http.ResponseWriter, _ *http.Request)`
- L179: `func (h *handlers) readyz(w http.ResponseWriter, _ *http.Request)`
- L191: `type handlers struct`
- L206: `type SearchArgs struct`
- L215: `type SearchHit struct`
- L226: `type SearchResult struct`
- L236: `func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error)`
- L301: `func (h *handlers) recordSearchDecision(query string, hits []index.Hit)`
- L318: `type GetArgs struct`
- L323: `type GetResult struct`
- L332: `func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, GetResult, error)`
- L366: `func (h *handlers) logToolError(tool string, start time.Time, err error, extra ...slog.Attr)`
- L386: `func bearerAuth(logger *slog.Logger, token string, next http.Handler) http.Handler`
- L404: `func bearerRejectReason(got, token, prefix string) string`

### usage.go (107 LOC)
- L20: `type usageStore interface`
- L38: `type usageRecorder struct`
- L45: `func newUsageRecorder(store usageStore, log *slog.Logger, now func() time.Time) *usageRecorder`
- L54: `func (u *usageRecorder) record(ids []string)`
- L80: `func (u *usageRecorder) recordPush(ids []string)`
- L103: `func (u *usageRecorder) flush()`

## internal/spool

### spool.go (185 LOC)
- L22: `type Report struct`
- L33: `func Enqueue(dir string, r Report) (string, error)`
- L42: `func enqueueJSON(dir, timePrefix string, v any) (string, error)`
- L72: `func List(dir string) ([]string, error)`
- L91: `func Read(path string) (Report, error)`
- L109: `type Transcript struct`
- L119: `func EnqueueTranscript(dir string, t Transcript) (string, error)`
- L124: `func ReadTranscript(path string) (Transcript, error)`
- L142: `type Issue struct`
- L154: `func EnqueueIssue(dir string, i Issue) (string, error)`
- L159: `func ReadIssue(path string) (Issue, error)`
- L172: `func Remove(path string) error { return os.Remove(path) }`
- L176: `func sanitize(s string) string`

## internal/telemetry

### decision.go (218 LOC)
- L29: `type ServedHit struct`
- L37: `type Decision struct`
- L48: `type Config struct`
- L59: `type Recorder struct`
- L76: `func NewRecorder(cfg Config) (*Recorder, error)`
- L114: `func (r *Recorder) Hash(query string) string`
- L127: `func (r *Recorder) Record(d Decision)`
- L146: `func (r *Recorder) run()`
- L182: `func (r *Recorder) rotate()`
- L203: `func (r *Recorder) Close() error`
- L213: `func (r *Recorder) Dropped() int64`

## scripts

### apply-branch-protection.sh (116 LOC)
- _(no top-level declarations matched)_

### check-workflows.sh (164 LOC)
- _(no top-level declarations matched)_

### codemap.sh (72 LOC)
- L14: `decl_pattern()`
- L28: `list_files()`

### continuous-pump.sh (37 LOC)
- _(no top-level declarations matched)_

### corpus-stall-alarm.sh (102 LOC)
- L25: `now() { date +%s; }`
- L26: `notify()`
- L35: `list_pipeline_prs()`
- L68: `main()`

### corpus-stall-alarm.test.sh (88 LOC)
- L12: `ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }`
- L13: `bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }`
- L14: `check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }`
- L15: `contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }`
- L27: `reset() { PRS=""; ALERTS=""; rm -f "$STATE_FILE"; }`
- L29: `list_pipeline_prs() { printf '%s' "$PRS"; }`
- L30: `now() { echo "$NOW"; }`
- L31: `notify() { ALERTS="${ALERTS}|$1"; }`

### daily-audit.sh (153 LOC)
- L29: `notify()`

### gemini-judge-shim.py (227 LOC)
- L52: `def _pace():`
- L63: `def _retry_backoff(err, attempt):`
- L75: `def _api_key():`
- L132: `def call_gemini(model, prompt, system=None):`
- L180: `class Handler(BaseHTTPRequestHandler):`

### new-issue.sh (78 LOC)
- _(no top-level declarations matched)_

### next-issue.sh (163 LOC)
- _(no top-level declarations matched)_

### ollama-watchdog.sh (16 LOC)
- _(no top-level declarations matched)_

### scheduled-import.sh (137 LOC)
- L52: `notify() { [ -n "$NTFY_URL" ] && curl -fsS -d "$1" "$NTFY_URL" >/dev/null 2>&1 || true; }`

### scheduled-validate.sh (204 LOC)
- L54: `notify()`
- L86: `merge_due()`
- L129: `abort()`

### sonnet-judge-shim.py (158 LOC)
- L78: `def _extract_verdict(text):`
- L94: `def call_sonnet(model, prompt, system=None):`
- L113: `class Handler(BaseHTTPRequestHandler):`

### start-fresh.sh (87 LOC)
- L26: `note() { printf '  %s\n' "$*"; }`
- L27: `fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }`

### sync-corpus-to-nas.sh (186 LOC)
- L48: `ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$NAS" "$@"; }`
- L51: `now() { date +%s; }`
- L54: `health_probe() { curl -fsS -m 5 "$HEALTH_URL" >/dev/null 2>&1; }`
- L57: `alert()`
- L66: `container_running()`
- L75: `healthz_wait()`
- L86: `_diag()`
- L95: `restart_container()`
- L106: `ensure_healthy()`
- L127: `main()`

### sync-corpus-to-nas.test.sh (132 LOC)
- L15: `ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }`
- L16: `bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }`
- L17: `check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }`
- L18: `contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }`
- L34: `reset() { RUNNING=true; HEALTHY=0; START_HEALS=1; SSH_LOG=""; ALERTS=""; MARKER_STAMPED=0; rm -f "$BREAKER_FILE"; }`
- L36: `health_probe() { return "$HEALTHY"; }`
- L37: `alert() { ALERTS="${ALERTS}|$1"; }`
- L38: `git()`
- L47: `ssh_nas()`
- L95: `ssh_nas()`
- L113: `ssh_nas()`

### sync-forgejo.sh (164 LOC)
- _(no top-level declarations matched)_

### twiceshy-watchdog.sh (86 LOC)
- L27: `ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=8 "$NAS" "$@"; }`
- L28: `now() { date +%s; }`
- L29: `health_probe() { curl -fsS -m 6 "$HEALTH_URL" >/dev/null 2>&1; }`
- L30: `alert()`
- L43: `healthz_wait()`
- L53: `main()`

### twiceshy-watchdog.test.sh (75 LOC)
- L11: `ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }`
- L12: `bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }`
- L13: `check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }`
- L14: `contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }`
- L27: `reset() { HEALTHY=0; START_HEALS=1; SSH_LOG=""; ALERTS=""; rm -f "$BREAKER_FILE"; }`
- L29: `health_probe() { return "$HEALTHY"; }`
- L30: `alert() { ALERTS="${ALERTS}|$1"; }`
- L31: `ssh_nas()`


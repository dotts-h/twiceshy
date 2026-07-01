# twiceshy — Codebase Map

> Where things live and how data moves through them. Navigational, not a
> tutorial — module purposes, entry points, the data-flow paths, and the
> architectural seams, anchored to `file:path` and the ADRs. The *why* is in
> [ARCHITECTURE.md](ARCHITECTURE.md); the per-symbol index is the generated
> [CODEMAP.md](CODEMAP.md); the vocabulary is [CONTEXT.md](CONTEXT.md). When
> this doc and the code disagree, the code wins — update this when a boundary
> moves.

Single Go module `github.com/dotts-h/twiceshy`, one deployable binary. The
markdown experience-record corpus is the source of truth; everything else is
derived or a thin edge over it ([ADR-0001](adr/ADR-0001-architecture.md)). That
corpus now lives in its own versioned data product (`twiceshy-corpus`,
[ADR-0021](adr/ADR-0021-decouple-corpus-as-a-data-product.md)); the engine repo
keeps only a small frozen fixture (`internal/testcorpus/`) for tests and loads a
real corpus through the `-corpus <dir>` seam.

## Module layout

### Binary

| Path | Purpose |
| --- | --- |
| `cmd/twiceshy/` | The binary. Thin `main` → `run(ctx, args, out, getenv)`; a `switch` over subcommands (see below) that delegate to `internal/`. `main.go` is the dispatch; `retro.go`, `screen.go`, `selfaudit.go` hold the smaller commands. |

### `internal/*` — pure core, then edges

| Package | Role (from the package doc comment) | ADR |
| --- | --- | --- |
| `internal/record/` | Parse/validate experience records (`Parse`, `LoadCorpus`, `Validate`, `Marshal`). YAML frontmatter + non-empty markdown body; owns the [SCHEMA.md](SCHEMA.md) format. Pure core. | §1 |
| `internal/fingerprint/` | Normative signature normalization + `sha256:` fingerprinting (`Normalize`/`Generic`/`App`). Same code runs at index time and query time, so fingerprint-exact retrieval is a hash lookup. Pure core. | §3 |
| `internal/index/` | Derived, always-rebuildable SQLite/FTS5 index over the corpus and the **embedding-free hot path**: fingerprint-exact → BM25 lexical, hard cap k≤3, relevance floor. `Open`/`Rebuild`/`Search`/`Get`/`Assess`; the push gate (`RetrievePush`/`RetrievePushTraced`); dense/RRF (`dense.go`) lives here but stays *off* the hot path. The only stateful package. | §1, §3–4, [9](adr/ADR-0009-dense-retrieval-is-pure-go-cosine.md) |
| `internal/ingest/` | Corpus-importer adapters + the dedup-at-ingest write-path core (`Prepare`: probe via `index.Assess`, screen, return a quarantined `Draft` or duplicate verdict). `Source` adapters: go/py deprecations, embedded + live OSV, and the live web watchers — endoflife.date (eol-live) + npm deprecation (npm-deprecation, #0073). `report.go` builds dispute counter-records. Pure core over the index seam. | [3](adr/ADR-0003-corpus-bootstrap-source-scope.md), [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) |
| `internal/screen/` | Ingestion safety gate: `Scan` candidate text for secrets / harmful-code / PII; masked findings, never echoes a raw secret. Pure detector — the caller owns the policy. Pure core. | — |
| `internal/server/` | MCP pull channel over streamable HTTP + the push channel (`POST /push`), behind one bearer-auth middleware chain. Six MCP tools; translates tool args to core calls. Edge. | §5–6 |
| `internal/spool/` | Intake queue for the deferred write-back channels — stores the *request* (not a built record) so ids are allocated at drain time. Payloads: `Report` (outcome), `Transcript` (retro), `Issue`. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) §E1, [18](adr/ADR-0018-session-retro-capture.md) |
| `internal/telemetry/` | Per-query gate-decision log for the retrieval channels (#0067): write-only, off the hot path, query hashed not stored, size-rotated. **Cannot** influence ranking. | §4 |
| `internal/pack/` | Builds distributable experience packs from validated records; `Classify` (fail-closed commercial-license eligibility) + `BuildManifest`. Pure core; file I/O is the `pack` command. | [2](adr/ADR-0002-licensing-strategy.md) §4 |
| `internal/doctor/` | Store-hygiene jobs — **delta-only, report/propose, never mutate** (git/PR is the trust boundary). `Doctor` seam + D2 `Staleness` over an `EOLSource` (endoflife.date). | [10](adr/ADR-0010-doctors-build-d2-defer-the-rest.md) |
| `internal/repro/` | Execution-validation harness: runs a record's repro test-set in an ephemeral gVisor (runsc) sandbox so a record is promoted *by execution, not trust*. `broker.go` is the only place untrusted code runs (hardcoded isolation); `revalidate.go` is the `Revalidator`. | [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) §3–4 |
| `internal/drafter/` | Turns a record's structured fact into a *candidate* repro the broker can prove. `Drafter` seam: deterministic Go-deprecation template + a cheap local-model drafter; both feed the same execution gate. | [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) §8 |
| `internal/judge/` | The keystone of the closed loop: a diverse frontier-model judge (different family from the drafter, never the local LLM) that checks what a green attestation can't — meaning, scope, license, poison. `Judge` seam; `ModelJudge`, `PanelJudge` (advisory), `MajorityJudge`, `TimingJudge`. Fails safe: no verdict = not-approved. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) §1, §6 |
| `internal/judgeeval/` | The judge-prompt eval: a labelled gold set + measured A/B of prompt/reasoning settings, scoring the false-approve direction. Replaces hand-tuned guessing. CI runs the deterministic scorer with a stub; the live A/B is endpoint-gated. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) #0028, [14](adr/ADR-0014-shared-result-aggregation-in-judgeeval.md) |
| `internal/promote/` | The decision packages of the closed loop. `promote.go` (`Promoter`): holding attestation + judge PASS flips `quarantined → validated` for the execution-provable class; a no-repro judge **panel** instead promotes the advisory class (`promoteAdvisory`, ADR-0016) and the prose class (`promoteProse`, ADR-0020), recording the audit trail (incl. panel verdicts) in `provenance.promotion`. `adapt.go` (`Adapter`): the demote/dispute direction. `journal.go`: per-run stop journal. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md), [16](adr/ADR-0016-advisory-class-panel-promotion.md), [20](adr/ADR-0020-prose-class-panel-promotion.md) |
| `internal/guard/` | Safety net the autonomous promote/demote loops consult: emergency stop, anomaly (rate) monitor, budget cap — bounding the residual risks (a compromised judge, a report-flood DoS). | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) §7 |
| `internal/notify/` | Guardrail alert seam: POSTs to ntfy when a guardrail trips, so an unattended halt is visible off the cron box. Env-gated; a failed post is logged, never returned. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) §B3 |
| `internal/lock/` | Single-flight `flock` on a corpus-local lockfile, so an overlapping cron tick can't double-write the mutating loop. Unix-only. | [13](adr/ADR-0013-closed-loop-autonomous-validation.md) §A2 |
| `internal/retro/` | Extracts reusable records from coding-agent session transcripts. `Analyzer` seam (the only model in the loop, drafts only); feeds candidates into the quarantine → PR ladder via `ingest.Prepare`. | [18](adr/ADR-0018-session-retro-capture.md) |
| `internal/selfaudit/` | Dogfoods twiceshy on its own `go.mod`: matches dependencies against ingested advisories and reports affected versions. | #0014 |
| `internal/eval/` | Retrieval-effectiveness eval (recall@k / near-miss rate over the real `search_experience` pull path) — the evidence gate for the store. Cheap, deterministic, no LLM. | §8, [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) §6 |
| `internal/similarity/` | Word-shingle (n-gram) overlap — `Shingles`/`Assess`. The optional ADR-0011 §5 net that flags authored prose running near-verbatim to a supplied reference. A lead for review, never an auto-reject. Pure core; stdlib only. | [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) §5 |
| `internal/author/` | `Scaffold` pre-stages a §5-clean authored record + repro skeleton(s) (authored-internal provenance pre-filled) — the file generation behind `twiceshy author`. Pure core; returns the files, caller owns the disk. | [11](adr/ADR-0011-corpus-growth-and-validation-engine.md) §5, #0091 |

## Entry points — `cmd/twiceshy <subcommand>`

Dispatch is the `switch args[0]` in `cmd/twiceshy/main.go` (~L197).

| Subcommand | What it does |
| --- | --- |
| `index` | Rebuild the SQLite/FTS5 index from the `experience/` corpus (`runIndex`). |
| `serve` | Run the MCP pull + push HTTP server (`runServe`). |
| `healthcheck` | Container HEALTHCHECK / external probe — GETs the health endpoint (`runHealthcheck`). |
| `ingest` | Import quarantined records from a license-clean source — `go`/`osv`/`py`/`osv-live` (#0007, `runIngest`). |
| `learned` | Capture one agent-authored lesson into the local corpus via `ingest.Prepare` (#0094, `runLearned`). |
| `draft` | Run the deterministic drafter pipeline: draft + broker-prove candidate repros over the corpus (`runDraft`). |
| `promote` | Positive direction of the closed loop (#0029): attestation + judge PASS auto-promotes quarantined records (`runPromote`). |
| `repromote` | Reversal/recovery (#0048): re-validate one stale or disputed record (`runRepromote`). |
| `adapt` | Negative direction (#0032): demote/dispute a quarantined record against counter-evidence (`runAdapt`). |
| `intake-reports` | Drain the report queue (§E1, #0042): each queued outcome → a quarantined dispute counter-record (`runIntakeReports`). |
| `intake-issues` | Drain the `report_issue` queue (#0066, #0075): materialize each spooled issue into `docs/issues/` via `scripts/new-issue.sh`, triage-flagged, dedup'd on title (`runIntakeIssues`). |
| `retro-intake` | Drain the session-retro queue (#0065): run the Analyzer per transcript, feed candidates into the ladder (`runRetroIntake`). |
| `screen` | Read text on stdin, run the ingestion content screen, print findings (`runScreen`). |
| `report` | Enqueue an outcome dispute into the report queue from the CLI (`runReport`). |
| `pack` | Build a distributable, license-clean experience pack (#0007, `runPack`). |
| `doctor` | Run a store-hygiene doctor (e.g. `staleness`) and print its proposed deltas (`runDoctor`). The `revalidate` execution doctor is `runRevalidate`. |
| `eval` | Run the retrieval-effectiveness eval over the corpus; `runEvalPush` covers push precision (`runEval`). |
| `usage-flush` | Materialize SQLite usage counters into each record's `provenance.usage` (`runUsageFlush`). |
| `gold-add` | Turn an audit-miss record into one `gold.yaml` judge-eval case (#0058, `runGoldAdd`). |
| `judge-eval` | Drive the diverse-model judge against the labelled gold set (#0028, `runJudgeEval`). |
| `self-audit` | Dogfood twiceshy on its own dependencies (#0014, `runSelfAudit`). |
| `similarity` | Flag an authored record's prose as near-verbatim to a supplied reference — the ADR-0011 §5 net (#0090, `runSimilarity`). Advisory lead, exits 0. |
| `author` | Pre-stage a §5-clean authored record + repro skeleton(s) under `-corpus`, refusing to overwrite (#0091, `runAuthor`). |
| `corpus-merge-check` | CI gate over `internal/mergecheck`: verify a corpus PR's base/head diff merges cleanly (`runCorpusMergeCheck`). |
| `corpus-pr-paths` | Companion to `corpus-merge-check`: print the changed-file paths a corpus PR touches (`runCorpusPRPaths`). |
| `nextid` | Print the next `exp-NNNN` id the corpus would allocate, honoring `-base` for merge-safe allocation (`runNextID`). |

## Primary data-flow paths

### 1. READ / pull (the hot path — embedding-free)

```
experience/**.md  →  record.LoadCorpus / record.Parse        (internal/record/record.go)
                  →  index.Rebuild → FTS5 + fingerprint rows  (internal/index/index.go:Rebuild, insertRecord)
   query          →  index.Search: fingerprintHits → lexicalHits, MaxK cap, relevance floor
                  →  MCP search_experience / get_experience    (internal/server/server.go: h.search, h.get)
```

`index.Search` (`internal/index/index.go`) is fingerprint-exact first, then BM25
lexical, hard-capped at `MaxK`=3 and floored — *empty is a valid answer*, never
padded with near-misses. `RetrieveFused` (`internal/index/dense.go`) adds
optional dense/RRF behind the `Embedder` seam but is **not** on the hot path.

### 2. PUSH (hook `additionalContext` — embedding-free)

```
prompt/query  →  POST /push                                   (internal/server/push.go: h.pushHTTP)
              →  index.RetrievePushTraced → eligibility + discriminative-term gate
                                                              (internal/index/index.go:454; discriminativeTokens L523)
              →  two-token corroboration (prompt trigger only) → PushResult card
                                                              (internal/index/index.go: corroborated; render.go: RenderPushContext)
              →  telemetry.Record (why served / not)          (internal/server/push.go: recordPushDecision)
```

The gate ([ADR-0015](adr/ADR-0015-push-discriminative-term-gate.md),
[ADR-0017](adr/ADR-0017-global-idf-push-gate-specificity.md), ADR-0028) injects
**nothing** unless the query carries a discriminative token — document frequency
computed over the **push-eligible subset** (validated, kind `trap`/`fix`,
non-importer `provenance.source.author` — #0107), against a stoplist. A
prompt-triggered query (`PushArgs.Trigger` != `"error"`) additionally needs TWO
DISTINCT discriminative tokens that co-occur on the SAME served record
(#0108) — a single rare token, or two tokens each living in a different record,
serve nothing. A deterministic stack match bypasses the whole gate
(`PushDecision.FingerprintBypass`, `internal/index/index.go:446`) — eligibility
and corroboration never apply to it. Quarantined records never reach this
channel.

### 3. WRITE paths

**Importer (batch):**
```
ingest.Source.Drafts  →  ingest.Prepare (dedup via index.Assess, screen)  →  quarantined record on disk
  (internal/ingest/{goadapter,osvadapter,osvlive}.go)   (internal/ingest/prepare.go)   (cmd: runIngest → writeRecord)
```

**Agent write-back (deferred via the spool):**
```
MCP record_experience  →  ingest.Prepare → quarantined record        (internal/server/record.go: h.record)
MCP report_outcome     →  spool.Enqueue(Report)                       (internal/server/report.go)
                          → intake-reports drain → ingest.BuildReport → quarantined dispute counter-record
MCP report_issue       →  spool.EnqueueIssue(Issue)                   (internal/server/issue.go)
MCP confirm_helpful    →  reinforcement signal (confirmed_helpful)    (internal/server/confirm.go) — never edits the record
SessionEnd hook        →  spool.EnqueueTranscript(Transcript)         (spool)
                          → retro-intake drain → retro.Analyzer → ingest.Prepare → quarantined record
```

Everything written is born `quarantined`; no code path writes `validated`
directly (ADR-0001 §6). The spool stores the **request**, so the `exp-NNNN` id is
allocated against the live corpus at drain time (no collisions across queued
entries).

### 4. VALIDATION loop (quarantined → validated, and back)

```
quarantined record
   ├─ execution-provable:  repro.Revalidator.RunWithAttestations  (gVisor broker, internal/repro/)
   │                       + judge.Judge PASS
   │                       → promote.Promoter.Promote → validated   (internal/promote/promote.go:153)
   ├─ advisory class:      judge.PanelJudge (diverse, no repro)     (internal/judge/panel.go)
   │                       → Promoter.promoteAdvisory               (internal/promote/promote.go:286)
   │                         [born-stale gate, ADR-0016 §7, #0071]
   ├─ prose class:         judge.PanelJudge (cross-family, no repro, Request.Prose) (internal/judge/panel.go)
   │                       → Promoter.promoteProse                  (internal/promote/promote.go:340)
   │                         [ADR-0020 — meaning/scope-only verdict, no executable proof]
   └─ stale / disputed:    doctor.Staleness or a report counter-record
                           → promote.Adapter (#0032) / Promoter.Repromote (#0048) → demote
```

`Promoter.Promote` (`internal/promote/promote.go:214`) records the attestation +
verdict in `provenance.promotion` as the git-committed audit trail; anything short
of (holding attestation AND judge approve) leaves the record quarantined. The
advisory path (`Promoter.promoteAdvisory`, `promote.go:286`,
[ADR-0016](adr/ADR-0016-advisory-class-panel-promotion.md)) skips repro and uses
the panel, gated by a born-stale check (`stalenessGate` field, `promote.go:45`,
wired via `WithStalenessGate`, `promote.go:82`, to `doctor.Staleness.WouldFlag`,
`staleness.go:123`) so an EOL/expired advisory is never promoted (ADR-0016 §7). The
prose path (`Promoter.promoteProse`, `promote.go:340`,
[ADR-0020](adr/ADR-0020-prose-class-panel-promotion.md)) likewise skips repro for a
no-source "prose" record, judged by a cross-family panel via `judge.Request{Prose:
true}` (`internal/judge/judge.go:129`); both panel paths record their member
verdicts in `provenance.promotion.panel` and are exempt from the validated-trap
guard requirement (`(*Record).panelPromoted`, `internal/record/record.go`).
`guard.*` bounds the whole loop (emergency stop, anomaly monitor, budget cap) and
`lock` single-flights it. Eligibility predicates: `Eligible` (`promote.go:144`) /
`EligibleAdvisory` (`promote.go:160`) / `EligibleProse` (`promote.go:178`) /
`Promotable` (`promote.go:194`) / `RepromoteEligible` (`promote.go:400`).

## Architectural seams

The injectable boundaries — stubbed in tests, no network in CI. The external
contracts are registered once in [CONTRACTS.md](CONTRACTS.md); this is the
`file:line` index.

| Seam | Defined at | Signature / role |
| --- | --- | --- |
| `ingest.Source` | `internal/ingest/source.go:11` | `Name() string`; `Drafts(ctx) ([]Draft, error)` — a license-clean knowledge source → quarantined Drafts. |
| `index.Embedder` | `internal/index/dense.go:34` | `Embed(ctx, text) ([]float32, error)` — pull-path dense retrieval only; never the hot/push path. |
| `judge.Judge` | `internal/judge/judge.go` | `Judge(ctx, req Request) (Verdict, error)` — the diverse-model gate; `Model`/`Panel`/`Majority`/`Timing` impls. |
| `promote.Attestor` | `internal/promote/promote.go:34` | `RunWithAttestations(ctx, recs) (doctor.Report, []repro.Attestation, error)` — satisfied by `repro.Revalidator`. |
| `repro.Broker` | `internal/repro/broker.go:55` | `Run(ctx, Job) (Result, error)`; `Healthy(ctx) error` — the gVisor sandbox; the only untrusted-code boundary. |
| `drafter.Drafter` | `internal/drafter/drafter.go:28` | `Name() string`; `Draft(ctx, root, rec) (path, error)` — fact → candidate repro for the gate. |
| `retro.Analyzer` | `internal/retro/analyzer.go:39` | `Analyze(ctx, transcript) ([]Candidate, error)` — transcript (untrusted DATA) → draft candidates. |
| `doctor.Doctor` | `internal/doctor/doctor.go:34` | `Name() string`; `Run(ctx, recs) (Report, error)` — delta-only, must not mutate. |
| `doctor.EOLSource` | `internal/doctor/doctor.go` | `Cycles(ctx, product) ([]Cycle, error)` — endoflife.date for D2 staleness; CI never calls it. |
| MCP tool boundary | `internal/server/server.go:129–134` | `search_experience`, `get_experience`, `record_experience`, `report_outcome`, `report_issue`, `confirm_helpful` — the external surface, all bearer-gated. |
| Spool queue boundary | `internal/spool/spool.go` | `Enqueue`/`EnqueueTranscript`/`EnqueueIssue` ↔ `List`/`Read*`/`Remove` — the async write-back deferral; payloads `Report`/`Transcript`/`Issue`. |

Dependency direction is acyclic and points inward: `cmd` → everything;
`server` → `index`/`record`/`ingest`/`spool`/`telemetry`; `ingest` →
`index`/`record`/`screen`; `promote` → `repro`/`judge`/`doctor`/`record`;
`record`/`fingerprint`/`screen` depend on nothing internal.

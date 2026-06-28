# `twiceshy learned` — going-forward capture command (#0094)

**Status:** approved design (2026-06-28). Implementation routed to Composer 2.5 (`ask-cursor`)
against the gate test `cmd/twiceshy/learned_test.go`; Claude orchestrates + reviews.

## Why

The corpus is ~99% imported security advisories and near-zero dev-stack craft traps (#0088).
`#0094` adds a **low-friction capture path** invoked by agents/humans at resolution time (and
heavily by upcoming authoring campaigns). It is the durable going-forward engine — beats hoping
a 20B model reconstructs a lesson from a session transcript. It is the CLI sibling of the MCP
`record_experience` tool (which serves remote agents with no corpus clone); `learned` serves
environments that *have* a local corpus clone (the brain, CI, authoring campaigns).

## Scope (one subcommand, zero new logic)

`twiceshy learned [flags]` — build one experience draft from flags, dedup + quarantine it via the
**existing** `ingest.Prepare` pipeline (identical to what `record_experience` and `runIngest` use),
then write it into the local corpus tree (or print it). **Reuse, do not duplicate:** the command
is the `runIngest` shape applied to a single flag-built draft.

### Handler

```go
func runLearned(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error
```

Dispatch: add `case "learned": return runLearned(ctx, args[1:], out, getenv)` to the `run()` switch
in `cmd/twiceshy/main.go` (and add `learned` to the usage strings).

### Flags → record fields

Reuse `addCommonFlags(fs)` for `-corpus` / `-db` / `-repo`. Then:

| Flag | Type | Maps to |
|------|------|---------|
| `-kind` | string, default `trap` | `Draft.Kind` (trap\|fix\|dead-end\|convention\|workflow) |
| `-title` | string, **required** (error if empty) | `Draft.Title` |
| `-summary` | string | `Symptom.Summary` |
| `-error` | **repeatable** (custom `stringList` flag.Value) | `Symptom.ErrorSignatures` (also the primary dedup signal) |
| `-root-cause` | string | `Resolution.RootCause` |
| `-fix` | string | `Resolution.Fix` |
| `-dead-end` | **repeatable** | `Resolution.DeadEnds` |
| `-verified-by` | string | `Guard.GuardingTest` |
| `-ecosystem` | string | `AppliesTo[0].Ecosystem` |
| `-package` | string | `AppliesTo[0].Package` |
| `-body` | string | `Draft.Body` (auto-composed if empty — see below) |
| `-author` | string, default `claude` | `Meta.Author` |
| `-session` | string | `Meta.Session` |
| `-stdout` | bool | print rendered markdown instead of writing |

Build the `Symptom`/`AppliesTo`/`Resolution`/`Guard` sub-structs **only when at least one of their
fields is non-empty** (mirror `internal/server/record.go` lines ~139–171). `SourceLicense` /
`SourceURL` stay `""` (agent-authored, ADR-0011 §5). Note `Guard.GuardingTest` and `Guard.Repro`
are `*string` — set `GuardingTest: &verifiedBy` only when `-verified-by` is non-empty.

### Behavior

1. Parse flags. If `-title` empty → return an error (nothing written).
2. `ix, _, err := buildIndex(ctx, c, false)`; `defer ix.Close()`.
3. `id, err := ingest.NextID(ctx, ix, c.corpus)`.
4. Build the `ingest.Draft` from flags. If `-body` is empty, **auto-compose** a minimal markdown
   body from the populated fields, e.g.:
   ```
   ## Symptom
   <summary, then each error signature as a fenced/quoted line>
   ## Root cause
   <root-cause>
   ## Fix
   <fix>
   ## Verified by
   <verified-by>
   ```
   (Omit a section whose field is empty. The body must be non-empty so the record renders.)
5. `outcome, err := ingest.Prepare(ctx, ix, c.repo, draft, ingest.Meta{ID: id, Author: author,
   Now: today, Session: session-if-set, IncludeQuarantined: true})`.
   **`IncludeQuarantined: true`** makes capture **idempotent** — like the importer, re-capturing a
   lesson that is already quarantined reports Known instead of piling up duplicate drafts. This
   matters because authoring campaigns capture in bulk; dedup is on lesson-identity (error-sig /
   summary / title), not on who/when (#0094 §43).
6. **Permissive capture bar (the agreed 2+3):** if `-root-cause` or `-fix` is empty, write a
   `warning:`-prefixed line to `out` noting the draft is likely to be *held* by the promote gate —
   **but still record it.** No hedge-word rejection (that is the promote gate's job, #0094 follow-up).
7. Outcome handling (identical to `runIngest`):
   - `outcome.Record == nil` (**Known**) → write nothing; report `learned: already covered by
     <id>` (use the strongest candidate id from `outcome.Candidates` if present) and return nil.
   - else (**Similar/Novel**) →
     - if `-stdout`: render via `record.Marshal(outcome.Record)` and write the markdown to `out`;
       **write no file.**
     - else: `writeRecord(c.corpus, outcome.Record)` and report `learned: wrote <Path> (<id>,
       <novelty>)`. Surface `outcome.Candidates` as `see also: <ids>` when Similar.

### Out of scope (YAGNI)

- No `-strict` full-evidence mode (add later if a campaign needs it).
- No commit/PR automation — a thin wrapper or the operator handles that, like `scheduled-import.sh`.
- The formal provenance **tag** (`git-history`/`authored`/`osv`/`retro-transcript`) for retrieval
  ranking is a separate #0094 follow-up; here `author` carries provenance.

## Gate (authored by the orchestrator, in `cmd/twiceshy/learned_test.go`)

Black-box via the top-level `run(ctx, []string{"learned", ...}, out, getenv)` so the dispatch case
is exercised too. Required passing cases:

- **writes a quarantined draft** with all sub-structs populated (parse the written file; assert
  `status: quarantined`, kind, error signatures, root-cause, fix, guarding test, ecosystem/package,
  author).
- **idempotent capture**: a second identical `-error` capture writes no second file (one record on
  disk) and reports `already covered`.
- **`-stdout`** prints the markdown and writes no file.
- **missing root-cause/fix** emits a `warning:` line but still writes the record.
- **auto-composed body** when `-body` omitted: written record `Body` contains the root-cause + fix.
- **`-title` required**: missing title errors and writes nothing.

Definition of done: `go test ./cmd/twiceshy/ -run Learned -race` green **and** `make lint` clean
**and** `make test` green. Capture is dogfooded — opening a `learned`-authored trap is the natural
first use once merged.

# CONVENTIONS

How this repo is built and kept healthy. Vocabulary:
[CONTEXT.md](CONTEXT.md). Architecture: [ADR-0001](adr/ADR-0001-architecture.md).

## Language & layout

- Go, single module `github.com/dotts-h/twiceshy`, one deployable service.
- Layout:
  - `cmd/twiceshy/` — the binary (thin `main`; logic lives in `internal/`).
  - `internal/` — all implementation packages; nothing exported as a library
    API yet.
  - `experience/` — the experience records (source of truth), one file per
    record: `experience/YYYY/NNNN-slug.md`.
  - `schema/` — versioned JSON Schema for record frontmatter.
  - `docs/` — CONTEXT, CONVENTIONS, SCHEMA, `adr/`, `research/`.
- Keep `main` thin and testable: `main()` calls a `run(args, env)` that
  returns an error.

## Dependency policy

Stdlib-first. The approved external dependency budget is:

1. a SQLite driver with FTS5 (CGO-free preferred, for NAS cross-builds),
2. an MCP / HTTP server library,
3. a YAML parser (record frontmatter is YAML by ADR-0001).

Anything beyond that list requires explicit owner approval **before** it is
added — open an issue or ask in the session. When in doubt, write the 50
lines yourself. (The research's prior art is explicit about transitive-dep
creep in single-purpose services.)

**Check a dependency against the locked build before planning around it.** The
release is **CGO-free** (`CGO_ENABLED=0`, pure-Go `modernc.org/sqlite`,
cross-compiled for linux/darwin × amd64/arm64). A storage/retrieval tech named
in a roadmap item (e.g. a vector store) must be verified against that invariant
*before* it's committed to — `sqlite-vec` was planned, then found to be a CGO
extension that breaks the build, and replaced with pure-Go cosine
([ADR-0009](adr/ADR-0009-dense-retrieval-is-pure-go-cosine.md)). Don't carry a
dep assumption from plan to implementation unchecked. (Retro 0001.)

## TDD & regressions

- **Test first.** New behavior starts with a failing test; the commit
  history should show specs preceding or accompanying implementation.
- **Every regression gets a guarding test.** A bug fix without a test that
  fails before the fix and passes after is not done.
- **Dogfood: every regression also gets an experience record.** When
  something in this repo bites us, the fix lands with a record under
  `experience/` in the [SCHEMA.md](SCHEMA.md) format, naming the guarding
  test. twiceshy is its own first corpus.
- Run race-enabled tests locally before pushing: `make test`.

## Decisions

- Every architectural decision gets an ADR: `docs/adr/ADR-NNNN-slug.md`,
  Nygard format (Status / Context / Decision / Consequences).
- ADR-0001's decisions are **locked** — do not relitigate them in reviews or
  refactors; supersede them with a new ADR if the world changes.

## Docs — one fact, one home

Every fact has exactly **one canonical home**; everywhere else **links** to it,
never copies it. A rule lives in CONVENTIONS, a decision in an ADR, the
vocabulary in CONTEXT, a fixed bug in REGRESSIONS, a promise in CONTRACTS, the
roadmap in NEXT_FEATURES. Duplicating a fact across docs guarantees they drift —
when something moves, update its one home and re-point the links. `CLAUDE.md` /
`AGENTS.md` stay thin pointers into this corpus, not second copies of it.

## CI & quality gates

- Single CI run per push: lint (`golangci-lint`) + `go test -race` +
  coverage floor. `make ci` reproduces it locally.
- The coverage floor lives in the Makefile (`COVER_FLOOR`); raising it is
  welcome, lowering it needs an ADR-grade reason in the commit message.
- CI must be green before merge. No `//nolint` without a trailing reason.
- **Process conformance:** this repo is under cookbook recipe management
  (`.recipes/lock.json`). `make doctor` runs the recipe-doctor aggregate; keep
  it green — process drift is caught by machinery, not memory.
- **Reading CI status (Forgejo):** poll `actions/tasks` filtered by `head_sha`
  for terminal per-run `success`/`failure` — the *combined* commit-status
  `.state` is unreliable (stale/partial), and a naive `group_by(.context)|.[-1]`
  reads the **oldest** status, not the latest. (Retro 0001.)
- **gitleaks scans the whole commit range, not just the working tree.** A
  follow-up "fix" commit does NOT clear a secret already in branch history —
  squash to a single clean commit. Corollary: **secret-shaped test data is
  assembled at run time** (e.g. `strings.Repeat`/split literals; a high-entropy
  value via `hex(sha256(seed))`), never a literal token in any commit — otherwise
  the scanner flags the very tests of a secret detector. (Retro 0001; REGRESSIONS.)

## Code style

- `gofmt` is law; idiomatic Go beats clever Go.
- Errors: wrap with `%w` and context (`fmt.Errorf("indexing %s: %w", path,
  err)`); define sentinel errors only where callers match on them.
- No package-level mutable state; dependencies enter through constructors.
- Context-first signatures for anything that does I/O.

## Security

- Bearer tokens come from the environment, are compared in constant time,
  and are never logged.
- Quarantined records never reach the push channel (ADR-0001 §6). Code
  paths that select records for injection must filter on
  `status: validated` explicitly, not by default-deny convention.
- Treat record content as untrusted input everywhere (memory poisoning is a
  studied attack class — research §6): escape it on render, never eval it,
  and never let a record influence which *other* records are retrieved.

## Commits & licensing

- Small, well-messaged steps. Prefix style: `docs:`, `feat:`, `fix:`,
  `test:`, `chore:`. One logical change per commit.
- Code is AGPL-3.0-only. External contributions require a signed CLA before
  merge — see [ADR-0002](adr/ADR-0002-licensing-strategy.md). Never link
  proprietary code into this module.

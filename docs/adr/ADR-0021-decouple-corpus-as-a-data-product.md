# ADR-0021: Decouple the corpus into a versioned data product, separate from the engine

- **Status:** Accepted (2026-06-22) — decider: **horia** (directed the split, on reliability
  + modularity + scalability grounds); claude proposed and authored. **The owed frontier
  gut-check (gemini + agy) is COMPLETE** (2026-06-22, issue #0077): both off-pool endpoints
  were unreachable at authoring time (ask-agy timed out at 8s, ask-gemini hung) but ran on
  the execution session. Both **confirm decision B** — the multi-tenant requirement (#0010)
  is the decisive factor neither model dislodged — and surfaced **four execution guards**,
  now folded into the migration plan below. See "Pre-flight gut-check (2026-06-22)".
- **Related:** [ADR-0001 §6](ADR-0001-architecture.md) (git is the trust boundary);
  [ADR-0005](ADR-0005-stable-seams.md) (the `-corpus` seam this leans on);
  [ADR-0011](ADR-0011-corpus-growth-and-validation-engine.md) (corpus growth as a live
  feed — the importer that churns); [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md)
  / [ADR-0016](ADR-0016-advisory-class-panel-promotion.md) /
  [ADR-0020](ADR-0020-prose-class-panel-promotion.md) (the autonomous promote/adapt loop
  that moves with the corpus); the **#0010 multi-tenant epic** (this is its precondition);
  the exp-0746 lesson (a data event must never silently freeze the loop).

## Context

The corpus (~2,400 `experience/` markdown records — **86% of the repo's tracked files**)
lives in the **same git repo** as the Go engine. A scheduled OSV importer commits ~150
new quarantined records to `main` **every ~43 min**; an autonomous loop (promote/adapt)
flips them `quarantined → validated` via CI-checked git PRs. Serving reads a NAS volume
**mirrored** from `origin/main:experience` (a read-replica, not the source of truth).

The engine is **already corpus-path-agnostic**: `serve`, `ingest`, `promote`, `draft`,
`eval` all take `-corpus <dir>` and read/write through `LoadCorpus(corpus)` /
`writeRecord(corpus, …)`. The coupling is therefore **operational, not architectural**:

1. the **default** corpus is the code-repo checkout (`-corpus .`);
2. **one shared, REQUIRED CI gate** runs the full Go suite over *both* code and data — so
   every data import runs lint+race-tests+govulncheck+gitleaks, and `block_on_outdated_branch`
   forces every code PR to rebase onto every import;
3. the **importer + autonomous loop + the corpus doctors** run *inside* the code repo and
   commit to it.

That shared failure domain has already bitten: a single quarantined import once tripped a
*whole-corpus* CI doctor and **silently froze the corpus for ~12h, unalerted** (exp-0746).
And the roadmap's **#0010 multi-tenant** epic is structurally impossible while the engine
owns the corpus: one engine cannot hold N tenants' corpora in its own source, and tenant
A's import must not red the gate that ships tenant B or the engine.

The stated goals are **reliability, modularity, scalability** — and the current shape
points the opposite way.

## Options considered

- **A — status quo.** Rejected: the shared failure domain (exp-0746) and the multi-tenant
  block remain; the code repo stays 86% data.
- **B — the corpus is a versioned data product in its own store, consumed by the engine
  via the `-corpus` seam + a schema contract (chosen).** Single corpus repo now;
  per-tenant corpus stores for multi-tenant. Preserves the git trust boundary (each corpus
  is git + CI); gives independent failure domains; slims the engine repo to a frozen test
  fixture.
- **C — corpus on a long-lived `corpus` branch in the same repo.** Rejected: still one CI
  config / one failure domain, fiddly two-trunk merges; the duck and our own analysis agree
  it is a dead-end.
- **D — keep one repo but fix only the CI/branch-protection coupling** (a required-check
  *shim* for data-only changes + relax `block_on_outdated_branch` + reduce import cadence).
  **Adopted as the INTERIM mitigation, not the endpoint** — it removes today's pain
  cheaply and reversibly, but does nothing for modularity or multi-tenant. Do it now; it is
  not throwaway (we want imports off the code CI regardless).
- **E — a non-git store on the NAS HDD.** Rejected: it destroys the trust boundary (git/CI
  is *what makes a validated record trustworthy and supersede-reversible*). The NAS is
  already a read-replica of git, and stays one.

## Decision

1. **The corpus becomes a standalone, versioned DATA PRODUCT.** It lives in its own git
   store (one `twiceshy-corpus` repo now; **per-tenant corpus stores** under multi-tenant,
   #0010). Each corpus keeps the **git trust boundary**: a record is `validated` only via a
   CI-checked, supersede-reversible commit — now in the *corpus* repo's CI.
2. **The engine consumes the corpus through the existing `-corpus` seam plus a versioned
   record-schema CONTRACT.** The schema (`schema/`, SCHEMA.md, `schema_version`) is the
   interface; a breaking change is a deliberate, coordinated version bump, never a silent
   break across the two repos. The engine declares the schema version(s) it supports, and
   **enforces them loudly**: `LoadCorpus` rejects an unsupported `schema_version` with a
   CRITICAL log + an alert metric (never a silent skip), and the corpus repo's CI rejects a
   record the *currently-deployed* engine can't parse before it reaches the NAS replica
   (gut-check guard 2).
3. **The autonomous loop moves to live WITH the corpus.** The importer, `promote`/`adapt`,
   the corpus doctors, and their scheduling + CI run against the corpus store — its own
   trust boundary, its own gate. The **engine repo's CI stops loading the live corpus**: it
   keeps a small *frozen fixture* for tests (which also de-flakes the gold-set golden tests
   that currently load the live corpus).
4. **Serving is unchanged in shape:** `serve` already reads a corpus path; the NAS sync
   re-points from the engine repo to the corpus store. Git stays the source of truth; the
   NAS stays a read-replica.
5. **Interim, do D now** (the required-check shim + relax `block_on_outdated_branch` +
   slower import cadence) to stop the bleeding while B is executed.
6. **exp-0746 guardrails are part of "done," not a follow-up:** corpus-side doctors stay
   scoped to the served (`validated`) subset; **no auto-merge result is ever swallowed** —
   a left-open red/stalled PR alerts, so a data event can never silently freeze the loop.

## Pre-flight gut-check (2026-06-22)

The owed frontier consult ran on the execution session (issue #0077); the brief was the
problem, the chosen option B, the rejected options, and the full migration plan.
**Both gemini and agy confirm decision B** — each independently probed for a simpler option
and neither dislodged the split, because the **multi-tenant requirement (#0010) is decisive**:
N tenants = N corpus stores served by one engine, which an in-repo branch cannot give.
agy's orphan-`corpus`-branch variant (a sharper take on rejected option C) and gemini's
"validate the trust boundary in-repo first" both reduce to **option D as the interim
proving step** — already adopted. The decision stands; what changed is the *plan got four
guards* it under-weighted:

1. **Quiesce needs a HARD write-lock, not just "stop timers + drain."** Both flagged that a
   stray local branch or a hung cron can push to `experience/` *after* "drained," forking the
   corpus. → Phase 3 installs a reject-all guard on `experience/` (branch protection / a
   pre-receive or pre-commit hook) so the old location is provably unwritable, and the
   **authoritative** lossless snapshot is taken as the *last* action under that lock — the
   phase-0 tag is only a baseline (see Phase 0/3).
2. **The schema contract must FAIL LOUD and be checked against the *deployed* engine** — a
   `schema_version` *declaration* is not enforcement. → The engine's `LoadCorpus` rejects an
   unsupported version with a CRITICAL log + an alert metric (never silently skips); the
   corpus repo's CI rejects a record the *currently-deployed* engine can't parse *before* it
   reaches the NAS replica (via the engine's supported-version set, not a static file).
   Folded into Decision §2 and issue #0079.
3. **CI dependency inversion** (agy): the importer/doctors run the engine's Go code, so corpus
   CI needs engine binaries while engine CI needs frozen corpus data — a cycle. → The engine
   repo publishes a **pinned, versioned CLI artifact**; the corpus repo's CI downloads and
   runs *that* rather than building the engine from source. Folded into Phase 2 and issue #0080.
4. **id-allocation must start at `max(exp-NNNN) + gap`, verified pre-restart** — a `max+1`
   allocator collides with any straggler PR ported over after cutover. → Phase 6 initialises
   the new allocator with an intentional gap (e.g. `+1000`) and a test asserts no collision
   before timers are re-enabled. Folded into issue #0082/#0083.

## Migration plan (STOP → MOVE → RESTART), reversible at each phase

Execution is a **dedicated session**. Each phase is independently revertable; do not start
the next until the current one is verified.

0. **Pre-flight:** run the gemini + agy gut-check (done — see "Pre-flight gut-check"). Tag
   `origin/main` as a **baseline** snapshot (`corpus-snapshot-pre-decouple-<date>`) — a
   recovery point, **not** the authoritative cutover snapshot. Because imports keep landing,
   the *authoritative* lossless snapshot is re-taken at quiesce under the write-lock (Phase 3),
   and the Phase-4 byte-match is against *that* drained SHA, not this baseline.
1. **Contract first (engine repo, no corpus move yet):** pin the engine's supported
   `schema_version`; replace live-corpus CI loads with the frozen fixture; add the
   required-check **shim** + relax `block_on_outdated_branch` (this is D, and it is the
   safe first step). Reversible: pure code/CI config.
2. **Stand up the corpus store (parallel, not yet authoritative):** create `twiceshy-corpus`,
   import the snapshot, stand up its CI (schema-validate + validated-scoped doctors) and
   the exp-0746 stall alarm. The corpus CI runs the engine's **pinned, versioned CLI
   artifact** (published by the engine repo), not a build-from-source — this breaks the
   dependency inversion the gut-check flagged (corpus CI needs engine code, engine CI needs
   corpus data) (gut-check guard 3). Nothing reads it yet. Reversible: delete the repo.
3. **QUIESCE the live pipeline:** pause the importer + the promote/adapt timers on the
   brain (`systemctl stop …`), and **drain in-flight import/validate PRs** to a clean point
   (no half-open auto-merge). Then install a **hard write-lock on `experience/`** — branch
   protection + a pre-receive/pre-commit reject-all so a stray local branch or hung cron
   cannot push after the drain (gut-check guard 1) — and only then take the **authoritative**
   lossless snapshot as the last action on the source. Confirm the engine-repo `experience/`
   is at a known SHA. Reversible: lift the lock + re-enable the timers.
4. **MOVE / cut over:** make the corpus store authoritative — sync its content from the
   drained engine-repo SHA (must byte-match the snapshot tag); re-point the NAS sync at the
   corpus store; re-point the importer + promote/adapt at `-corpus <corpus-store>`. The
   engine repo's `experience/` is now frozen (or removed, leaving the fixture). Reversible:
   the snapshot tag + the still-present engine-repo history.
5. **RESTART on the new home:** re-enable the importer + loop timers against the corpus
   store; verify a full cycle (import → quarantined PR → validate → served) end-to-end, and
   that serving still answers from the re-pointed NAS replica.
6. **Verify + decommission:** confirm id-allocation is correct in the new store — initialise
   the allocator at **`max(exp-NNNN) + gap`** (e.g. `+1000`), not `max+1`, so a straggler PR
   ported over after cutover cannot collide, and assert no collision by test *before* timers
   are re-enabled (gut-check guard 4) — the gold/eval suites pass against the fixture, and the
   stall alarm fires on a synthetic red. Only then retire the engine-repo corpus path.

## Consequences

- **Reliability:** independent failure domains — a bad import reds the *corpus* gate, not
  the engine's, and vice-versa; the exp-0746 silent-freeze class is closed by the
  validated-scoped doctors + the stall alarm.
- **Modularity:** the engine (search/serve/validate machinery) and the corpus (a
  distributable data product — packs, the "feed experience to agents" thesis) become two
  products with a versioned contract between them.
- **Scalability / multi-tenant:** the engine already loads an arbitrary `-corpus`, so N
  tenants = N corpus stores served by one engine; this ADR is the precondition that makes
  #0010 buildable.
- **Costs / risks:** cross-repo coordination becomes a NEW surface (schema-version drift,
  id-allocation across the cutover, the NAS sync re-point) — mitigated by the contract,
  the snapshot tag, the quiesce-before-move discipline, and the reversibility of each
  phase. A sloppy split would trade one coupling for coordination bugs, so the contract +
  each side's independent-but-**alerted** CI are part of "done."
- **Engine repo slims** from 86% data to code + a fixture; CI no longer runs on imports.

## What this does NOT do

Re-architect the engine (the `-corpus` seam already exists), change retrieval/the push
gate, or change *what makes a record validated* (still a CI-checked git commit, now in the
corpus store). Per-tenant isolation/auth/anti-abuse is #0010's own scope, downstream of
this.

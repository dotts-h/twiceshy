# ADR-0027: Runner-local operational run-state is not corpus data

- **Status:** Accepted (2026-06-26) — claude drafted + shipped the corpus-side fix
  (twiceshy-corpus PR #40); engine guard follows.
- **Related:** [ADR-0021](ADR-0021-decouple-corpus-as-a-data-product.md) (the corpus is a
  versioned data product — this draws the data/operational-state line ADR-0021 implied but
  left in `runs/`); [ADR-0022](ADR-0022-promote-throughput-and-hold-cooldown.md) (the hold
  ledger); [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) §2 (veto-window PR);
  #0054 (run journal / resume cursor); #0095 (this fix).

## Context

The nightly promote/adapt driver writes three **fixed-path** files under `<corpus>/runs/`
and commits them into each `validate/run-*` PR:

- `promote.journal.json`, `adapt.journal.json` — per-run resume cursors (#0054), rewritten
  wholesale every run.
- `promote.holds.json` — the promote hold-cooldown ledger (ADR-0022), a `map[id]→heldAt`.

Because they sit at fixed paths overwritten every run, **any two open validate PRs always
collide on them.** That is exactly what froze the corpus 2026-06-23→26: once one validate PR
sat unmerged (the merger had been disabled), every later run conflicted on the journal and
could not merge — a single stuck PR cascaded into a total freeze, invisible because the
stall alarm was also muted (#0093).

The per-run audit files (`run-<id>-promote.json`, `run-<id>-adapt.json`) do **not** have this
problem — they are uniquely named per run and never conflict.

The code already calls these files "operational state on the -corpus path, NOT an experience
record" (cmd/twiceshy/holds.go) — they are not part of the served data product, and both are
**fail-open** (a missing/corrupt journal or ledger costs at most one extra judging pass,
never a stuck pipeline).

## Decision drivers

- A single stuck PR must never be able to block subsequent runs (no cross-run cascade).
- The cooldown ledger must still persist across runs (it is the ADR-0022 $-guard).
- Lowest risk: the validate loop had only just been unstuck.

## Options

- **O1 — Untrack the fixed-path operational files; gitignore them.** They keep being written
  to `<corpus>/runs/` but are no longer committed. As ignored files they survive the
  driver's `git clean -fd` + `reset --hard` (verified empirically), so the ledger persists in
  the runner's clone. *Pro:* two-line change, zero engine risk, immediate. *Con:* relies on
  git-clean-skips-ignored semantics; ledger is now runner-local (lost on a clone rebuild —
  fail-open, acceptable).
- **O2 — Move the files to a `-state-dir` outside the corpus tree (engine change).** Make the
  operational/data boundary structural and unit-testable. *Pro:* doesn't depend on git
  mechanics; architecturally clean. *Con:* signature refactor (`JournalPath`, ledger path) +
  flag wiring + driver env; larger blast radius.
- **O3 — Custom merge driver / `merge=union` on the JSON.** *Con:* JSON objects don't union
  into valid JSON; brittle.

## Decision

**O1 now, O2 as the durable follow-up.** Ship the untrack+gitignore fix to the corpus repo
immediately (the reliability win), and track the structural `-state-dir` move (O2) as the
engine-side hardening so the boundary stops depending on `.gitignore` + git-clean semantics.

## Consequences

- Validate PRs now differ only in distinct experience records + their own per-run audit
  files → they merge back-to-back; one stuck PR cannot poison the next.
- The cooldown ledger is runner-local; a clone rebuild resets it (fail-open — records judged
  one extra time). The 223-entry ledger was preserved across this cutover.
- **Open (O2):** add `-state-dir` to the run command + a unit test asserting journal/ledger
  paths resolve outside the corpus record tree, so a future change can't silently re-commit
  them. Tracked as #0096.

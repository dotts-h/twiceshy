# DEV_LOOP.md — the twiceshy unit-of-work ritual

> The deterministic session loop: every unit of work follows the same path from
> "what's next" to "merged green". The scripts encode the failure-prone steps so
> they can't be hand-rolled wrong. Picking an item means *building* it — the loop
> doesn't stop between steps to ask, except at the genuine ambiguities listed below.

## The loop (copy this checklist and tick it off)

- [ ] **1. Pick the next item.** Run `scripts/next-issue.sh`. It reads
      `docs/issues/`, reconciles epic-vs-child status, follows `depends_on` edges
      (it never recommends a blocked item), and prints a ranked recommendation:
      **[1] BUILD** an unblocked child · **[2] BREAK DOWN** a childless epic
      (file child #1 via `scripts/new-issue.sh`) · **[3] STALE** epic flag ·
      **[4] nothing open**.
- [ ] **2. Read the item's charter** (`docs/issues/<id>-*.md`): the what, the
      seams it touches, the why-now, any linked decision. A needed decision is
      recorded *first*, on the same branch.
- [ ] **3. Start fresh from a trustworthy base.** Run
      `scripts/start-fresh.sh <branch> [--expect-sha <sha>] [--require <file>,<file>]`.
      It fetches, resolves `refs/remotes/origin/main` explicitly,
      asserts any expected SHA/foundation files, and cuts the branch. If it exits
      non-zero, **stop** — the base is wrong; never branch over it. Branch names:
      scope-prefixed kebab (`feat/…`, `fix/…`, `docs/…`); one item per branch.
- [ ] **4. Confirm a green base**: `make lint` and `make test` once
      before writing code.
- [ ] **5. Build test-first.** Failing test, then the smallest code to pass it.
      Fold the item's supporting docs (decision record, regression row, glossary
      term, issue close-out) into the **same** branch.
- [ ] **6. Gates before pushing**: `make lint` and `make test`,
      plus a self-review of the diff sized to the diff.
- [ ] **7. Open the PR** against `main`, titled from the item
      (reference the issue id). Record the PR number on the issue **on the same
      branch before merging** — a follow-up "record PR #N" PR doubles CI for
      bookkeeping git already encodes.
- [ ] **8. Merge when CI is green.** A red or pending check is never merged
      around: diagnose, fix on the branch, re-push, re-wait. Confirm
      `origin/main` advanced, then delete the branch.

## When the pick is ambiguous (ask, don't guess)

- **Multiple open epics** — which one this session advances is a product call.
  Surface candidates and ask. Harness metadata (auto-generated session branch
  names/titles) never breaks the tie — it's plumbing, not product intent.
- **[3] STALE** — an epic is open but every child is closed: either close it or
  it has un-filed follow-ups. Don't silently close or invent work.
- **[4] nothing open** — don't fabricate an item; propose a roadmap pass instead.

## Parallel lanes (optional, advanced)

Two items may run as concurrent lanes only if both are unblocked **and** touch
disjoint seams (compare their "Touches"/Summary lines). Reserve shared monotonic
ids up front, then one worktree + branch + PR per lane. The `depends_on` graph is
what makes this safe — only items with no edge between them belong in one batch.

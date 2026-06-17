---
name: get-next
description: Does the next unit of work end-to-end — verifies the base and branches fresh from a trustworthy main, picks the next roadmap item (the next open child of an open epic, or the next epic to break down) from docs/issues/, builds it test-first, then opens the PR, code-reviews it (applying fixes), and merges once CI is green. Use at the start of a session when asked to "get the next item", "start fresh", "pick up the next issue/feature", "what's next", or to begin the next epic child.
allowed-tools: Read, Bash, Grep, Glob, Edit, Write
---

# Get next (start fresh, pick the next item, drive it to merged)

The cost of getting this wrong is silent and expensive: work cut from a **stale
`origin/main`** (or from a **`main` *tag*** that shadows the branch)
lands on the wrong base, and a half-finished branch reused for a new item smuggles unrelated
changes into a PR. This skill makes the session-start ritual **deterministic**: verify the base,
branch fresh, pick the next item from the single source of truth — then **carry the item all the
way to a merged PR**. Invoking `/get-next` means "do the next unit of work", not "tell me what it
would be": after setup, continue straight into the build (test-first), then PR → code review
(+fixes) → CI-green → merge, without stopping to ask between steps. Stop mid-ritual only for the
genuine ambiguities listed below. It does **not** file or restructure issues (that's
`tracking-issues`) — it *selects*, *sets up*, *builds, and lands*.

## Workflow (copy this checklist into your reply and tick it off)

> **The loop's scripts live in the target repo's `scripts/` directory** (installed by the `loop`
> recipe): run `scripts/next-issue.sh` and `scripts/start-fresh.sh` from the repo root. A
> `No such file` (127) means the `loop` recipe isn't installed — run the doctor
> (`recipes/loop/scripts/doctor.sh <repo>`) — do **not** fall back to hand-rolling the ritual (it
> skips `start-fresh.sh`'s codified base assertions). The full playbook is `docs/DEV_LOOP.md`.

- [ ] **1. Pick the next item.** Run `scripts/next-issue.sh`. It reads `docs/issues/`, reconciles
      epic-vs-child status, follows `depends_on` edges (it will **not** recommend a blocked item),
      and prints a ranked recommendation:
  - **[1] BUILD `<id>`** — an open, **unblocked** child of an open epic → this is your item.
  - **[2] BREAK DOWN epic `<id>`** — an open epic with no children yet → file child #1 with
        `tracking-issues` (`scripts/new-issue.sh "<first slice>" --group <id> [--depends <id>]`),
        then build it.
  - **Blocked** items are listed with their open blocker (finish that first); **Parallelizable now**
        lists the unblocked set.
  - **[3] STALE flag** / **[4] nothing open** → see "When the pick is ambiguous" below.
- [ ] **2. Read the item's charter.** Open `docs/issues/<id>-*.md`: the *what*, the seam/files it
      touches, the "why now", and any linked decision record. If it needs a decision, that record is
      written **first**, on the same branch — note it now.
- [ ] **3. Choose the branch name.** Scope-prefixed kebab per CONVENTIONS: `feat/…`, `fix/…`,
      `docs/…` (e.g. `feat/billing-cache-write-pricing`). One item per branch.
- [ ] **4. Start fresh from main.** Run
      `scripts/start-fresh.sh <branch> [--expect-sha <sha>] [--require <file>,<file>]`.
      It fetches, fast-forwards `main`, prints the resolved SHA, and cuts the branch
      from `refs/remotes/origin/main` (never the bare `main`, which a tag
      can shadow). **Pass `--require` a file the item builds on** (a seam it touches) so a wrong base
      fails loud, not silent. If it exits non-zero, **stop**: the base is wrong; do not branch over
      it. — CONVENTIONS "Verify the base before branching".
- [ ] **5. Set up the build.** Confirm a green base: run `make lint` and `make test`
      once.
- [ ] **6. Build the item** test-first: failing test first, then the implementation, with the item's
      decision record / contracts / issue close-out folded into the **same** branch. **Do not stop
      after step 5 to ask whether to build** — selecting the item WAS the instruction to build it.
      Gates before pushing: `make lint` and `make test` (plus the e2e gate when the UI
      changed).
- [ ] **7. Open the PR.** Push with `git push -u origin <branch>` and open a PR against
      `main` titled from the item (reference the issue id + epic/slice label). The PR
      body carries the what/how and the item's acceptance boxes, ticked. **On forgejo: if `gh`
      / `tea` isn't available in the environment, open the PR with the forge's MCP/API tool and read
      checks + merge through the same path.**
- [ ] **8. Code-review the PR (+ apply fixes).** Run `/code-review --fix` on the diff. Apply
      confirmed correctness/reuse/simplification findings, note refuted/deferred ones in the review
      summary, re-run the gates, and push the fix commit to the same branch.
- [ ] **9. Merge when CI is green.** Wait for ALL checks on the PR head (lint, test, e2e) — CI green
      is a hard precondition (CONVENTIONS). If you can't poll with a CLI, re-check the run status
      after a *background* sleep timer (`run_in_background: true` — foreground `sleep` is blocked) and
      loop until every run is `completed`/`success`; don't busy-wait. Then merge the PR (merge commit)
      and confirm `origin/main` advanced. If a check fails: diagnose, fix on the branch,
      re-push, re-wait — a red check is never merged around.

## When the pick is ambiguous

The recommender flags, it doesn't decide. **Ask the user** (don't guess) when:

- **Multiple open epics** — which pillar this session advances is a product call. Surface the
  candidates + their "why now" and let them choose. **Harness metadata never breaks this tie**: an
  auto-generated session branch name or session title is plumbing, not product intent. Ask.
- **[3] STALE** — an epic is `open` but every child is `closed`. Either it's done (close it via
  `tracking-issues`) or it has un-filed follow-ups. Don't silently close or invent work.
- **[4] nothing open** — the roadmap is exhausted. Don't fabricate an item: a research pass
  (re-read the code + tech-debt register + the differentiators, propose the next roadmap) is the
  next move — say so and offer to run it.

## Harness/session branches are plumbing

A remote harness (an Action, a web session) may designate an auto-generated session branch and a
session title. **Neither carries product intent**: the item comes from `docs/issues/` via
`next-issue.sh` — never infer the item from the session branch name.

- **Soft case (default): pick your own branch.** When the harness only *suggests* a branch (or
  none), use the scope-prefixed feature branch from step 3/4.
- **Hard case: the harness mandates a push target.** Then **branch *is* the mandate** — don't invent
  a `feat/` name. Cut **that branch itself fresh from `origin/main`** so the base is
  still trustworthy: `start-fresh.sh` *refuses an existing branch*, so when the harness pre-created
  it, first confirm it carries **no unique work**
  (`git log origin/main..X` empty), delete it (`git branch -D X`), then
  `start-fresh.sh X --require <seam>`. The item still comes from the picker; only the branch name is
  the harness's.

## Boundaries (what this skill does NOT do)

- **Doesn't file/restructure issues.** Creating, grouping, or closing issues is `tracking-issues`;
  this skill only *reads* the store to pick and *delegates* a needed file to it.
- **Doesn't write decision records itself.** A decision the item needs is recorded first, on the
  same branch.
- **Doesn't cut releases.** Tagging/publishing is `cut-release`, run separately when asked.

## This repo (facts the scripts rely on)

- Source of truth: `docs/issues/INDEX.md` (epics + children). Hard ordering is the issue frontmatter
  `depends_on: [ids]` (blocked-by edges), filed by `tracking-issues` (`new-issue.sh --depends`). The
  picker reads these to skip blocked items and compute the parallelizable set.
- A **tag named `main`** can collide with the branch, and a sandbox can serve a
  **stale `origin/main`** on first fetch — both silently mis-base work.
  `start-fresh.sh` always resolves `refs/remotes/origin/main` and asserts
  SHA/foundation for that reason.
- Branch from `main`, never commit to it; one item per branch; fold supporting docs
  into the **same** feature branch.

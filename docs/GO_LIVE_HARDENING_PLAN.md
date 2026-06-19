# Go-live hardening plan — bulletproof the autonomous loop (single-tenant)

> **Purpose.** A self-contained execution backlog to take twiceshy's autonomous
> promote/demote loop from "works when run by hand" to "runs unattended nightly,
> is fully observable, and every action is reversible" — *before* any public /
> dashboard / multi-tenant work. A fresh session should be able to execute this
> top-to-bottom without prior conversation context.
>
> **Scope.** Single-tenant (the brain). Public presentation, the dashboard,
> per-agent auth, and multi-tenant isolation are **explicitly deferred** (see
> "Deferred", bottom). Grounded in a 5-agent code audit (2026-06-19) of the
> observability, rollback, and loop-robustness surfaces.

## 0. Where things stand (read first)

- **Read path (consume):** done — `search_experience` / `get_experience` over MCP,
  structured `slog` JSON logging, relevance floor + k≤3. This is the product.
- **Write path (contribute):** propose-only. `record_experience` and
  `report_outcome` return a quarantined draft / counter-record to the caller;
  nothing reaches consumers without the gate + judge (or a human PR).
- **Judge (calibrated):** `internal/judge` + the off-pool gpt-oss:20b shim. The
  prompt was just measured (`internal/judgeeval`): **prose @ think=false wins
  (0 false-approve / 0 false-reject at repeat=5)**; `think=true` is harmful. The
  winner is pinned into `promote`/`adapt`. **Prerequisite: merge PR #89.**
  Residual: gpt-oss:20b is non-deterministic at temp 0 on license-boundary cases
  (~1/7 false-approve on the AGPL case under prose, single-shot — see exp-0046).
- **Guardrails (#0033):** emergency stop (`TWICESHY_PAUSE`), anomaly monitor,
  budget caps — **all detect but only print to stdout.**
- **Importer:** real records arrive nightly (`twiceshy-import.timer`); **39 real
  quarantined records** sit ready as loop fuel.

### The keystone finding (why this plan exists)

The autonomous loop is **not yet autonomous, observable, or reversible**, and
**ADR-0013 §2's PR/soak/veto window — the headline human-oversight safety net — is
not implemented in code.** Concretely:

1. **The morning review is blind.** `promote`/`adapt` mutate the working tree
   directly and exit; nothing commits per-run, there is no run manifest, and
   stdout is ephemeral. "What happened overnight" is reconstructable only by an
   operator who captured stdout live *and* runs `git diff` on the uncommitted tree.
2. **The loop doesn't run itself.** No timer runs `promote`/`adapt`; the import
   timer only imports. And `report_outcome` returns markdown to the caller and
   never lands in the corpus, so `adapt` has no nightly input without a human
   paste-commit-merge.
3. **The anomaly fires too late.** Promotions are persisted *before* the anomaly
   check; the run then exits 0. A compromised judge approving everything writes
   the bad records to disk, prints "ANOMALY:" to a log nobody reads, and succeeds.
4. **No veto window / no rollback boundary.** No promotion PR, no cooldown, no
   per-run commit — so a night's changes aren't batched into one revertible unit,
   and a mid-corpus abort leaves an unmarked partial working tree.

The MVP workstream below closes exactly these four.

---

## Workstreams

Each item: **ID · priority · size · what · why · touches · acceptance.**
Priority — **mvp** (needed before a first instrumented overnight run), **hardening**
(before trusting it unattended), **deferred** (pre-public).

### A — Nightly driver + the ADR-0013 §2 veto window (keystone)

- **A1 · mvp · M — Nightly validate driver + veto-window PR.**
  New `scripts/scheduled-validate.sh` (sibling of `scheduled-import.sh`): dedicated
  clone, build, run `import → promote → adapt` on a dedicated branch, **batch the
  whole night into ONE commit, open ONE PR** (this *is* the held queue / veto
  window), auto-merge on green **after a soak cooldown** (config, ~the ADR's 48h),
  ntfy on PR-open and on anomaly. systemd timer (`twiceshy-validate.timer`),
  ordered after the import timer.
  *Why:* realizes §2 (held queue + cooldown + close-to-veto), the per-run commit
  (rollback boundary), and the scheduling — all four keystone gaps at once.
  *Touches:* new `scripts/scheduled-validate.sh`, new systemd unit+timer; reuse the
  forgejo-ci-merge + ntfy plumbing from `scheduled-import.sh`.
  *Acceptance:* a dry weeknight run opens a single PR titled with the run id;
  CI-green PRs self-merge only after the cooldown; closing the PR vetoes the batch;
  `TWICESHY_PAUSE=1` short-circuits the driver before any mutation.

- **A2 · mvp · S — Single-flight lock.** `flock` on a corpus-local lockfile around
  `promote`/`adapt` (or in the driver) so an overlapping cron tick or a manual run
  during the timer cannot both load the corpus and double-write.
  *Touches:* `cmd/twiceshy/main.go` (runPromote/runAdapt) or the driver; a small lock helper.
  *Acceptance:* a second invocation while one holds the lock exits non-zero with a
  clear "another run in progress" message; a test covers it.

- **A3 · mvp · S — Preflight healthcheck.** Before walking the corpus, probe
  `docker info` (+ runsc runtime present) and a judge liveness ping; abort cleanly
  (distinct exit + log) if either is down, rather than discovering it mid-run.
  *Touches:* `cmd/twiceshy/main.go` setup; a `Ping`/`Healthy` seam in `internal/repro` and `internal/judge`.
  *Acceptance:* with docker stopped or judge URL unreachable, the command exits
  before processing any record with a named preflight error.

### B — Observability (make a run legible)

- **B1 · mvp · M — Structured loop logging.** Add an `slog` logger to
  `promote`/`adapt` emitting one JSON event per record decision —
  `{run_id, stage, record_id, outcome (promoted|held|demoted|disputed|ineligible|orphan), reason, judge_model, judge_decision, reproduced_under, attestation_ran_at, duration_ms}`
  — *alongside* the existing human prose. Emit a per-run summary event
  (`started_at/finished_at/duration_ms/counts`). Reuse `internal/server`'s
  `slog.NewJSONHandler` pattern. **Every outcome must log**, including `held`
  (today the adapt `held` branch emits nothing).
  *Touches:* `cmd/twiceshy/main.go` (runPromote/runAdapt/promoteCorpus/adaptCorpus).
  *Acceptance:* a run produces a parseable JSON line per record + one summary line;
  held/ineligible/error outcomes all appear.

- **B2 · mvp · M — `-json` run manifest.** Add a machine-readable `-json` outcome to
  `promote`/`adapt` (and `ingest`): the stats struct + per-record actions
  (`id, from_status, to_status, judge_model, judge_decision, reproduced_under, reason`).
  The driver (A1) writes it to a committed `run-<id>.json`. **This is the artifact
  the morning review and the daily Opus audit read.**
  *Touches:* `cmd/twiceshy/main.go` runPromote/runAdapt; small struct in `internal/promote` or new pkg.
  *Acceptance:* `promote -json` emits valid JSON listing every record's transition;
  the daily audit (F2) consumes it without scraping stdout.

- **B3 · mvp · S — Route guardrail trips to a channel.** A small `internal/notify`
  seam (http POST, env-gated `TWICESHY_ALERT_URL` → ntfy). Fire on anomaly,
  emergency-stop-engaged, and budget-cap-reached, at `slog` Warn with explicit
  event names. The brain already runs ntfy.
  *Touches:* new `internal/notify`; `cmd/twiceshy/main.go` guardrail sites; optionally `internal/guard`.
  *Acceptance:* an anomalous run posts to the ntfy topic; unset `TWICESHY_ALERT_URL` is a silent no-op.

- **B4 · hardening · S — Success heartbeat.** On clean completion, POST to a
  configurable Uptime-Kuma push URL (`TWICESHY_HEARTBEAT_URL`, env-gated). So a
  silently-skipped or misconfigured nightly run (no run = no diff = looks like a
  quiet night) is *detectable*.
  *Touches:* end of runPromote/runAdapt; reuse the B3 notify seam.

- **B5 · hardening · S — One log stream.** Convert `internal/repro/broker.go`
  reaper `log.Printf` calls to the project `slog` logger (structured fields). Kills
  the third logging style so a nightly operator has one format to grep/ship.

- **B6 · hardening · M — Judge latency + verdict distribution.** Time the judge
  call (`time.Since` around it) and aggregate approve/reject/held *ratios* per run
  into the B2 summary. Catches a degrading/hung judge **and** the subtler
  "judge approves a higher *fraction*" compromise the raw-count anomaly monitor misses.
  *Touches:* `internal/judge/model.go` (or a timing wrapper); promote/adapt stats.

### C — Rollback / recovery

- **C1 · mvp · S — Committed run manifest** *(satisfied by B2 + A1 once the driver
  commits `run-<id>.json`)* — listed separately so batch-rollback has a trustworthy
  emitted action list, not a reconstruction from `git diff`.

- **C2 · hardening · M — Re-promote / un-demote path.** A command
  (`twiceshy promote -id exp-NNNN` / a `revalidate-and-promote`) that takes a
  wrongly-demoted `stale`/`disputed` record back through the gate+judge and, on a
  hold, returns it to `validated` — **clearing `provenance.valid.until` and the
  `provenance.demotion` block**. Today the only un-demote is a hand-edit.
  *Touches:* `internal/promote` (allow a re-promote entry from stale/disputed), `cmd/twiceshy/main.go`.
  *Acceptance:* a demoted record can be restored by one command; valid.until/demotion are unwound.

- **C3 · hardening · M — True effect-preview dry-run.** Run gate+judge, **write
  nothing**, print the would-be status delta per record (`exp-X: quarantined→validated`).
  Today `-dry-run` only lists candidates (skips gate+judge), so it can't preview the
  actual outcome of a batch.
  *Touches:* promoteCorpus/adaptCorpus no-persist mode; reuse existing dry-run flags.

- **C4 · hardening · S — Validator desync guards.** In `validateProvenance`, reject
  `status=validated` with a past `provenance.valid.until`, and reject `validated`
  still carrying a `provenance.demotion` block. Guards manual reversals from
  silently desyncing (else the staleness doctor re-flags it → validated↔stale flip-flop).
  *Touches:* `internal/record/record.go` validateProvenance + tests.

- **C5 · hardening · S — Rollback runbook.** `docs/` runbook: engage `TWICESHY_PAUSE`;
  close the open promotion PR to veto; `git revert` the night's commit to batch-roll
  back; the C2 command to restore a record. Cross-link ADR-0013 §2 + SCHEMA lifecycle.

### D — Loop robustness / fail-safe

- **D1 · mvp · M — Anomaly = HALT + non-zero exit, checked before persist.** Check
  `Budget.Anomalous()` **before** persisting further actions, stop the run, set a
  distinct non-zero exit, and surface it as a field in the run summary (B2) + a B3
  alert. Today the loop persists then checks then exits 0 — the guardrail detects
  *after* the damage.
  *Touches:* `cmd/twiceshy/main.go` promoteCorpus/adaptCorpus (check-before-persist + propagate), main exit mapping.
  *Acceptance:* a forced anomaly stops mid-run with no further writes, non-zero exit, and a fired alert.

- **D2 · hardening · S — Wire the Reaper at startup.** Call
  `repro.NewReaper().Reap` before the corpus walk (reaper already exists but is
  never invoked by the loop), so a crashed prior run's gVisor containers/volumes are
  swept. Document a periodic sweep too.
  *Touches:* `cmd/twiceshy/main.go`; `internal/repro/reaper.go` (already built).

- **D3 · hardening · M — Fail-safe verification tests.** Table tests for the
  under-covered failure modes: (a) broker/docker outage → attestation error →
  records-before-it persisted, run aborts non-zero, **nothing bad promoted**;
  (b) a poison/unparseable record is skipped, not fatal to the whole run.
  *Touches:* `cmd/twiceshy/promote_test.go`, `adapt_test.go` (broker stub returning start errors + a poison fixture).

- **D4 · hardening · M — Run journal / resume cursor.** A mid-corpus abort leaves a
  machine-readable "promoted X,Y; stopped at Z because <err>" marker; the next run
  resumes rather than re-walking from scratch.
  *Touches:* promoteCorpus/adaptCorpus (journal per action + on abort); reuse the B2 JSON.

### E — Close the closed-loop gaps (from the completeness critic)

- **E1 · mvp · M — `report_outcome` → corpus intake.** A path that materializes
  queued counter-records into `experience/` automatically (e.g. `twiceshy
  intake-reports`, run by the A1 driver) so `adapt` has nightly input. Today
  `report_outcome` returns markdown to the caller and never lands on disk, so the
  "reverse direction" (a misfire self-corrects the corpus) needs a manual
  paste-commit-merge — the break in the auto-adapt chain.
  *Touches:* `internal/server/report.go` (queue) + a new intake CLI; `scripts/scheduled-validate.sh`.
  *Acceptance:* a reported outcome appears as a quarantined counter-record the next run's `adapt` processes, with no human paste step.

- **E2 · hardening · M — Materialize the usage signal.** Flush the serve host's
  SQLite usage (`retrieved/confirmed_helpful/last_hit`) back into
  `provenance.usage` (a delta-only doctor or a nightly flush the driver commits), so
  the loop's `Eligible()` and the daily audit can *read* the reinforcement signal.
  Today it's write-only on one host; the CLI loads markdown and sees `usage:0`.
  *Touches:* `internal/index/usage.go` (materialize); a doctor or driver flush.

- **E3 · hardening · M — Positive-outcome path.** An MCP "this lesson worked" tool
  (and `ConfirmHelpful` caller) so `confirmed_helpful` is non-zero. Today the loop
  can only ever demote/dispute — never reinforce — skewing entirely toward attrition
  (ADR-0013 §4's decay/reinforce balance is otherwise unrealized).
  *Touches:* `internal/server` (a confirm tool), `internal/index/usage.go`.

### F — Judge MVP (from the judge-eval work, PR #89)

- **F0 · mvp · trivial — Merge PR #89.** Calibrated prompt + the `judge-eval`
  harness + gold set + exp-0046. Step 0 of everything below.
- **F1 · mvp · S — Production majority voting.** Judge each record repeat-N
  (default 3), take the majority. Turns the measured single-shot **0.7% → ~0%**
  false-approve. *Touches:* `internal/promote` / the promote judge call.
  *Acceptance:* `promote` calls the judge N times per record and promotes on majority-approve only.
- **F2 · mvp · M — Daily Opus 4.8 audit routine.** A scheduled headless Claude Code
  (Opus 4.8 → **Fable 5** when available) session that reads the night's run
  manifest (B2/C1), re-judges each promotion at full reasoning (seeing *more* than
  the 20B: full body, diff, versions), **auto-demotes/flags disagreements** (via the
  C2/`adapt` path), and posts an ntfy digest. The automated half of the morning
  review. Auditor ≠ drafter for template/qwen-drafted records (independent); the
  thin spot (Claude-drafted prose) is human-gated anyway.
  *Touches:* a `/schedule` routine or systemd timer + a small audit script; reads B2 JSON; writes via C2.
  *Acceptance:* the morning after a run, a digest lists promotions + the audit's agree/disagree per record; disagreements are demoted or flagged.
- **F3 · hardening · S — Adaptive `-confirm` in `judge-eval`.** One pass, then
  re-sample only the cases that flipped (≈3× cheaper re-runs at equal confidence).
- **F4 · ongoing — Grow the gold set from audit misses.** Every F2 disagreement
  becomes a new `internal/judgeeval/gold.yaml` case → re-measure the prompt. The
  flywheel that makes the judge monotonically better.

---

## Ordered execution checklist (the MVP critical path)

Do these in order; each is independently shippable and CI-gated.

1. **F0** — merge PR #89.
2. **B1 + B2** — structured logs + `-json` run manifest (nothing else is legible without this).
3. **D1** — anomaly halt + non-zero exit (stop persisting-before-checking).
4. **B3** — anomaly/guardrail → ntfy.
5. **A2 + A3** — single-flight lock + preflight healthcheck.
6. **F1** — production majority voting.
7. **E1** — `report_outcome` → corpus intake (so `adapt` has input).
8. **A1** — the nightly driver + veto-window PR + timer.
9. → **First instrumented overnight run.** Review the committed `run-<id>.json` + the PR the next morning.
10. **F2** — the daily Opus audit (closes the morning-review loop).

Then the **hardening** tier (B4–B6, C2–C5, D2–D4, E2–E3, F3) in any order, with
F4 running continuously off F2's output.

## Deferred (explicitly out of scope — pre-public / multi-tenant, epic #0010)

- Dashboard + public product presentation.
- Per-agent identity/auth; per-caller rate limit on `report_outcome` (size caps exist).
- Multi-tenant isolation; per-tenant rollback.
- Hard disk-size cap on the per-run `/work` volume (#0025).
- Formal LLM threat model for adversarial submitters (prompt-injection of the judge/drafter).

## References

- ADR-0013 (closed-loop autonomous validation) — the architecture this hardens.
- `internal/judgeeval` + `docs/REGRESSIONS.md` + `experience/2026/0046-*` — the judge calibration + non-determinism finding.
- Issue #0033 (guardrails) — the emergency-stop/anomaly/budget primitives this makes *actionable*.
- Audit source: 5-agent code audit, 2026-06-19 (observability, rollback, loop-robustness, completeness critique).

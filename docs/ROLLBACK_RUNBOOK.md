# Rollback runbook — the autonomous promote/adapt loop

When the nightly loop does the wrong thing — a bad auto-promotion, a wrong
auto-demotion, a suspected compromised judge — this is how you stop it and undo
it. Levers are ordered fastest-to-engage first. None of them delete a record:
twiceshy **supersedes, never deletes** (SCHEMA.md lifecycle; ADR-0013 §2).

Grounded in [ADR-0013](adr/ADR-0013-closed-loop-autonomous-validation.md) §2 (the audited,
reversible loop) and the [SCHEMA](SCHEMA.md) `## Lifecycle` section.

---

## 0. Stop the bleeding — emergency pause (seconds)

Engage the emergency stop so the next scheduled `promote` / `adapt` run writes
**nothing**:

```sh
export TWICESHY_PAUSE=1      # any truthy value; persist it where the cron reads env
```

Both loops honor it before any write:

- `promote: emergency stop engaged (TWICESHY_PAUSE) — no promotions`
- `adapt: emergency stop engaged (TWICESHY_PAUSE) — no demotions`

This is the same flag the anomaly halt tells you to set. Leave it on until you
have rolled back and understand the cause. To resume, unset it
(`unset TWICESHY_PAUSE` / remove it from the cron environment).

---

## 1. Veto an in-flight proposal (before it merges)

The loop proposes changes as a **pull request**, not a direct push. If the
night's promotion/demotion PR is still **open**, you veto it by closing it — no
code rollback needed:

```sh
# inspect the proposed status flips first, then close to veto
#   (close the open promote/adapt PR in the forge UI or via the API)
```

Closing the PR is the cleanest reversal: nothing landed on `main`.

---

## 2. Batch rollback — the change already merged

If the night's commit already merged to `main`, roll the whole batch back with a
revert (preserves history; supersede-not-delete):

```sh
git revert <night-commit-sha>     # the promote/adapt merge/commit
git push                          # land the revert via the normal PR gate
```

Find the commit from the run manifest / the merged PR. Prefer reverting the
**single** promote/adapt commit over hand-editing many records.

---

## 3. Restore one demoted record — `repromote` (#0048)

A wrong auto-demote (sandbox ≠ prod, a flaky counter-report, a compromised
judge) is undone by taking the record **back through the gate + judge**. On a
holding attestation + a judge PASS, `repromote` restores `validated` and unwinds
the demotion — it clears `valid.until` and the `provenance.demotion` block:

```sh
# preview first (no gate/judge, writes nothing):
twiceshy repromote -corpus <dir> -id <exp-NNNN> -dry-run

# then restore for real (needs docker + runsc + a diverse judge model):
twiceshy repromote -corpus <dir> -id <exp-NNNN> \
    -drafter-model <model> -judge-model <diverse-model>
```

`repromote` only acts on a `stale`/`disputed` record and, like `promote`,
re-promotes on **majority**-approve (`-votes`, ADR-0013 §F1) — a single judge
shot can't flip it.

---

## 4. Preview any status flip before acting — `-effect`

Before promote/adapt (or after un-pausing), dry-run the **true effect**: how many
records *would* change status, writing nothing (#0049):

```sh
twiceshy promote -corpus <dir> -effect    # N of M record(s) would change status — nothing written
twiceshy adapt   -corpus <dir> -effect
```

Use this to confirm the loop will do what you expect before you let it write
again.

---

## Recovery checklist

1. `TWICESHY_PAUSE=1` — stop further writes.
2. Open PR? Close it (§1) and you're done.
3. Already merged? `git revert` the batch (§2).
4. One record wrongly demoted? `repromote … -dry-run`, then restore (§3).
5. `-effect` to confirm the next run's effect (§4), then unset `TWICESHY_PAUSE`.
6. Record the cause as an experience record (we dogfood) so the trap is captured.

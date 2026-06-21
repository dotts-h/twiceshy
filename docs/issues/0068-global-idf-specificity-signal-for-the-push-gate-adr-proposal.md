---
id: 0068
title: Global-IDF specificity signal for the push gate (ADR proposal)
status: closed
severity: medium
group: 0064
depends_on: []
forgejo: 254
links:
  adr: docs/adr/ADR-0017-global-idf-push-gate-specificity.md
  prs: [270]
  issues: [0064, 0005]
  regression:
assets: []
---

## Summary
exp-0622's root cause: **document frequency in a tiny corpus is not term
specificity** — common dev words (`http`, `cache`, `request`) have low df only
because the corpus is small, and leaked into off-domain prompts. The shipped fix
(hand-maintained stoplist + fixed `pushMaxDF=3`, ADR-0015) works but is a
hand-tuned guard that needs **re-validation as the corpus grows**. This proposes
a principled replacement.

## Approach
- Derive term specificity from a **large external DEV/CODE background corpus**
  (StackOverflow / GitHub scale), precomputed **offline**, used as a **global
  IDF** lookup. Hot path stays **lexical and embedding-free** (ADR-0001 §4).
  - **Critical:** it must be a *dev/code* corpus, **not** general English —
    general English would rate `http`/`cache`/`request` as rare and
    **re-introduce the exp-0622 bug**. This is the make-or-break detail.
- **Run alongside the existing stoplist first**; retire the stoplist only after
  proving no regression against the negative/positive sets (watch for
  double-filtering genuine queries).

## Explicitly deferred / out of scope
- **Score calibration (Platt / isotonic)** stays **deferred** — it overfits at
  tens-of-records and could itself breach the k≤3 cap by pushing borderline
  queries over the floor (rubber-duck validated). Revisit once #0067 yields real
  labeled volume.
- Wilson CIs are **reported for honesty**, not used as a gate at current N.

## Process
This is an **ADR proposal**, not a silent change. Relates to **ADR-0015**
(push discriminative-term gate) and the **deferred ADR-0006** (score-banding).
Record the decision via `recording-decisions`; supersede, never relitigate the
locked ADRs silently. Add a `TestPushGate*` guard for the global-IDF path.

## Decision recorded
[ADR-0017](../adr/ADR-0017-global-idf-push-gate-specificity.md) (Status: **Proposed**)
records the decision: a global dev/code IDF replaces the hand-maintained stoplist as the
push gate's specificity signal, run **alongside** the stoplist first and retired only
after proving no regression (against the negative/positive sets + #0067 real-traffic
labels); score calibration explicitly deferred. Per the issue framing ("an ADR proposal,
not a silent change") and the repo's pattern (decisions recorded, implementation lands
separately), the **ADR is this issue's deliverable** — the implementation (sourcing the
dev/code background corpus + licensing, the offline IDF precompute, the gate wiring + a
`TestPushGate*` guard) is the follow-up that ADR-0017 authorizes.

## Notes
Child of epic #0064. Independent root-cause quality fix — parallelizable with the
other children. Guarded ultimately by the same live-corpus precision test as
exp-0622 (`TestPushPrecisionOnLiveCorpus`).

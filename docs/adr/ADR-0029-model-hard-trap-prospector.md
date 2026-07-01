# ADR-0029: Model-hard trap prospector — measure per-record value by running the base model without the card

- **Status:** Accepted (deciders: claude drafted from the 2026-07-01 wave-2
  planning session; horia ratified "proceed the same way").
- **Related:** #0005 (the eval this scales — its ModelRunner/BrokerVerifier
  infrastructure is reused, not rebuilt), ADR-0028/#0106 (the push-precision
  fix that makes serving worth measuring in the first place), ADR-0011
  (the drafter/broker heritage this loop's TaskDrafter and BrokerVerifier
  build on), ADR-0013 (off-pool model discipline this loop follows), #0112–
  #0114 (the epic and children this decision authorizes).

## Context

The validated corpus has no per-record value signal. The one live gold case
for #0005's on/off eval is a NULL result: qwen2.5-coder:14b avoids the
React-19 `useRef` trap unaided, on/off arms identical. Hand-curating gold
traps is biased toward what humans GUESS a model will fail — exactly the
guess ADR-0028's push-precision diagnosis showed is unreliable at scale (the
df gate inverted because ordinary dev words looked rare in a small corpus;
a human's intuition about "which record is a trap" is subject to the same
kind of miscalibration). The production served→used join (#0069) measures
real value, but slowly and noisily, and cannot steer which records to add
next. We want a signal that tells us, per record, whether a base model
genuinely needs the card — measured, not guessed.

## Options considered

1. **Hand-curate gold traps.** Doesn't scale past a handful of cases and is
   guess-biased — the same failure mode ADR-0028 diagnosed in the push gate's
   calibration. Rejected as the primary mechanism.
2. **LLM-judge "would a model fail this?"** Still a guess, dressed as a
   judgment call; the usefulness check (#0110) already covers this judgment
   layer at promotion time. Adding a second judged guess doesn't produce a
   measured signal. Rejected.
3. **Prospector: draft a task from the record, run the base model WITHOUT the
   card, verify EXECUTABLY; failures are model-hard by measurement; an
   ON-arm run gives the delta (CHOSEN).** Reuses `agenteval.ModelRunner`
   (`internal/agenteval/runner.go:105`), `BrokerVerifier`
   (`internal/agenteval/verifier.go:56`), and the gVisor broker — off-pool
   models only, embedding-free CI (stubs). The verdict is executable, not
   judged: a bad drafted task fails to discriminate, it cannot fake a
   measured delta.
4. **Rely on the #0069 production join alone.** Complements the prospector
   (it measures real sessions) but is too slow and noisy to steer which
   records to add or which existing ones to re-check. Kept as a
   complementary signal, not a substitute.

## Decision

Adopt Option 3. Numbered:

1. **`twiceshy prospect` measurement loop**, engine-repo outputs only: a
   `runs/` report plus an `agenteval` prospect-gold file. Corpus records are
   never mutated — model-hardness lives in the gold set and the report, not
   as a written-back field on the record (a provenance field on the record
   itself is future work, not part of this decision).
2. **`TaskDrafter` seam.** The drafted task must not leak the trap/escape it
   probes — enforced by an `internal/similarity` shingle-overlap guard
   (`similarity.Assess`/`Report.Flagged`) against the record's resolution
   text: overlap at or above threshold skips the record, counted.
3. **Verify classes v1: parametrized `"tsc"`** (deps derived from the
   record's `applies_to`) **and `"gobuild"`**. A record with no matching
   verify class skips via `ErrTaskUnsupported` and is counted — honest
   coverage reporting over a forced fit.
4. **Eligibility mirrors the push channel's** (ADR-0028): `status =
   validated`, `kind ∈ {trap, fix}`, non-importer
   `provenance.source.author`.
5. **OFF arm is a single run v1.** Sampling (`k > 1`) is deferred until
   variance data exists to justify the added cost.
6. **Env-gated live** (`TWICESHY_AGENTEVAL_*`), hermetic stubs in CI — the
   same discipline #0005's live integration test already follows.

## Consequences

- Per-record measured value (the avoidance delta) becomes the corpus's
  steering metric and the future pack's evidence — replacing guesswork about
  which records matter.
- The gold set grows from measurement, not curation: every new gold case
  carries the OFF-arm failure and (once run) the ON-arm delta that qualified
  it.
- A validated record NO model fails is a push-noise lead — explicitly **not**
  auto-demoted in v1; a demote signal needs its own decision, deferred.
- Task-draft quality is the main risk. It is bounded three ways: the leak
  guard (a drafted task cannot smuggle the trap's own escape), the
  unsupported-skip honesty (`ErrTaskUnsupported` never forces a bad fit into
  a verdict), and executable verification (a bad task fails to discriminate
  cleanly — it cannot fake a delta the way a judged guess could).
- Cost is bounded by `-max` and by running only off-pool/local models,
  matching ADR-0013's off-pool discipline and #0005's existing cost posture.

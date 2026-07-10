# Design-partner measurement protocol

This protocol measures whether twiceshy changes engineering-agent outcomes in a
partner team. It is an operational evaluation, not billing telemetry and not a
claim that an observational comparison proves causality.

Recruitment, consent preparation, onboarding, outreach drafts, and the final decision
rubric live in the [design-partner playbook](DESIGN_PARTNER_PLAYBOOK.md).

## Pilot design

1. Agree a baseline window with injection disabled (recommended: 14 days), then a
   non-overlapping treatment window with the same repositories, agents, and team
   (recommended: 14–28 days). During baseline, the partner-side observer emits the
   same privacy-safe decision shape for error events (`served: []`) without calling
   twiceshy or adding context to the agent. Record deployments or workflow changes
   that could confound the comparison.
2. Enable gate-decision telemetry with raw query capture **off**. The report rejects
   any `query_text` field, unknown field, malformed line, missing file, or invalid
   timestamp rather than producing a partial result. Keep the same
   deployment salt for both windows so repeated salted query and session hashes
   remain comparable. Pass the active JSONL and every required rotation/archive as
   separate, explicit `-telemetry` arguments; no generation is discovered implicitly.
3. Create `cohorts.csv` containing only opaque team labels and already-salted
   session hashes:

   ```csv
   team,session_hash
   payments,0123456789abcdef0123456789abcdef
   ```

   Never put names, email addresses, repository URLs, tokens, prompts, or raw
   session IDs in this file. Sessions absent from the map are reported as
   `unattributed`, not silently discarded.
4. Independently judge served cards and append one privacy-safe JSON object per
   served exposure to `outcomes.jsonl`:

   ```json
   {"ts":"2026-07-08T12:00:00Z","exposure_id":"89abcdef0123456789abcdef01234567","session_hash":"0123456789abcdef0123456789abcdef","record_id":"exp-0149","used":true,"confirmed":true,"incorrect":false}
   ```

   `ts` is the judgement time. `exposure_id` is the v2 stable identity defined by
   [ADR-0036](adr/ADR-0036-privacy-safe-pilot-measurement-attribution.md); the
   judgement is assigned to that exposure's arm, even when reviewed later. Legacy
   rows without it remain supported by deterministic session+record FIFO matching.
   `used` means the agent applied the lesson; `confirmed` is an explicit positive
   outcome; `incorrect` means the advice was wrong or harmful in context. Omit
   `used` when the transcript cannot be judged. Do not store evidence or transcript
   text here—the strict loader rejects unknown fields. Outcomes are attributed only
   when telemetry proves that record was served to that session, capped at the
   number of observed exposures.
5. Run both JSON (audit/archive) and CSV (analysis) reports:

   ```sh
   twiceshy pilot-report \
     -telemetry /var/lib/twiceshy/gate-decisions.2026-07-01.jsonl \
     -telemetry /var/lib/twiceshy/gate-decisions.2026-07-15.jsonl \
     -cohorts cohorts.csv -outcomes outcomes.jsonl \
     -baseline-start 2026-07-01T00:00:00Z -baseline-end 2026-07-15T00:00:00Z \
     -treatment-start 2026-07-15T00:00:00Z -treatment-end 2026-08-12T00:00:00Z \
     -format json > pilot.json
   ```

## Metrics and interpretation

- **Exposure:** a served card; **hit rate:** decisions serving at least one card /
  all decisions.
- **Outcome coverage:** served exposures with a used/ignored judgement / exposures.
  Interpret used and incorrect rates only alongside this coverage.
- **Used rate:** used / judged; **helpful rate:** explicitly confirmed / exposures;
  **incorrect rate:** incorrect / judged.
- **Repeated-error proxy:** after the first `trigger:error` decision, another event
  with the same salted query hash in the same salted session and arm. This is a
  privacy-preserving recurrence proxy, not proof that two stack traces are identical.
- Every rate includes a 95% Wilson interval. Small cohorts and rare records should
  be described as inconclusive when intervals are wide; do not rank records by point
  estimate alone.

Compare matched teams baseline-to-treatment first, then inspect record summaries for
high exposure, narrow intervals, and incorrect reports. Report telemetry drops,
outcome coverage, and operational confounders with every pilot result. A recommended
commercial readiness gate is multiple teams showing lower repeated-error proxy with
no material increase in incorrect rate—not a record-count milestone.

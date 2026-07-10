# ADR-0036: Pilot measurement is fail-closed and attributed to stable exposures

Status: Accepted

## Context

Design-partner reports compare baseline and treatment traffic over weeks. The gate
log rotates, judgements can arrive after an arm closes, and historical outcome files
identify only a salted session plus record. Silently skipping corrupt telemetry makes
a partial report look complete. Assigning a judgement by its timestamp can move a
baseline exposure into treatment. Raw-query telemetry also violates the pilot's
privacy promise even if a report happens not to print it.

## Decision

1. `pilot-report` consumes one or more **explicit** telemetry paths. Every path must
   exist, be non-empty, and contain only valid known decision fields. Malformed JSON,
   invalid timestamps/hashes/enums/counts, unknown fields, and any `query_text` field
   fail the report. Duplicate paths and unknown input modes are rejected. The default
   `disjoint` mode concatenates rotations; the explicit `overlap-snapshot` mode uses
   a multiset maximum to remove copied overlap.
2. An exposure receives `exposure_id = first-16-bytes(SHA-256(NUL-joined(
   "twiceshy-exposure-v1", ts, channel, trigger, salted-session-hash,
   salted-query-hash, record-id, identical-decision-occurrence)))`, hex encoded.
   Occurrence is the zero-based position among exposures sharing that base tuple
   after the selected input-combination policy; score, tokens, and other decision
   metadata do not create a second occurrence namespace. No raw input enters the identity.
3. Outcome schema v2 adds optional `exposure_id`; it is checked against session and
   record and the judgement inherits that exposure's time/arm. Schema-v1 rows without
   the field remain readable by deterministic FIFO matching within session+record;
   all explicit v2 ids are reserved before any legacy row is assigned.
   An unmatched or duplicate outcome fails the report instead of disappearing.
4. JSON and CSV expose the same rates. Every binomial rate carries a 95% Wilson
   interval. Record summaries contain only exposure/outcome denominators; query-level
   decision/hit/repeated-error fields exist only at aggregate and team scopes.
5. Inputs and outputs contain no raw prompt, query, transcript, evidence, source code,
   contact, or raw session id. The outcome decoder rejects unknown fields.

## Consequences

- Operators must retain and name every telemetry generation needed by a 14–28 day
  window and repeat `-telemetry` for each file.
- Only in `overlap-snapshot` mode, snapshots are combined as a multiset union: for each identical event,
  retain the maximum multiplicity present in any one file. This removes copied
  overlap without erasing legitimate same-second repeats recorded within a file.
- In default `disjoint` mode all events from all files are retained. Operators must
  pre-register the relationship; no content heuristic can safely infer it.
- A report stops on incomplete or unsafe inputs, making gaps visible but requiring
  operators to repair or explicitly recollect them.
- Old outcome files remain deterministic; new collectors should write exposure ids to
  remove FIFO ambiguity.
- Wilson intervals communicate sampling uncertainty but do not remove observational
  confounding or establish causality.

# Issues index — twiceshy

Source of truth for tracked work. Markdown files here are canonical; mirror to
Forgejo/Gitea Issues via `scripts/sync-forgejo.sh` (REST API, token-auth). File new issues with
`scripts/new-issue.sh "<title>" [--epic] [--group <id>] [--severity <s>] [--depends id,id]`
— it appends the row here. Format reference: [TEMPLATE.md](TEMPLATE.md).

Epics group children via the `group:` field; an epic may live in the Epics table
or as an `Epic:`-titled row in the Issues table — pickers handle both. Hard
ordering lives in each issue's `depends_on:` frontmatter (real blockers only,
never a cycle).

## Epics

| id | title | status | children |
|----|-------|--------|----------|
| [0008](0008-epic-deployable-twiceshy-remaining-phases.md) | Epic: Deployable twiceshy — the remaining phases | open | 0007, 0006, 0004, 0002, 0005 |
| [0009](0009-epic-pre-deploy-security.md) | Epic: Pre-deploy security hardening (Tier A) | open | 0011, 0012, 0013, 0014 |
| [0010](0010-epic-public-release.md) | Epic: Public release (Tier B) — multi-tenant, trial, anti-abuse | open | — |
| [0027](0027-epic-closed-loop-autonomous-validation-no-human-in-the-provable-loop.md) | Epic: Closed-loop autonomous validation — no human in the provable loop | closed | 0028, 0029, 0030, 0031, 0032, 0033 |
| [0034](0034-epic-go-live-hardening-bulletproof-the-autonomous-loop-single-tenant.md) | Epic: Go-live hardening — bulletproof the autonomous loop (single-tenant) | closed | 0035, 0036, 0037, 0038, 0039, 0040, 0041, 0042, 0043, 0044, 0045, 0046, 0047, 0048, 0049, 0050, 0051, 0052, 0053, 0054, 0055, 0056, 0057, 0058 |

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|
| [0001](0001-seed-corpus.md) | Phase 0 — seed the corpus from our own repos | closed | medium | — | ADR-0001 |
| [0002](0002-push-path.md) | Push path — hook → trap cards via additionalContext | closed | high | 0008 | ADR-0001 §5 |
| [0003](0003-write-path.md) | Phase 3 — write path + quarantine (record_experience) | closed | high | — | ADR-0008 |
| [0004](0004-doctors.md) | Doctors — framework + D2 staleness (D1/D3/D4/D5 deferred, ADR-0010) | closed | high | 0008 | ADR-0001 §7 |
| [0005](0005-evals-trap-avoidance.md) | Trap-avoidance eval suite — memory on/off regression | in-progress | medium | 0008 | ADR-0001 §8 |
| [0006](0006-dense-retrieval-sqlite-vec-rrf.md) | Dense retrieval — pure-Go cosine + RRF (pull channel only) | closed | medium | 0008 | ADR-0006 |
| [0007](0007-corpus-importer.md) | Corpus importer — license-clean version-knowledge bootstrap | closed | high | 0008 | ADR-0003 |
| [0011](0011-ingestion-safety-gate.md) | Ingestion safety gate — secret/harmful-code/PII screening | closed | high | 0009 | SEC §2 |
| [0012](0012-injection-safe-rendering.md) | Injection-safe rendering — record content is data, not instructions | closed | high | 0009 | SEC §1 |
| [0013](0013-app-hardening-gaps.md) | App-hardening gaps — body cap, timeouts, rate limit, path/error hygiene | closed | medium | 0009 | SEC §3 |
| [0014](0014-ongoing-security-maintenance.md) | Ongoing security maintenance — deps, OSV self-dogfood, PR checklist | open | medium | 0009 | SEC §4 |
| [0015](0015-epic-adr-0011-corpus-growth-as-a-live-feed-execution-validation-engine.md) | Epic: ADR-0011 — Corpus growth as a live feed + execution-validation engine | open | high | — | |
| [0016](0016-schema-guard-test-set-positive-negative-repros-schema-version-1-compatible.md) | Schema — guard test-set (positive+negative repros), schema_version-1 compatible | closed | high | 0015 | PR#58 |
| [0017](0017-gvisor-runsc-on-the-brain-pinned-repro-base-images-go-ecosystem.md) | gVisor/runsc on the brain + pinned repro-base images (Go ecosystem) | closed | high | 0015 | |
| [0018](0018-sandbox-broker-watchdog-ephemeral-gvisor-hardcoded-policy-two-phase-egress-guaranteed-cleanup.md) | Sandbox broker + watchdog — ephemeral gVisor, hardcoded policy, two-phase egress, guaranteed cleanup | closed | critical | 0015 | |
| [0019](0019-extend-ingestion-screen-to-repro-script-content-execution-trust-boundary.md) | Extend ingestion screen to repro-script content + execution trust boundary | closed | high | 0015 | |
| [0020](0020-internal-repro-revalidate-doctor-version-matrix-attestation-report-only-go-first.md) | internal/repro revalidate doctor — version matrix + attestation, report-only (Go first) | closed | high | 0015 | |
| [0021](0021-live-osv-importer-fetch-osv-dev-deterministic-distill-quarantined-idempotent.md) | Live OSV importer — fetch osv.dev, deterministic distill, quarantined, idempotent | closed | high | 0015 | PR#62 |
| [0022](0022-schedule-live-importers-on-the-brain-the-feed-heartbeat-cron.md) | Schedule live importers on the brain — the feed heartbeat (cron) | closed | medium | 0015 | PR#65,66 |
| [0023](0023-live-deprecation-importer-deps-dev-endoflife-to-deprecation-codemod-records-quarantined.md) | Live deprecation importer — deps.dev/endoflife to deprecation+codemod records, quarantined | closed | medium | 0015 | |
| [0024](0024-llm-wrong-canon-so-reframe-authoring-gated-on-adr-0011-section-5-sign-off.md) | LLM-wrong canon + SO-reframe authoring (§5 accepted internal-only; commercial still gated) | closed | medium | 0015 | AUTHORING, corpus exp-2758 |
| [0025](0025-hard-disk-size-cap-on-the-repro-work-volume-multi-tenant-precondition.md) | Hard disk-size cap on the repro work volume (multi-tenant precondition) | open | medium | 0015 | |
| [0026](0026-repro-test-gen-pipeline-drafter-to-broker-gate-to-attach-adr-0011-section-8.md) | Repro test-gen pipeline — drafter to broker-gate to attach (ADR-0011 section 8) | closed | high | 0015 | PR#265 |
| [0028](0028-judge-seam-diverse-model-verdict-on-proven-records.md) | Judge seam — diverse-model verdict on a proven record (meaning/scope/license/poison) | closed | high | 0027 | ADR-0013, PR#81 |
| [0029](0029-auto-promotion-attestation-plus-judge-promotes-quarantined-to-validated.md) | Auto-promotion — attestation + judge PASS promotes quarantined to validated | closed | high | 0027 | ADR-0013, PR#84 |
| [0030](0030-usage-signal-retrieval-increments-provenance-usage.md) | Usage signal — retrieval increments provenance.usage (unblocks D4) | closed | medium | 0027 | ADR-0013, PR#82 |
| [0031](0031-outcome-report-intake-mcp-report-outcome-gated-counter-evidence.md) | Outcome-report intake — MCP report_outcome, quarantined counter-evidence | closed | medium | 0027 | ADR-0013, PR#83 |
| [0032](0032-counter-evidence-gate-and-adapt-demote-or-supersede-on-reproduced-failure.md) | Counter-evidence gate + adapt — demote/supersede on reproduced failure | closed | high | 0027 | ADR-0013, PR#85 |
| [0033](0033-guardrails-anomaly-monitoring-emergency-stop-budget-caps.md) | Guardrails — anomaly monitoring, emergency stop, budget caps | closed | high | 0027 | ADR-0013, PR#86 |
| [0035](0035-structured-slog-logging-on-the-promote-adapt-loop.md) | Structured slog logging on the promote/adapt loop | closed | high | 0034 | ADR-0013, PR#150 |
| [0036](0036-json-run-manifest-for-promote-adapt.md) | `-json` run manifest for promote/adapt | closed | high | 0034 | ADR-0013, PR#151 |
| [0037](0037-anomaly-halt-non-zero-exit-checked-before-persist.md) | Anomaly = HALT + non-zero exit, checked before persist | closed | high | 0034 | ADR-0013, PR#152 |
| [0038](0038-route-guardrail-trips-to-a-channel-ntfy-notify-seam.md) | Route guardrail trips to a channel (ntfy notify seam) | closed | high | 0034 | ADR-0013, PR#153 |
| [0039](0039-single-flight-lock-around-promote-adapt.md) | Single-flight lock around promote/adapt | closed | high | 0034 | ADR-0013, PR#154 |
| [0040](0040-preflight-healthcheck-docker-runsc-judge-liveness.md) | Preflight healthcheck (docker/runsc + judge liveness) | closed | medium | 0034 | ADR-0013, PR#155 |
| [0041](0041-production-majority-voting-in-promote.md) | Production majority voting in promote | closed | high | 0034 | ADR-0013, PR#156 |
| [0042](0042-report-outcome-corpus-intake-so-adapt-has-nightly-input.md) | report_outcome → corpus intake (so adapt has nightly input) | closed | high | 0034 | ADR-0013, PR#157 |
| [0043](0043-nightly-validate-driver-adr-0013-2-veto-window-pr.md) | Nightly validate driver + ADR-0013 §2 veto-window PR | closed | high | 0034 | ADR-0013, PR#158 |
| [0044](0044-daily-opus-4-8-audit-routine-auto-demote-disagreements.md) | Daily Opus-4.8 audit routine (auto-demote disagreements) | closed | high | 0034 | ADR-0013, PR#159 |
| [0045](0045-success-heartbeat-uptime-kuma-push.md) | Success heartbeat (Uptime-Kuma push) | closed | medium | 0034 | ADR-0013, PR#160 |
| [0046](0046-unify-broker-reaper-logging-to-slog.md) | Unify broker reaper logging to slog | closed | low | 0034 | ADR-0013, PR#161 |
| [0047](0047-judge-latency-verdict-distribution-metrics.md) | Judge latency + verdict-distribution metrics | closed | medium | 0034 | ADR-0013, PR#162 |
| [0048](0048-re-promote-un-demote-path.md) | Re-promote / un-demote path | closed | high | 0034 | ADR-0013, PR#163 |
| [0049](0049-true-effect-preview-dry-run.md) | True effect-preview dry-run | closed | medium | 0034 | ADR-0013, PR#164 |
| [0050](0050-validator-desync-guards-valid-until-demotion.md) | Validator desync guards (valid.until / demotion) | closed | medium | 0034 | ADR-0013; deps — |
| [0051](0051-rollback-runbook.md) | Rollback runbook | closed | medium | 0034 | ADR-0013; deps [#0043, #0048] |
| [0052](0052-wire-the-reaper-at-promote-adapt-startup.md) | Wire the Reaper at promote/adapt startup | closed | medium | 0034 | ADR-0013; deps — |
| [0053](0053-fail-safe-verification-tests-broker-outage-poison-record.md) | Fail-safe verification tests (broker outage / poison record) | closed | high | 0034 | ADR-0013; deps — |
| [0054](0054-run-journal-resume-cursor.md) | Run journal / resume cursor | closed | medium | 0034 | ADR-0013; deps [#0036] |
| [0055](0055-materialize-the-usage-signal-into-provenance-usage.md) | Materialize the usage signal into provenance.usage | closed | medium | 0034 | ADR-0013; deps — |
| [0056](0056-positive-outcome-mcp-path-confirmed-helpful.md) | Positive-outcome MCP path (confirmed_helpful) | closed | medium | 0034 | ADR-0013; deps — |
| [0057](0057-adaptive-confirm-mode-in-judge-eval.md) | Adaptive `-confirm` mode in judge-eval | closed | low | 0034 | ADR-0013; deps — |
| [0058](0058-grow-the-gold-set-from-daily-audit-misses-ongoing.md) | Grow the gold set from daily-audit misses (ongoing) | closed | low | 0034 | ADR-0013; deps [#0044] |
| [0059](0059-record-experience-id-allocator-returns-colliding-ids-indexes-fewer-records-than-the-filesystem.md) | record_experience id allocator returns colliding ids (indexes fewer records than the filesystem) | closed | medium | 0008 | PR#261 |
| [0060](0060-automate-corpus-sync-to-the-nas-volume-live-corpus-drifts-from-the-repo-drift-crash-looped-serve.md) | Automate corpus sync to the NAS volume (live corpus drifts from the repo; drift crash-looped serve) | closed | high | 0008 | PR#264 |
| [0061](0061-importer-transcription-fidelity-bugs-mis-scoped-ecosystem-malformed-package-path-fixed-null-fix-text-contradiction.md) | Importer transcription-fidelity bugs: mis-scoped ecosystem, malformed package path, fixed-null fix-text contradiction | open | medium | — | |
| [0062](0062-buildadvisoryprompt-omits-fixed-null-line-cheap-advisory-judge-can-t-catch-the-largest-0061-defect-class.md) | BuildAdvisoryPrompt omits fixed:null line — cheap advisory judge can't catch the largest #0061 defect class | closed | medium | 0015 | PR#263 |
| [0063](0063-judgeeval-lacks-advisory-prompt-routing-can-t-measure-cheap-judges-on-the-sonnet-advisory-gold-set.md) | judgeeval lacks advisory-prompt routing — can't measure cheap judges on the Sonnet advisory gold-set | closed | medium | 0015 | |
| [0064](0064-epic-agent-native-feedback-loop-capture-submit-measure.md) | Epic: Agent-native feedback loop — capture, submit, measure | open | high | — | |
| [0065](0065-session-retro-capture-hook-automatic-trap-submission-at-the-lifecycle-seam.md) | Session-retro capture hook — automatic trap submission at the lifecycle seam | closed | high | 0064 | ADR-0018 |
| [0066](0066-agent-issue-submission-tool-report-issue-intake.md) | Agent issue-submission tool (report_issue) + intake | closed | medium | 0064 | |
| [0067](0067-per-query-gate-decision-telemetry.md) | Per-query gate-decision telemetry | closed | medium | 0064 | PR#269 |
| [0068](0068-global-idf-specificity-signal-for-the-push-gate-adr-proposal.md) | Global-IDF specificity signal for the push gate (ADR proposal) | closed | medium | 0064 | ADR-0017 |
| [0069](0069-session-retro-helpfulness-signal-join-transcript-against-the-0067-decision-log-to-score-served-cards-used-vs-ignored.md) | Session-retro helpfulness signal — join transcript against the #0067 decision log to score served cards used vs ignored | open | medium | 0064 | |
| [0070](0070-prose-class-promotion-path-captured-session-traps-are-quarantined-dead-weight-without-a-non-advisory-promoter-decide-via-adr-extend-adr-0016-panel.md) | Prose-class promotion path — captured session traps are quarantined dead-weight without a non-advisory promoter (decide via ADR, extend ADR-0016 panel) | closed | high | 0064 | ADR-0020 |
| [0071](0071-promotion-side-eol-gap-advisory-class-promotion-of-eol-runtime-advisories-trips-the-validated-staleness-guard-validate-prs-stuck-promote-companion-to-302.md) | Promotion-side EOL gap — advisory-class promotion of EOL-runtime advisories trips the validated staleness guard (validate PRs stuck); promote companion to #302 | closed | high | 0015 | |
| [0072](0072-corpus-pipeline-hardening-importer-pre-flight-gate-ntfy-on-failure-red-pr-stall-alarm-never-silent-again-web-watchers-osv-historical-vs-recent-fetch.md) | Corpus-pipeline hardening — importer pre-flight gate + ntfy on failure, red-PR stall alarm (never silent again), web watchers, osv historical-vs-recent fetch | closed | high | 0015 | |
| [0073](0073-non-osv-web-watchers-changelog-advisory-eol-deprecation-feeds-emit-quarantined-drafts-split-from-0072-item-3.md) | Non-OSV web watchers — changelog/advisory/EOL/deprecation feeds emit quarantined drafts (split from #0072 item 3) | closed | medium | 0015 | |
| [0074](0074-ingest-the-85-sonnet-advisory-labels-as-advisory-gold-cases-uses-0063-routing.md) | Ingest the 85 Sonnet advisory labels as advisory gold cases (uses #0063 routing) | closed | medium | 0015 | |
| [0075](0075-intake-issues-drainer-materialize-the-report-issue-spool-into-docs-issues-uses-0066-capture.md) | intake-issues drainer — materialize the report_issue spool into docs/issues/ (uses #0066 capture) | closed | medium | 0064 | |
| [0076](0076-epic-decouple-the-corpus-into-a-versioned-data-product-adr-0021.md) | Epic: Decouple the corpus into a versioned data product (ADR-0021) | closed | high | — | |
| [0077](0077-decouple-pre-flight-gemini-agy-gut-check-lossless-corpus-snapshot-tag.md) | Decouple pre-flight: gemini+agy gut-check + lossless corpus snapshot tag | closed | medium | 0076 | ADR-0021 |
| [0078](0078-interim-take-corpus-imports-off-the-code-ci-shim-relax-outdated-branch-slower-cadence.md) | Interim: take corpus imports off the code CI (shim + relax outdated-branch + slower cadence) | closed | medium | 0076 | ADR-0021 |
| [0079](0079-schema-contract-frozen-test-fixtures-engine-stops-loading-the-live-corpus-in-ci.md) | Schema contract + frozen test fixtures (engine stops loading the live corpus in CI) | closed | high | 0076 | ADR-0021 |
| [0080](0080-stand-up-the-twiceshy-corpus-store-its-ci-the-no-silent-freeze-stall-alarm.md) | Stand up the twiceshy-corpus store + its CI + the no-silent-freeze stall alarm | closed | high | 0076 | ADR-0021 |
| [0081](0081-quiesce-cut-over-make-the-corpus-store-authoritative-re-point-sync-importer-loop.md) | Quiesce + cut over: make the corpus store authoritative, re-point sync/importer/loop | closed | critical | 0076 | ADR-0021 |
| [0082](0082-restart-on-the-new-home-verify-end-to-end.md) | Restart on the new home + verify end-to-end | closed | high | 0076 | ADR-0021 |
| [0083](0083-decommission-the-engine-repo-corpus-path.md) | Decommission the engine-repo corpus path | closed | medium | 0076 | |
| [0084](0084-promote-throughput-throttle-decoupled-from-anomaly-halt-and-hold-cooldown.md) | Promote throughput — decouple the throttle from the anomaly halt + hold cooldown | closed | high | 0034 | ADR-0022 |
| [0085](0085-approval-rate-anomaly-that-survives-a-throughput-cap.md) | Promotion-rate anomaly that survives a throughput cap | closed | medium | 0034 | ADR-0022 |
| [0086](0086-hybrid-gemini-sonnet-advisory-panel-frontier-judge.md) | Hybrid advisory-panel frontier judge — Gemini primary, Sonnet fallback | closed | medium | 0034 | ADR-0016 |
| [0087](0087-error-scoped-retrieval-trigger-posttooluse-hook-queries-twiceshy-with-the-verbatim-error-line-on-the-2nd-occurrence.md) | Error-scoped retrieval trigger — PostToolUse hook queries twiceshy with the verbatim error line on the 2nd occurrence | closed | high | 0064 | |
| [0088](0088-corpus-coverage-seed-engineering-traps-across-the-full-stack-rn-react-ts-python-native-not-just-security-advisories.md) | Corpus coverage — seed engineering traps across the full stack (RN/React/TS/Python/native), not just security advisories | open | high | 0015 | |
| [0089](0089-record-experience-id-allocation-collides-two-drafts-in-one-session-get-the-same-exp-nnnn-route-through-ingest-nextid-exp-0743-stale-id-trap.md) | record_experience id allocation collides — two drafts in one session get the same exp-NNNN; route through ingest.NextID (exp-0743 stale-id trap) | closed | medium | — | |
| [0090](0090-authored-record-similarity-check-flag-near-verbatim-reproduction-of-public-snippets-adr-0011-5-mitigation.md) | Authored-record similarity check — flag near-verbatim reproduction of public snippets (ADR-0011 §5 mitigation) | closed | medium | 0015 | |
| [0091](0091-authoring-scaffold-cli-twiceshy-author-pre-stages-a-record-repro-skeleton.md) | Authoring-scaffold CLI — twiceshy author pre-stages a record + repro skeleton | closed | low | 0015 | |
| [0092](0092-teststructuredloggingemitssafefields-flakes-under-the-full-race-suite-logging-concurrency-nondeterminism.md) | TestStructuredLoggingEmitsSafeFields flakes under the full -race suite (logging-concurrency nondeterminism) | open | low | — | |

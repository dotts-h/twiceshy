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
| [0061](0061-importer-transcription-fidelity-bugs-mis-scoped-ecosystem-malformed-package-path-fixed-null-fix-text-contradiction.md) | Importer transcription-fidelity bugs: mis-scoped ecosystem, malformed package path, fixed-null fix-text contradiction | open | medium | 0015 | |
| [0062](0062-buildadvisoryprompt-omits-fixed-null-line-cheap-advisory-judge-can-t-catch-the-largest-0061-defect-class.md) | BuildAdvisoryPrompt omits fixed:null line — cheap advisory judge can't catch the largest #0061 defect class | closed | medium | 0015 | PR#263 |
| [0063](0063-judgeeval-lacks-advisory-prompt-routing-can-t-measure-cheap-judges-on-the-sonnet-advisory-gold-set.md) | judgeeval lacks advisory-prompt routing — can't measure cheap judges on the Sonnet advisory gold-set | closed | medium | 0015 | |
| [0064](0064-epic-agent-native-feedback-loop-capture-submit-measure.md) | Epic: Agent-native feedback loop — capture, submit, measure | open | high | — | |
| [0065](0065-session-retro-capture-hook-automatic-trap-submission-at-the-lifecycle-seam.md) | Session-retro capture hook — automatic trap submission at the lifecycle seam | closed | high | 0064 | ADR-0018 |
| [0066](0066-agent-issue-submission-tool-report-issue-intake.md) | Agent issue-submission tool (report_issue) + intake | closed | medium | 0064 | |
| [0067](0067-per-query-gate-decision-telemetry.md) | Per-query gate-decision telemetry | closed | medium | 0064 | PR#269 |
| [0068](0068-global-idf-specificity-signal-for-the-push-gate-adr-proposal.md) | Global-IDF specificity signal for the push gate (ADR proposal) | closed | medium | 0064 | ADR-0017 |
| [0069](0069-session-retro-helpfulness-signal-join-transcript-against-the-0067-decision-log-to-score-served-cards-used-vs-ignored.md) | Session-retro helpfulness signal — join transcript against the #0067 decision log to score served cards used vs ignored | closed | medium | 0064 | |
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
| [0092](0092-teststructuredloggingemitssafefields-flakes-under-the-full-race-suite-logging-concurrency-nondeterminism.md) | TestStructuredLoggingEmitsSafeFields flakes under the full -race suite (logging-concurrency nondeterminism) | closed | low | — | |
| [0093](0093-twiceshy-ntfy-alerting-still-mute-on-the-go-binary-anomaly-path-and-corpus-stall-alarm-sh-missing-bearer-token.md) | twiceshy ntfy alerting still mute on the Go binary anomaly path and corpus-stall-alarm.sh — missing Bearer token | closed | medium | — | |
| [0094](0094-higher-signal-experience-capture-git-fail-to-fix-miner-twiceshy-learned-command-structurally-beats-transcript-mining.md) | Higher-signal experience capture — git fail-to-fix miner + twiceshy learned command (structurally beats transcript mining) | in-progress | medium | — | |
| [0095](0095-opt-in-author-side-pii-redactor-clear-incidental-private-ip-email-so-a-draft-isn-t-quarantined.md) | Opt-in author-side PII redactor — clear incidental private-IP/email so a draft isn't quarantined | closed | low | — | |
| [0096](0096-engine-binary-drifts-stale-from-main-no-post-merge-redeploy-or-version-guard-scheduled-jobs-break-on-new-flags.md) | Engine binary drifts stale from main — no post-merge redeploy or version guard; scheduled jobs break on new flags | closed | high | — | |
| [0097](0097-capture-pipeline-stalls-are-silent-watch-real-experience-inflow-queue-drain-not-total-record-count.md) | Capture-pipeline stalls are silent — watch real-experience inflow / queue-drain, not total record count | closed | high | — | |
| [0098](0098-activate-the-measurement-chain-in-production-enable-serve-telemetry-cross-host-decision-log-access-so-the-0069-helpfulness-join-runs-live.md) | Activate the measurement chain in production — cross-host decision-log sync so the #0069 helpfulness join runs live on real traffic | closed | medium | 0064 | |
| [0099](0099-usage-judge-502-retro-analyzer-shim-rejects-the-verdicts-schema-zeroing-the-0069-confirmed-helpful-signal.md) | Usage-judge 502: retro analyzer shim rejects the verdicts schema, zeroing the #0069 confirmed-helpful signal | closed | high | 0064 | |
| [0100](0100-retro-transcripts-dead-lettered-on-the-first-transient-analyzer-failure-no-retry-flaky-gpt-oss-empty-mis-shaped-output-permanently-loses-recoverable-learning.md) | Retro transcripts dead-lettered on the first transient analyzer failure (no retry) — flaky gpt-oss empty/mis-shaped output permanently loses recoverable learning | closed | high | 0064 | |
| [0101](0101-epic-fleet-wide-enforcement-adapters-adr-0026-o3-capture-inject-across-gemini-cli-ask-codex-ask-cursor-wrappers-and-the-non-agentic-gateway-floor.md) | Epic: Fleet-wide enforcement adapters (ADR-0026 O3) — capture & inject across Gemini CLI, ask-codex/ask-cursor wrappers, and the non-agentic gateway floor | closed | medium | — | |
| [0102](0102-gemini-cli-sessionend-shipper-post-retro-on-session-end-mirror-the-claude-code-hook.md) | Gemini CLI SessionEnd shipper — POST /retro on session end (mirror the Claude Code hook) | closed | medium | 0101 | |
| [0103](0103-wrapper-session-end-shippers-for-ask-codex-and-ask-cursor-stop-finally-post-retro.md) | Wrapper session-end shippers for ask-codex and ask-cursor (Stop/finally → POST /retro) | closed | medium | 0101 | |
| [0104](0104-gateway-floor-for-non-agentic-executors-code-exec-injects-pre-call-and-emits-a-session-end-record-on-close.md) | Gateway floor for non-agentic executors — code-exec injects pre-call and emits a session-end record on close | closed | medium | 0101 | |
| [0105](0105-served-used-confirmed-helpful-stuck-at-0-retro-analyzer-gpt-oss-20b-drops-22-of-transcripts-as-unprocessable-throttling-the-0069-join.md) | Served→used confirmed-helpful stuck at 0: retro analyzer (gpt-oss:20b) drops ~22% of transcripts as unprocessable, throttling the #0069 join | closed | high | 0064 | |
| [0106](0106-epic-push-precision-collapse-serve-agent-actionable-cards-only-measured.md) | Epic: Push precision collapse — serve agent-actionable cards only, measured | open | high | — | |
| [0107](0107-push-eligibility-only-agent-actionable-origins-and-kinds-reach-the-push-channel.md) | Push eligibility: only agent-actionable origins and kinds reach the push channel | closed | high | 0106 | |
| [0108](0108-two-token-corroboration-for-prompt-triggered-push-error-trigger-keeps-single-token.md) | Two-token corroboration for prompt-triggered push; error trigger keeps single-token | closed | high | 0106 | |
| [0109](0109-flag-gated-raw-query-text-on-gate-decision-telemetry.md) | Flag-gated raw query text on gate-decision telemetry | closed | medium | 0106 | |
| [0110](0110-promotion-judge-gains-a-usefulness-dimension-would-this-card-change-a-competent-agent-s-action.md) | Promotion judge gains a usefulness dimension: would this card change a competent agent's action | closed | medium | 0106 | |
| [0111](0111-extend-the-push-stoplist-with-a-static-dev-vocabulary-list-adr-0017-cheap-proxy.md) | Extend the push stoplist with a static dev-vocabulary list (ADR-0017 cheap proxy) | open | low | 0106 | |
| [0112](0112-epic-model-hard-trap-prospector-measure-which-records-flip-a-base-model.md) | Epic: Model-hard trap prospector — measure which records flip a base model | open | high | — | |
| [0113](0113-prospector-core-record-to-task-templating-off-arm-run-broker-verdict-model-hard-report.md) | Prospector core: record-to-task templating, OFF-arm run, broker verdict, model-hard report | closed | high | 0112 | |
| [0114](0114-prospector-gold-emission-failed-tasks-become-0005-gold-cases-on-arm-delta-measured.md) | Prospector gold emission: failed tasks become #0005 gold cases; ON-arm delta measured | closed | medium | 0112 | |
| [0115](0115-breaking-changes-release-notes-source-mine-breaking-sections-from-github-releases-into-quarantined-drafts.md) | Breaking-changes source: mine Node.js SEMVER-MAJOR changelog entries into quarantined drafts (v1 of the release-notes miners) | closed | high | — | |
| [0116](0116-metrics-digest-daily-push-join-promote-health-digest-to-ntfy.md) | Metrics digest: daily push/join/promote health digest to ntfy | closed | medium | — | |
| [0117](0117-dev-code-idf-table-compute-word-level-df-from-a-permissive-code-corpus-adr-0017-phase-1.md) | Dev-code IDF table: compute word-level df from a permissive code corpus (ADR-0017 phase 1) | closed | medium | — | |
| [0118](0118-push-eligibility-excludes-importer-origin-breaking-change-traps-decide-the-admission-path.md) | Push eligibility excludes importer-origin breaking-change traps — decide the admission path | closed | medium | — | |
| [0119](0119-prospector-verdicts-need-a-per-case-positive-control-run-1-scored-5-5-vacuous-model-hard.md) | Prospector verdicts need a per-case positive control — run 1 scored 5/5 vacuous model-hard | closed | high | 0112 | |
| [0120](0120-live-corpus-push-recall-guard-dormant-since-adr-0021-fails-0-6-on-the-grown-corpus.md) | Live-corpus push recall guard dormant since ADR-0021 — fails 0/6 on the grown corpus | open | medium | — | ADR-0021 |
| [0121](0121-merge-safe-id-allocation-ignores-open-prs-parallel-open-corpus-prs-allocate-colliding-record-ids.md) | Merge-safe ID allocation ignores OPEN PRs — parallel-open corpus PRs allocate colliding record IDs | open | medium | 0064 | ADR-0021 |
| [0122](0122-promotions-liveness-alarm-alert-on-consecutive-promoted-0-validate-runs-while-quarantine-is-non-empty.md) | Promotions-liveness alarm: alert on consecutive promoted=0 validate runs while quarantine is non-empty | closed | high | — | |
| [0123](0123-validate-run-timeout-poisons-the-backlog-context-canceled-judge-calls-enter-the-168h-hold-cooldown-and-the-uncommitted-batch-is-lost.md) | Validate-run timeout poisons the backlog: context-canceled judge calls enter the 168h hold cooldown and the uncommitted batch is lost | closed | high | — | |
| [0124](0124-epic-public-alpha-hosted-multi-tenant-remote-mcp-service-adr-0030.md) | Epic: Public alpha — hosted multi-tenant remote-MCP service (ADR-0030) | open | high | — | |
| [0125](0125-multi-tenant-token-layer-issue-revoke-per-token-quotas-and-rate-limits-on-the-mcp-surface.md) | Multi-tenant token layer: issue/revoke, per-token quotas and rate limits on the MCP surface | closed | medium | 0124 | |
| [0126](0126-per-tenant-telemetry-token-dimension-on-usage-gate-decisions-operator-dashboard.md) | Per-tenant telemetry: token dimension on usage + gate decisions, operator dashboard | closed | medium | 0124 | |
| [0127](0127-landing-page-product-explainer-self-serve-token-signup-terms-acceptance.md) | Landing page: product explainer, self-serve token signup, terms acceptance | closed | medium | 0124 | |
| [0128](0128-write-path-hardening-for-untrusted-contributors-hostile-input-pii-secret-scrub-low-trust-origin-tier.md) | Write-path hardening for untrusted contributors: hostile-input PII/secret scrub + low-trust origin tier | closed | medium | 0124 | |
| [0129](0129-off-homelab-public-deployment-isolated-host-tls-own-corpus-clone-and-secrets.md) | Off-homelab public deployment: isolated host, TLS, own corpus clone and secrets | closed | medium | 0124 | |
| [0130](0130-contribution-data-license-terms-privacy-policy-for-the-hosted-alpha.md) | Contribution data-license terms + privacy policy for the hosted alpha | closed | medium | 0124 | |
| [0131](0131-alpha-follow-ups-from-pr-511-512-review-quota-debit-ordering-over-quota-counter-inflation-signup-per-ip-cap-behind-a-proxy.md) | Alpha follow-ups from PR #511/#512 review: quota debit ordering, over-quota counter inflation, signup per-IP cap behind a proxy | closed | medium | — | |
| [0132](0132-landing-page-fast-follow-live-try-a-search-demo-box-backed-by-a-rate-limited-public-demo-token.md) | Landing page fast-follow: live try-a-search demo box backed by a rate-limited public demo token | open | low | 0124 | |
| [0133](0133-external-oss-git-trap-miner-mine-fix-shaped-commits-from-permissively-licensed-high-star-repos-react-rn-swift-kotlin-coverage.md) | External-OSS git trap miner — mine fix-shaped commits from permissively-licensed high-star repos (React/RN/Swift/Kotlin coverage) | closed | medium | 0088 | |
| [0134](0134-wtfjs-wtfpython-importer-wtfpl-curated-gotcha-collections-as-quarantined-experience-records.md) | wtfjs/wtfpython importer — WTFPL curated gotcha collections as quarantined experience records | closed | low | 0088 | |
| [0135](0135-atomic-fail-closed-contribution-quota-debit-decouple-write-path-quota-enforcement-from-best-effort-tenant-telemetry.md) | Atomic fail-closed contribution-quota debit — decouple write-path quota enforcement from best-effort tenant telemetry | closed | high | 0124 | |
| [0136](0136-unified-alpha-write-path-policy-seam-forced-origin-stamping-caps-and-secret-posture-for-report-outcome-report-issue-quota-for-confirm-helpful-operator-only-retro.md) | Unified alpha write-path policy seam — forced origin stamping, caps and secret posture for report_outcome/report_issue, quota for confirm_helpful, operator-only /retro | closed | high | 0124 | |
| [0137](0137-tenant-registry-is-not-derived-state-rebuild-preservation-test-contract-wording-runbook-backup-note-adr-0034-phase-1.md) | Tenant registry is not derived state — Rebuild-preservation test, contract wording, runbook backup note (ADR-0034 phase 1) | closed | medium | 0124 | |
| [0138](0138-declared-middleware-pipeline-stage-list-with-checked-requires-after-constraints-per-request-state-signup-joins-the-chain.md) | Declared middleware pipeline — stage list with checked requires/after constraints, per-request state, signup joins the chain | closed | medium | 0124 | |
| [0139](0139-enable-the-alpha-write-path-0128-adr-0030-phase-2-record-experience-spool-vps-queue-wiring-brain-side-spool-pull-docs-copy.md) | Enable the alpha write path (#0128 / ADR-0030 phase 2): record-experience spool, VPS queue wiring, brain-side spool pull, docs copy | closed | high | 0124 | |
| [0140](0140-live-prospector-run-17-record-re-run-with-positive-controls-model-hard-set-or-honest-null-deferred-from-0119.md) | Live prospector run — 17-record re-run with positive controls; model-hard set or honest null (deferred from 0119) | closed | high | 0112 | |
| [0141](0141-index-md-is-hand-maintained-derived-state-generate-it-from-issue-frontmatter-or-teach-next-issue-sh-to-read-frontmatter-so-drift-is-impossible-not-just-detected.md) | INDEX.md is hand-maintained derived state — generate it from issue frontmatter (or teach next-issue.sh to read frontmatter) so drift is impossible, not just detected | open | low | — | |
| [0142](0142-prospect-run-aborts-on-npm-e404-for-a-record-derived-dep-deterministic-dep-not-found-must-be-a-counted-per-case-skip-deps-not-a-run-level-error.md) | Prospect run aborts on npm E404 for a record-derived dep — deterministic dep-not-found must be a counted per-case skip (deps), not a run-level error | closed | high | 0112 | |
| [0143](0143-live-model-edge-a-single-transport-blip-timeout-reset-aborts-a-whole-multi-record-prospect-sweep-retry-once-with-backoff-bump-drafter-timeout-to-match-runner.md) | Live model edge: a single transport blip (timeout/reset) aborts a whole multi-record prospect sweep — retry once with backoff; bump drafter timeout to match runner | closed | medium | 0112 | |
| [0144](0144-drafted-task-quality-56-control-fail-loss-task-record-relevance-mismatches-produce-noise-model-hard-verdicts-and-the-report-lacks-per-record-skip-reasons.md) | Drafted-task quality: 56% control-fail loss, task-record relevance mismatches produce noise model-hard verdicts, and the report lacks per-record skip reasons | open | high | 0112 | |
| [0145](0145-announce-the-alpha-remaining-mcp-directory-listings-show-hn-post-operator-fired.md) | Announce the alpha: remaining MCP directory listings + Show HN post (operator-fired) | open | medium | 0124 | |
| [0146](0146-usage-judge-recall-is-0-33-on-the-synthetic-gold-set-precision-1-0-confirmed-helpful-will-under-count-real-usage-when-it-happens.md) | Usage judge recall is 0.33 on the synthetic gold set (precision 1.0) — confirmed_helpful will under-count real usage when it happens | open | medium | 0064 | |
| [0147](0147-wtf-importer-record-quality-polish-synthetic-anchor-slug-error-signatures-auto-composed-symptom-summaries-empty-title-heading-skip.md) | wtf importer record-quality polish: synthetic anchor-slug error_signatures, auto-composed symptom summaries, empty-title heading skip | open | low | 0088 | |
| [0148](0148-validate-driver-preserve-the-journaled-promote-batch-across-an-out-of-band-sigterm-reboot-deploy-so-partial-work-survives-the-next-run-s-git-reset-hard.md) | Validate driver: preserve the journaled promote batch across an out-of-band SIGTERM (reboot/deploy) so partial work survives the next run's git reset --hard | open | medium | 0034 | |

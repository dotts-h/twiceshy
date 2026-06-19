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
| [0034](0034-epic-go-live-hardening-bulletproof-the-autonomous-loop-single-tenant.md) | Epic: Go-live hardening — bulletproof the autonomous loop (single-tenant) | open | 0035, 0036, 0037, 0038, 0039, 0040, 0041, 0042, 0043, 0044, 0045, 0046, 0047, 0048, 0049, 0050, 0051, 0052, 0053, 0054, 0055, 0056, 0057, 0058 |

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|
| [0001](0001-seed-corpus.md) | Phase 0 — seed the corpus from our own repos | closed | medium | — | ADR-0001 |
| [0002](0002-push-path.md) | Push path — hook → trap cards via additionalContext | open | high | 0008 | ADR-0001 §5 |
| [0003](0003-write-path.md) | Phase 3 — write path + quarantine (record_experience) | closed | high | — | ADR-0008 |
| [0004](0004-doctors.md) | Doctors — framework + D2 staleness (D1/D3/D4/D5 deferred, ADR-0010) | closed | high | 0008 | ADR-0001 §7 |
| [0005](0005-evals-trap-avoidance.md) | Trap-avoidance eval suite — memory on/off regression | in-progress | medium | 0008 | ADR-0001 §8 |
| [0006](0006-dense-retrieval-sqlite-vec-rrf.md) | Dense retrieval — pure-Go cosine + RRF (pull channel only) | closed | medium | 0008 | ADR-0006 |
| [0007](0007-corpus-importer.md) | Corpus importer — license-clean version-knowledge bootstrap | closed | high | 0008 | ADR-0003 |
| [0011](0011-ingestion-safety-gate.md) | Ingestion safety gate — secret/harmful-code/PII screening | closed | high | 0009 | SEC §2 |
| [0012](0012-injection-safe-rendering.md) | Injection-safe rendering — record content is data, not instructions | open | high | 0009 | SEC §1 |
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
| [0023](0023-live-deprecation-importer-deps-dev-endoflife-to-deprecation-codemod-records-quarantined.md) | Live deprecation importer — deps.dev/endoflife to deprecation+codemod records, quarantined | open | medium | 0015 | |
| [0024](0024-llm-wrong-canon-so-reframe-authoring-gated-on-adr-0011-section-5-sign-off.md) | LLM-wrong canon + SO-reframe authoring (GATED on ADR-0011 section 5 sign-off) | open | medium | 0015 | |
| [0025](0025-hard-disk-size-cap-on-the-repro-work-volume-multi-tenant-precondition.md) | Hard disk-size cap on the repro work volume (multi-tenant precondition) | open | medium | 0015 | |
| [0026](0026-repro-test-gen-pipeline-drafter-to-broker-gate-to-attach-adr-0011-section-8.md) | Repro test-gen pipeline — drafter to broker-gate to attach (ADR-0011 section 8) | in-progress | high | 0015 | |
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
| [0041](0041-production-majority-voting-in-promote.md) | Production majority voting in promote | open | high | 0034 | ADR-0013; deps — |
| [0042](0042-report-outcome-corpus-intake-so-adapt-has-nightly-input.md) | report_outcome → corpus intake (so adapt has nightly input) | open | high | 0034 | ADR-0013; deps — |
| [0043](0043-nightly-validate-driver-adr-0013-2-veto-window-pr.md) | Nightly validate driver + ADR-0013 §2 veto-window PR | open | high | 0034 | ADR-0013; deps [#0036, #0037, #0038, #0039, #0040, #0041, #0042] |
| [0044](0044-daily-opus-4-8-audit-routine-auto-demote-disagreements.md) | Daily Opus-4.8 audit routine (auto-demote disagreements) | open | high | 0034 | ADR-0013; deps [#0043, #0036] |
| [0045](0045-success-heartbeat-uptime-kuma-push.md) | Success heartbeat (Uptime-Kuma push) | open | medium | 0034 | ADR-0013; deps [#0043] |
| [0046](0046-unify-broker-reaper-logging-to-slog.md) | Unify broker reaper logging to slog | open | low | 0034 | ADR-0013; deps [#0035] |
| [0047](0047-judge-latency-verdict-distribution-metrics.md) | Judge latency + verdict-distribution metrics | open | medium | 0034 | ADR-0013; deps [#0036] |
| [0048](0048-re-promote-un-demote-path.md) | Re-promote / un-demote path | open | high | 0034 | ADR-0013; deps — |
| [0049](0049-true-effect-preview-dry-run.md) | True effect-preview dry-run | open | medium | 0034 | ADR-0013; deps — |
| [0050](0050-validator-desync-guards-valid-until-demotion.md) | Validator desync guards (valid.until / demotion) | open | medium | 0034 | ADR-0013; deps — |
| [0051](0051-rollback-runbook.md) | Rollback runbook | open | medium | 0034 | ADR-0013; deps [#0043, #0048] |
| [0052](0052-wire-the-reaper-at-promote-adapt-startup.md) | Wire the Reaper at promote/adapt startup | open | medium | 0034 | ADR-0013; deps — |
| [0053](0053-fail-safe-verification-tests-broker-outage-poison-record.md) | Fail-safe verification tests (broker outage / poison record) | open | high | 0034 | ADR-0013; deps — |
| [0054](0054-run-journal-resume-cursor.md) | Run journal / resume cursor | open | medium | 0034 | ADR-0013; deps [#0036] |
| [0055](0055-materialize-the-usage-signal-into-provenance-usage.md) | Materialize the usage signal into provenance.usage | open | medium | 0034 | ADR-0013; deps — |
| [0056](0056-positive-outcome-mcp-path-confirmed-helpful.md) | Positive-outcome MCP path (confirmed_helpful) | open | medium | 0034 | ADR-0013; deps — |
| [0057](0057-adaptive-confirm-mode-in-judge-eval.md) | Adaptive `-confirm` mode in judge-eval | open | low | 0034 | ADR-0013; deps — |
| [0058](0058-grow-the-gold-set-from-daily-audit-misses-ongoing.md) | Grow the gold set from daily-audit misses (ongoing) | open | low | 0034 | ADR-0013; deps [#0044] |

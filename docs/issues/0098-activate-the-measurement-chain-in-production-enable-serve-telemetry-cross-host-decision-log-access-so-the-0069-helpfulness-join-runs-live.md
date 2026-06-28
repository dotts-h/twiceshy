---
id: 0098
title: Activate the measurement chain in production — enable serve telemetry + cross-host decision-log access so the #0069 helpfulness join runs live
status: open
severity: medium
group: 0064
depends_on: []
forgejo: 417
links:
  adr:
  prs: []
  issues: [0067, 0069]
  regression:
assets: []
---

## Summary

The whole #0067/#0069 measurement chain (per-query gate-decision log → served-set reader →
served-vs-used helpfulness join, built and merged in PRs #415/#416) is **built but DORMANT in
production**. No `TWICESHY_TELEMETRY_*` is configured anywhere in the deployed env, so the serve
container never writes the #0067 decision log — and with no decision log, the join has nothing to
attribute verdicts against (it confirms nothing, silently). The code path is complete and tested;
this is the *operational activation* that makes the signal actually flow.

The wrinkle is that activation is **cross-host**:
- The serve (MCP) runs as a **Docker container on the NAS** (`192.168.50.244:8722`, mounted `/data`
  volume; restarted by the brain's `twiceshy-watchdog` over SSH).
- The **retro drain** (`twiceshy-retro.service` → `scheduled-retro.sh` → `twiceshy retro-intake`)
  runs on the **brain**.
- The join reads the serve's decision log from the brain — so the log must live on a path **both**
  can reach: a shared mount, a sync (like `corpus-sync` mirrors `experience/`), or run the drain on
  the NAS next to `/data`.

## Repro
1. `grep -rE 'TWICESHY_TELEMETRY' /etc/twiceshy/*.env /home/ori/.config/twiceshy/*.env` → no hits.
2. The deployed serve `docker run` has no `-telemetry-log`, so no `/data/decisions.log` is written.
Expected: a live `search_experience` appends a decision (served ids + salted session hash); a later
retro drain confirms a used+served card via `RecordHelpfulnessAttributed`.
Actual: nothing is logged; the join is a silent no-op; `confirmed_helpful` never advances from real
traffic; #0069 acceptance 3's "real-traffic precision/recall" is unmeasurable.

## Evidence
- The code is ready (no engine change needed): `serve -telemetry-log <path>` + `TWICESHY_TELEMETRY_SALT`
  enable the decision log; `retro-intake -telemetry-log` defaults to `getenv("TWICESHY_TELEMETRY_LOG")`
  and the salt to `TWICESHY_TELEMETRY_SALT` — both MUST match the serve's, or the session hash
  diverges and `ServedInSession` returns empty (the failure `internal/telemetry/hash_test.go` guards
  in code, but the *salt value* must also match across the two deployed processes).
- Serve deployment: `docs/DEPLOY.md` (container on NAS, `/data` volume). Retro: `scripts/scheduled-retro.sh`,
  `scripts/twiceshy-retro.service` (`EnvironmentFile=/home/ori/.config/twiceshy/retro.env`).

## Acceptance
- [ ] The serve writes the #0067 decision log to a path the brain's retro drain can read (decide:
      shared mount vs sync vs drain-on-NAS — an architecture call; may warrant a short ADR).
- [ ] `retro.env` carries `TWICESHY_TELEMETRY_LOG` (that path) + `TWICESHY_TELEMETRY_SALT` matching
      the serve container's salt.
- [ ] Verified end-to-end: a live `search_experience` produces a decision-log entry, and a subsequent
      retro drain confirms a `Used`-and-served card (the join logs `confirmed N helpful`).

## Notes

Unblocks #0069 acceptance 3 (real-traffic precision/recall — currently measured only on the synthetic
`UsageGold` set) and feeds #0005 slice 2. Pure ops/wiring + one architecture decision (where the log
lives); no twiceshy engine change. Found 2026-06-28 while shipping the join (#415) and the judge-eval
(#416). Under the #0064 measurement epic.

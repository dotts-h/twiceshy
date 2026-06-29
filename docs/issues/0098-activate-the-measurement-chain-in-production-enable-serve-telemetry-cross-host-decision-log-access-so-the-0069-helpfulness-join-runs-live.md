---
id: 0098
title: Activate the measurement chain in production — cross-host decision-log sync so the #0069 helpfulness join runs live on real traffic
status: open
severity: medium
group: 0064
depends_on: []
forgejo: 417
links:
  adr: docs/adr/ADR-0026-runtime-enforcement-of-experience-adoption.md
  prs: [421, 425]
  issues: [0067, 0069]
  regression:
assets: []
---

## Summary

**Corrected premise (2026-06-29, verified against prod).** The serve-side half is *already live*:
the running container `twiceshy:v0.2.8` runs `serve … -telemetry-log /data/gate-decisions.jsonl`
and the log is actively written (338 records, last entry 2026-06-29 05:52Z). The original repro
(`grep TWICESHY_TELEMETRY` → no hits) was misleading: the flag is a **CLI arg**, not an env var.
So "enable serve telemetry" is **done**; this issue is now scoped to the *other* half — getting
that log to the join.

The whole #0067/#0069 chain (per-query gate-decision log → served-set reader → served-vs-used
helpfulness join, merged in PRs #415/#416) still does not run live, because the **join runs on the
brain and the log lives on the NAS**:
- The serve (MCP) runs as a **Docker container on the NAS** (`192.168.50.244:8722`, `/data` volume
  owned by the distroless uid `65532`).
- The **retro drain** (`twiceshy-retro.service` → `scheduled-retro.sh` → `twiceshy retro-intake`)
  runs on the **brain**, and `retro-intake` activates the join only when `TWICESHY_TELEMETRY_LOG`
  points at a readable decision log (retro.go:40).
- **Decision (chosen 2026-06-29):** pull the log **NAS→brain on a timer**, mirroring `corpus-sync`
  (rejected: a shared NFS/SMB mount, and relocating the drain to the NAS — both add coupling the
  system otherwise avoids). The brain reads its local copy; no mount, no drain relocation.

## Repro
1. On the NAS, the serve already writes the log: `docker inspect twiceshy` shows
   `-telemetry-log /data/gate-decisions.jsonl`, and the file has 338+ live records.
2. On the **brain**, `/home/ori/.config/twiceshy/retro.env` sets no `TWICESHY_TELEMETRY_LOG`, and
   no brain-local copy of the log exists — so `retro-intake` builds no join (retro.go:66), and the
   served-vs-used attribution never runs.
Expected: the brain holds a fresh copy of the decision log; a retro drain confirms a used+served
card via the join (`confirmed N helpful`).
Actual: the log is stranded on the NAS; the join is a silent no-op; `confirmed_helpful` never
advances from real traffic; #0069 acceptance 3's "real-traffic precision/recall" is unmeasurable.

## Evidence
- No engine change needed. The serve writes the log (live, verified). `retro-intake` activates the
  join when `-telemetry-log` (default `getenv("TWICESHY_TELEMETRY_LOG")`) is non-empty (retro.go:40,66)
  and hashes sessions with `TWICESHY_TELEMETRY_SALT` (retro.go:71). The serve runs with **no salt env
  set ⇒ empty salt ⇒ unsalted `sha256`**, so the brain drain must also use an empty salt or
  `ServedInSession` returns empty (`internal/telemetry/hash_test.go` guards the divergence).
- The pull: `scripts/sync-decisions-from-nas.sh` + `twiceshy-decisions-sync.{service,timer}` mirror
  `corpus-sync`, reading the log via the uid-65532-safe `docker run --rm -v twiceshy-data:/data
  alpine cat` idiom to `/home/ori/twiceshy-telemetry/gate-decisions.jsonl`.
- Serve deployment: `docs/DEPLOY.md`. Retro: `scripts/scheduled-retro.sh`,
  `scripts/twiceshy-retro.service` (`EnvironmentFile=/home/ori/.config/twiceshy/retro.env`).

## Acceptance
- [x] A brain-side timer pulls the serve's decision log NAS→brain to a path the retro drain reads
      (sync, mirroring corpus-sync — `sync-decisions-from-nas.sh` + `twiceshy-decisions-sync.{service,timer}`,
      PR #421; timer enabled on the brain, log present at `/home/ori/twiceshy-telemetry/gate-decisions.jsonl`).
- [x] `retro.env` carries `TWICESHY_TELEMETRY_LOG` (the brain-local path) + a salt that matches the
      serve's — which falls back to the **bearer token** (telemetrySalt), NOT empty. Fixed in PR #425;
      retro-intake now applies the same token fallback (`scheduled-retro.sh` sources `TWICESHY_TOKEN`).
- [x] Verified end-to-end: with the matching salt the drain's `ServedInSession` resolves correlated
      sessions to their served sets (proven: `Hash(token, known-session)` reproduces the logged hash
      exactly, and **6 of 77 transcripts** correlate with served-card sessions — was 0 under the wrong
      salt). The join LOGIC (confirm-only-Used) is CI-tested in `internal/retro/helpful_test.go`;
      `confirmed N` flows from the next scheduled drain (timer live).

## Resolution (2026-06-29)

The chain is **wired live and now correlates** (after the PR #425 salt fix); the residual is neither
plumbing nor traffic — `confirmed N` flows from the next scheduled drain.

- **Corrected premise:** #0067 telemetry was never dormant — the prod serve (`twiceshy:v0.2.8`) already
  ran with `-telemetry-log` (a CLI arg, which is why the original `grep TWICESHY_TELEMETRY <env>` repro
  found nothing). Gotcha logged: inspect a container's *args*, not just its env, before declaring a
  flag-configured feature off.
- **Shipped (PR #421):** the NAS→brain decision-log sync (script + units + DEPLOY.md), the brain
  `retro.env` wiring (log path + matching empty salt), and the 30-min sync timer.
- **Correction (2026-06-29, PR #425): the "0 correlation" was a SALT BUG, not adoption.** The first
  resolution concluded the join produced ~0 confirmations because searched- and captured-sessions don't
  overlap, and read that as evidence for ADR-0026's thesis. **That was wrong.** The serve salts session
  hashes with the **bearer token** (`TWICESHY_TELEMETRY_SALT` empty → token, main.go); the brain
  `retro.env` (PR #421) set the salt **empty**, so every session hash diverged and the join matched
  nothing — *by construction*, regardless of traffic. Re-correlating with `salt=token` finds **6**
  matches. Root-caused by reading the serve's salt-resolution CODE (its env has no salt var, which is
  what misled me). Fixed via `telemetrySalt` (serve + retro now share the rule) + sourcing
  `TWICESHY_TOKEN` into the drain. ADR-0026 enforcement is still valid for *pull adoption* and *fleet
  coverage*, but it was never the blocker for the feedback join — this salt fix is.

## Notes

Unblocks #0069 acceptance 3 (real-traffic precision/recall — currently measured only on the synthetic
`UsageGold` set) and feeds #0005 slice 2. Pure ops/wiring (the architecture call — sync NAS→brain — is
decided); no twiceshy engine change. The serve-side telemetry turned out **already live** (v0.2.8), so
this narrowed from "enable telemetry + cross-host access" to just the cross-host sync + salt match.
Found 2026-06-28 while shipping the join (#415) and the judge-eval (#416); premise corrected 2026-06-29
against prod. Under the #0064 measurement epic.

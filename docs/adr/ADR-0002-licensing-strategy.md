# ADR-0002: Licensing strategy — AGPL core, CLA-gated contributions, separately licensed experience packs

- **Status:** Accepted (2026-06-12)
- **Deciders:** horia
- **Related:** [ADR-0001 §10](ADR-0001-architecture.md), repo `LICENSE`
  (AGPL-3.0, set at repo creation).

## Context

twiceshy's code is useful, but the durable value is the **validated
experience corpus** the system accumulates — records that have survived
sandbox revalidation and human review. The corpus is the moat, not the
server. The licensing setup must (a) keep the code copyleft so hosted
derivatives give back, (b) keep a future commercial path open, and (c) keep
the corpus's value capturable independently of the code.

## Decision

1. **The code is AGPL-3.0-only.** The network-interaction clause is the
   point: anyone operating a modified twiceshy as a service must offer its
   source to users. No license exceptions are granted ad hoc.

2. **Dual-licensing is kept possible: CLA before any external contribution
   is merged.** Every external contributor signs a Contributor License
   Agreement granting the project the right to relicense their contribution
   before their first PR is merged. Without 100% CLA coverage,
   dual-licensing dies the day the first un-papered patch lands. Maintainer
   commits are covered by ownership.

3. **Paid functionality ships as separate services/processes — never
   proprietary code linked into the AGPL core.** Anything commercial
   (hosted multi-tenant control plane, premium doctors, eval dashboards)
   communicates with the AGPL core over its network APIs (MCP/HTTP) as an
   independent program. No proprietary module may be compiled or linked
   into this Go module, and this module must not import proprietary code.
   This keeps the AGPL boundary clean in both directions.

4. **The experience corpus ("experience packs") is licensed separately from
   the code.** Records under `experience/` in this repo are part of this
   repo's development history, but *distributable packs* of validated
   records are data products, not program code, and carry their own license
   terms per pack (which may be commercial, source-available, or open).
   AGPL obligations attach to the program; they do not automatically
   open-license the data the program serves.

## Consequences

- A `CLA.md` + signing workflow must exist **before** the repo accepts its
  first external PR; until then, external PRs are not merged. (Tracked as a
  pre-public-launch task.)
- Pack licensing terms live with the pack (e.g. `LICENSE` inside the pack
  artifact), not in this repo's LICENSE.
- Commercial features must be designed API-first against the core's public
  surface — which doubles as good architecture discipline.
- Contributors' employers' IP policies become our problem at CLA time, not
  at lawsuit time.
- If dual-licensing is never exercised, the cost was one signature per
  contributor; if it is, it was the whole ballgame.

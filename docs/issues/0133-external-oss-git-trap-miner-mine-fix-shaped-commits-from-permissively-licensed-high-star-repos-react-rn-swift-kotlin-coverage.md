---
id: 0133
title: External-OSS git trap miner — mine fix-shaped commits from permissively-licensed high-star repos (React/RN/Swift/Kotlin coverage)
status: open
severity: medium
group: 0088
depends_on: []
forgejo: 534
links:
  adr:
  prs: []
  issues: [0094, 0134]
  regression:
assets: []
---

## Summary

Extend the proven git fail→fix trap miner (#0094 engine: fix-shaped non-merge
commits → bounded message+diff queue entry → retro-intake → quarantine → judge)
from our own repos to **external permissively-licensed high-star OSS repos**.
This is the only identified web-scale source of *experience-shaped* records
(first-person trap account in the commit message, validated fix in the diff,
guard in the regression test) — as opposed to the documentation-shaped catalogs
in `docs/WEB_SOURCES.md`. Target the epic's zero-coverage areas first: React,
React Native/Expo, Swift/iOS, Kotlin/Android, TS frontend.

## Repro
1. `mcp twiceshy search_experience` for any React Native / Swift / Kotlin trap.
Expected: relevant experience records.
Actual: zero records in those ecosystems (#0088 evidence); corpus is ~64%
imported security advisories.

## Evidence

- #0094 first batch: 11 real dev-stack traps from our repos alone (corpus PR
  #34) — the engine works; this issue is scale-out, not new machinery.
- `docs/WEB_SOURCES.md` row 16: issue/comment *prose* is a no-go, but
  "mine licensed commits/tests; retain only independently derived facts" is
  explicitly in-bounds — commits and tests carry the repo's own license.

## Status 2026-07-08 (PR pending) — machinery shipped, rollout gated

Built and verified; the rollout wave is deliberately NOT fired (operator call).

- **Part A (engine):** the retro-queue `Transcript` carries `source_url` +
  `source_license`, and `retro-intake` stamps them into each mined draft's
  structured provenance — the legal-compliance prerequisite (an external MIT/
  Apache commit records its upstream URL + SPDX license as fields, not prose).
- **Part B (script):** `scripts/external-trap-miner.sh` — a curated-seedfile,
  license-gated miner. Permissive SPDX allowlist {MIT, Apache-2.0, BSD-2/3,
  ISC, 0BSD, Unlicense, WTFPL} enforced BEFORE clone; everything else (GPL/
  AGPL/NOASSERTION/empty/unresolvable) is fail-closed rejected, never cloned.
  Hermetic tests (`external-trap-miner.test.sh`, in `make test-scripts`) prove
  the reject path never invokes clone. **Real dry-run 2026-07-08:** react-use
  (→Unlicense) mined 3 React fix-commits with correct provenance; git/git
  (→NOASSERTION) rejected before clone.
- **Rollout (remaining, operator):** curate `external-trap-miner.seeds.example`
  per gap ecosystem and run ONE small batch at a time into the live retro queue,
  letting validate drain between batches (judge-budget: a large wave = days).
  No timer is enabled and the script is not deployed to `~/.local/bin` — firing
  the wave is a deliberate operator step, not automated here.

## Notes

Scope sketch (engine reuse, mostly configuration + policy):

- **License allowlist gate before cloning**: SPDX in {MIT, Apache-2.0,
  BSD-2/3-Clause, ISC, WTFPL, 0BSD, Unlicense}; skip NOASSERTION/none (e.g.
  100-go-mistakes verified license-less 2026-07-07). Record the license +
  commit SHA in provenance.
- **Repo selection**: curated seed list per gap ecosystem (high-star, active,
  permissive), not a crawler. Shallow clones, bounded per-repo commit budget.
- **Candidate filter**: same fix-shaped heuristics as #0094 (fix/bug/regression
  markers, small diff, test-touching preferred) + MAXDIFF bound so entries fit
  the 4096-ctx analyzer without chunking.
- **Provenance**: `author=git-history`, plus upstream repo URL + SHA; dedup on
  lesson-identity, not commit hash (Codex reframe, see #0094).
- **Volume control**: everything lands quarantined; promote pipeline currently
  drains ~720/day so a 10³-candidate wave is ~days of judge budget — batch the
  rollout per ecosystem, don't fire-hose.
- Legal boundary restated: mine the *commit + diff + test* under the repo
  license; never ingest linked issue/PR discussion prose (row 16 stands).

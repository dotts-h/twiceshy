---
id: 0008
title: "Epic: Deployable twiceshy — the remaining phases"
status: open
severity: high
group:
depends_on: []
forgejo: 98
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: []
  issues: [0002, 0004, 0005, 0006, 0007]
  regression:
assets: []
---

## Summary
The umbrella for finishing twiceshy to a deployable state: build the remaining
phases **in order → seed the corpus → deploy** (NAS Docker = always-on server;
brain = engine for the importer, the doctors' sandbox repro execution, and
evals — ADR-0001 §9). Read path (Phase 1) and write path (Phase 3) already ship.

## Children (build order; hard edges in each child's `depends_on`)
1. **#0007 corpus importer** — BUILD FIRST; unblocked. Seeds the (quarantined) corpus.
2. **#0006 dense retrieval** — unblocked, parallelizable; pull-search quality + ADR-0006.
3. **#0004 doctors** — depends on #0007; D3 promotes quarantine→validated.
4. **#0002 push path** — depends on #0004; auto-injection at decision time.
5. **#0005 evals** — depends on #0002; prove the store helps.

## Acceptance (tick when the epic closes)
- [ ] #0007, #0006, #0004, #0002, #0005 all closed.
- [ ] Corpus seeded from license-clean sources; ≥1 record reaches `validated` via D3.
- [ ] Server deployed (NAS Docker) and registered as an MCP endpoint we use.
- [ ] Trap-avoidance eval shows measurable avoidance with memory on vs off.

## Notes
Roadmap of record: [docs/NEXT_FEATURES.md](../NEXT_FEATURES.md). The
test-generation / "check issues in isolated containers" enhancement is an
explicit **post-deploy** conversation, not in this epic.

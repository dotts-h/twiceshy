# Architecture Decision Records — twiceshy

One record per decision, MADR-lite (Context · Options · Decision · Consequences ·
Status). Files are `NNNN-kebab-title.md`, zero-padded, monotonic. Never edit an
accepted ADR's decision — supersede it (`Status: superseded-by-NNNN`).

**A new ADR isn't landed until this index has its row.**

| id | title | status |
|----|-------|--------|
| [0001](ADR-0001-architecture.md) | twiceshy architecture — git-backed records, derived SQLite index, hybrid injection | Accepted |
| [0002](ADR-0002-licensing-strategy.md) | Licensing strategy — AGPL core, CLA-gated contributions, separate experience packs | Accepted |
| [0003](ADR-0003-corpus-bootstrap-source-scope.md) | Corpus bootstrap source scope — license-clean only, seeded precision-first | Accepted |
| [0004](ADR-0004-relevance-floor-is-index-policy.md) | The relevance floor is index policy, not a per-call argument | Accepted |
| [0005](ADR-0005-stable-seams.md) | Register the stable seams in CONTRACTS.md | Accepted |
| [0006](ADR-0006-defer-score-banding.md) | Keep three-state novelty; defer score-banding to the dense phase | Accepted |
| [0007](ADR-0007-floor-on-the-read-path.md) | The relevance floor applies to the read path too, via a single injection seam | Accepted |
| [0008](ADR-0008-write-path-persistence-is-a-cli-concern.md) | Persistence is a trusted-CLI concern — the MCP server never holds push credentials | Accepted |
| [0009](ADR-0009-dense-retrieval-is-pure-go-cosine.md) | Dense retrieval is pure-Go cosine, not sqlite-vec — preserve the CGO-free build | Accepted |
| [0010](ADR-0010-doctors-build-d2-defer-the-rest.md) | Doctors — build the framework + D2 staleness now; defer D1/D3/D4/D5 | Accepted |
| [0011](ADR-0011-corpus-growth-and-validation-engine.md) | Corpus growth as a live feed, with execution-validation as the moat | Proposed |
| [0012](ADR-0012-cicd-trust-posture-and-runner-isolation.md) | CI/CD trust posture — self-merge gate + an isolated hardened runner for twiceshy CI | Accepted |

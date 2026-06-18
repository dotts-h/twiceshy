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

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|
| [0001](0001-seed-corpus.md) | Phase 0 — seed the corpus from our own repos | closed | medium | — | ADR-0001 |
| [0002](0002-push-path.md) | Push path — hook → trap cards via additionalContext | open | high | 0008 | ADR-0001 §5 |
| [0003](0003-write-path.md) | Phase 3 — write path + quarantine (record_experience) | closed | high | — | ADR-0008 |
| [0004](0004-doctors.md) | Doctors — background store-hygiene jobs (D1–D5), delta-only | open | high | 0008 | ADR-0001 §7 |
| [0005](0005-evals-trap-avoidance.md) | Trap-avoidance eval suite — memory on/off regression | open | medium | 0008 | ADR-0001 §8 |
| [0006](0006-dense-retrieval-sqlite-vec-rrf.md) | Dense retrieval — sqlite-vec + RRF (pull channel only) | open | medium | 0008 | ADR-0006 |
| [0007](0007-corpus-importer.md) | Corpus importer — license-clean version-knowledge bootstrap | open | high | 0008 | ADR-0003 |

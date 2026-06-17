# Issues index — twiceshy

Source of truth for tracked work. Markdown files here are canonical; mirror to
GitHub via `scripts/sync-github.sh` (requires `gh`). File new issues with
`scripts/new-issue.sh "<title>" [--epic] [--group <id>] [--severity <s>] [--depends id,id]`
— it appends the row here. Format reference: [TEMPLATE.md](TEMPLATE.md).

Epics group children via the `group:` field; an epic may live in the Epics table
or as an `Epic:`-titled row in the Issues table — pickers handle both. Hard
ordering lives in each issue's `depends_on:` frontmatter (real blockers only,
never a cycle).

## Epics

| id | title | status | children |
|----|-------|--------|----------|

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|

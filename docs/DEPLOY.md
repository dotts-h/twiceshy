# DEPLOY.md — running twiceshy (single-tenant)

> The single-tenant server: the **pull** MCP tools (`search_experience`,
> `get_experience`, `record_experience`) over streamable HTTP, plus the
> importer-seeded corpus and (optional) dense retrieval. Topology per
> [ADR-0001 §9](adr/ADR-0001-architecture.md): one Go service in **Docker on the
> NAS**; the brain is the engine (importer, doctors, evals). Pre-deploy security
> gate: [SECURITY_ANALYSIS.md](research/SECURITY_ANALYSIS.md) Tier A — ingestion
> safety gate (#0011) and app-hardening (#0013) are in; the push channel is not
> deployed (it awaits D3 / runnable repros — ADR-0010).

## Image

Pure-Go / CGO-free (ADR-0009) → a static binary on `distroless/static:nonroot`
(non-root, no shell — #0013 container hardening). Build:

```sh
docker build -t twiceshy:<tag> .
```

## Volume layout (`/data`)

The container is read-only except a mounted `/data`:

- `/data/corpus/` — the experience-record repo (the `experience/` tree + the
  importer's output). The **source of truth**; back this up, not the index.
- `/data/twiceshy.db` — the derived SQLite index. Rebuilt from the corpus on
  every `serve` start, so it is disposable.

**Volume permissions (gotcha):** the image runs as the distroless **nonroot uid
`65532`** (no shell, can't `chown` at runtime). The host `/data` must be
readable (corpus) and writable (the db) by uid 65532, e.g.
`sudo chown -R 65532:65532 /opt/twiceshy/data`. A volume owned by another uid
fails with `loading corpus: … permission denied` on start (verified).

## Run

```sh
docker run -d --name twiceshy --restart unless-stopped \
  -p 8722:8722 \
  -e TWICESHY_TOKEN="$(openssl rand -hex 32)" \
  -v /opt/twiceshy/data:/data \
  twiceshy:<tag>
```

- **`TWICESHY_TOKEN`** is mandatory (no unauthenticated mode). Store it in the
  stack's secret store ([[secrets-store]]), never in the compose file in git.
- **Dense retrieval (optional, pull-only):** add
  `-e` … and `--`, i.e. override the command with `-embed-url
  http://192.168.50.150:11434` to point at the Ollama VM. Empty → dense off,
  server falls back to fingerprint + BM25 (ADR-0009). The server never hard-fails
  if Ollama is down.

## Seed the corpus

The importer runs on the **brain** (engine), writing license-clean quarantined
records into the corpus repo, which are merged via PR and then synced to
`/data/corpus` on the NAS:

```sh
twiceshy ingest go   -corpus . -db /tmp/ix.db
twiceshy ingest osv  -corpus . -db /tmp/ix.db
twiceshy ingest py   -corpus . -db /tmp/ix.db
# review the new experience/ files, PR + merge, then deploy/refresh the corpus volume
```

Everything imported is born `quarantined` (pull-only, labeled) and screened by
the ingestion safety gate (#0011); promotion to `validated` awaits D3 (ADR-0010).

## Register as an MCP server (the consumer)

twiceshy speaks **streamable HTTP** MCP. In the consuming agent's MCP config,
add an HTTP server pointing at the listen address with the bearer token, e.g.
(Claude Code `~/.claude` MCP config):

```jsonc
{
  "mcpServers": {
    "twiceshy": {
      "type": "http",
      "url": "https://<twiceshy-host>:8722",
      "headers": { "Authorization": "Bearer <TWICESHY_TOKEN>" }
    }
  }
}
```

The agent then calls `search_experience` / `get_experience` on demand, and
`record_experience` to propose new records (returned as a ready-to-PR markdown
draft — propose-only, ADR-0008).

## Verify (post-deploy)

- [ ] `serve` logs `indexed N records; listening on :8722`.
- [ ] An authenticated MCP `search_experience` returns hits; an unauthenticated
      request gets 401.
- [ ] Oversized / abusive requests are bounded (rate limit, body cap — #0013).

## Nightly validation driver (issue 0043, ADR-0013 §A1/§2)

`scripts/scheduled-validate.sh` runs the autonomous loop on the **brain**: it
intakes queued outcome reports, runs `promote` + `adapt` (judge-gated), batches
the whole night into ONE commit, and opens ONE PR — the **held queue / veto
window**. A later nightly run auto-merges that PR once the soak
(`TWICESHY_SOAK_SECONDS`, default 48h) has elapsed and the PR is still open
(**closing the PR vetoes the batch**) and green. An anomaly halt (promote/adapt
exit 3, #0037) is held for review, never auto-merged. `TWICESHY_PAUSE=1`
short-circuits the whole run before any mutation.

**Dedicated clone (not a working checkout):** the script `git reset --hard`s, so
point `TWICESHY_REPO` at a clone used only by the driver (default
`/home/ori/twiceshy-validate`), exactly like the importer's
`/home/ori/twiceshy-import`.

**Queue wiring (#0042):** set the SAME directory for both sides, or intake is a
no-op — `serve -report-queue <dir>` (the server enqueues there) and
`TWICESHY_REPORT_QUEUE=<dir>` for the driver (it drains there). e.g.
`/home/ori/.local/share/twiceshy/report-queue`.

**Operator step — enable the timer** (like `twiceshy-import.timer`):

```sh
# 1. dedicated clone
git clone <forgejo>/claude/twiceshy.git /home/ori/twiceshy-validate
# 2. secrets + knobs (0600; NOT in the repo)
install -Dm600 /dev/stdin /home/ori/.config/twiceshy/validate.env <<'ENV'
TWICESHY_JUDGE_URL=http://localhost:8723
TWICESHY_JUDGE_MODEL=gpt-oss:20b
TWICESHY_DRAFTER_MODEL=qwen2.5-coder
TWICESHY_REPORT_QUEUE=/home/ori/.local/share/twiceshy/report-queue
TWICESHY_ALERT_URL=https://ntfy.example/twiceshy-alerts
NTFY_URL=https://ntfy.example/twiceshy
# TWICESHY_SOAK_SECONDS=172800   # 48h veto window (default)
# TWICESHY_AUTOMERGE=1           # auto-merge soaked, non-anomalous PRs on green (default)
# TWICESHY_PAUSE=1               # emergency stop
ENV
# 3. dry-run first (builds + runs locally, never pushes/merges)
TWICESHY_VALIDATE_DRYRUN=1 scripts/scheduled-validate.sh
# 4. install + enable the timer
sudo cp scripts/twiceshy-validate.service scripts/twiceshy-validate.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now twiceshy-validate.timer
```

**Sandbox-orphan reaping (#0052).** Each `promote` / `adapt` run now sweeps any
containers/volumes a crashed prior run leaked (label `twiceshy.repro`) **before**
it walks the corpus — so the nightly timer self-cleans. `-effect` dry-runs skip
the sweep (they delete nothing). For a host that runs the loop infrequently, add
a periodic out-of-band sweep (the Reaper is idempotent):

```sh
# belt-and-suspenders: prune orphaned repro sandboxes on a short timer
docker ps -aq  --filter label=twiceshy.repro | xargs -r docker rm -f
docker volume ls -q --filter label=twiceshy.repro | xargs -r docker volume rm -f
```

Pause: `TWICESHY_PAUSE=1` in `validate.env`, or
`sudo systemctl disable --now twiceshy-validate.timer`.

## Daily audit routine (issue 0044)

`scripts/daily-audit.sh` is the morning second-opinion check on the **brain**:
it reads the newest `runs/*-promote.json` manifest from the validate clone,
re-judges each `promoted` record with a high-reasoning auditor (default Opus
4.8 via `claude -p`), queues `audit-disagreement` counter-reports for
disagreements (`twiceshy report` → `intake-reports` → `adapt` demotes/flags),
and posts an ntfy digest listing each promotion with AGREE/DISAGREE. ADR-0013's
escape hatch when the overnight judge may have been compromised.

**Queue wiring:** `TWICESHY_REPORT_QUEUE` must be the **same** directory as
`serve -report-queue` and the validation driver's `TWICESHY_REPORT_QUEUE`, or
queued disputes never reach adapt.

**Operator step — enable the timer** (after `twiceshy-validate.timer`):

```sh
install -Dm600 /dev/stdin /home/ori/.config/twiceshy/audit.env <<'ENV'
TWICESHY_REPORT_QUEUE=/home/ori/.local/share/twiceshy/report-queue
AUDIT_CMD=claude -p
AUDIT_MODEL=claude-opus-4-8
NTFY_URL=https://ntfy.example/twiceshy
# TWICESHY_PAUSE=1
# TWICESHY_AUDIT_DRYRUN=1
ENV
# dry-run first (audits + digest, never queues or notifies)
TWICESHY_AUDIT_DRYRUN=1 scripts/daily-audit.sh
sudo cp scripts/twiceshy-audit.service scripts/twiceshy-audit.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now twiceshy-audit.timer
```

Pause: `TWICESHY_PAUSE=1` in `audit.env`, or
`sudo systemctl disable --now twiceshy-audit.timer`.

### Gold-set flywheel (#0058)

A daily-audit **disagreement** (auditor rejects what the overnight judge
approved) is a labelled judge miss. Capture it as a new gold case, then
re-measure the prompt:

```sh
twiceshy gold-add \
  -record experience/2026/<audit-miss>.md \
  -id Gnn \
  -mode <mode> \
  -checks <check> \
  -rationale "<why the auditor is right>" \
  -append
twiceshy judge-eval            # optionally -confirm to promote a winning variant
```

Each loop grows `internal/judgeeval/gold.yaml` from real production misses so
the judge eval tracks what actually slips through.

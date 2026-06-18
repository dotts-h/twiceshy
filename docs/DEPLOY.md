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

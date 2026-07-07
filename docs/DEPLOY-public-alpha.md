# DEPLOY-public-alpha.md — bringing up the hosted public alpha

Runbook for standing up the public alpha instance described in
[ADR-0030](adr/ADR-0030-public-alpha-hosted-multitenant-mcp.md) and issue
[#0129](issues/0129-off-homelab-public-deployment-isolated-host-tls-own-corpus-clone-and-secrets.md):
one isolated host, its own corpus clone and token store, TLS, and the
watchdog/backup set. Nothing on this host may reach the LAN
(ADR-0030's blast-radius rule) — the only data link is a **pull**: the corpus
sync in the other direction.

## DECIDED (2026-07-07) — the instance is live

1. **Provider: Hetzner Cloud.** The CX22 this runbook originally suggested no
   longer exists — its successor is **CX23** (2 vCPU / 4 GB / 40 GB, ~€6.6/mo
   gross). Live instance: `twiceshy-alpha` (Debian 13, fsn1, Hetzner cloud
   firewall allowing only 22/80/443 inbound). Any Docker-capable VPS still
   works; nothing below is Hetzner-specific.
2. **Domain: `twiceshy.app`** (Cloudflare Registrar; `.dev`/`.com` were
   taken). DNS on Cloudflare, **DNS-only (not proxied)** so Caddy's HTTP-01
   ACME works; A + AAAA for apex/`www`/`api`.
3. **Both git repos stay PRIVATE** (ADR-0030 names the hosted corpus + tokens
   as the controllable surface — the corpus repo is the product). Earlier
   drafts of this runbook assumed a public corpus repo; that was drift,
   corrected throughout: the corpus pulls with a **dedicated read-only
   Forgejo account** (`corpus-pull`, collaborator on `twiceshy-corpus` ONLY —
   verified its token cannot read the engine repo), and the engine source
   ships to the host as a one-way **git bundle** pushed from the LAN side.
   The host clones nothing anonymously and holds no LAN-wide credential.

## Architecture recap

- Host: one small VPS (2 vCPU / 2+ GB RAM / 40 GB disk, Debian 12+), Docker +
  the `docker compose` plugin.
- `deploy/public-alpha/compose.yaml` — two services:
  - `twiceshy` — this repo's `Dockerfile` (distroless nonroot), its **own**
    operator token + a fresh SQLite index in the `twiceshy-data` volume (NOT
    the LAN instance's db — tokens/telemetry here are public-instance state).
    `TWICESHY_SIGNUP=1` is fixed on for this deployment.
  - `caddy` — TLS via Let's Encrypt, serves `web/landing/` as a static root
    (`/`, `/docs`, `/terms`), reverse-proxies `/signup`, `/statz`, and
    everything else non-static (the MCP endpoint) to `twiceshy:8722`.
- Corpus link: **pull-only**. `corpus-refresh.sh` (+ `.service`/`.timer`) on
  the host pulls the (private) corpus repo
  (`https://git.radulescu.app/claude/twiceshy-corpus.git`, reachable via the
  NAS's Cloudflare tunnel) over HTTPS on a timer as the scoped `corpus-pull`
  account, mirrors it into the `twiceshy-data` volume, and SIGHUP-reloads the
  server in place (#0060). The credential lives in
  `/etc/twiceshy/corpus-pull.env` (root:twiceshy 0640), injected into the
  service via a systemd drop-in; it can read that one repo and nothing else.
  No SSH key to the LAN exists anywhere on this host.

## Prerequisites

- A VPS/VM per the DECIDED section above, with a public IPv4/IPv6.
- Docker Engine + the `docker compose` plugin installed
  (`curl -fsSL https://get.docker.com | sh`).
- DNS: `A`/`AAAA` records for both hostnames pointing at the host (Caddy needs
  them resolvable **before** it can issue Let's Encrypt certs):

  ```
  twiceshy.app.      A     <host-ip>
  www.twiceshy.app.  A     <host-ip>
  api.twiceshy.app.  A     <host-ip>
  ```

- Ports **80** and **443** open inbound (Let's Encrypt HTTP-01 + the site
  itself); no other port needs to be public — `twiceshy` is `expose`d
  internally only, never `ports:`-published.
- A dedicated, unprivileged service account for the corpus-refresh timer, in
  the `docker` group so it can run `docker` without `sudo`:

  ```sh
  sudo useradd -r -m -s /usr/sbin/nologin twiceshy
  sudo usermod -aG docker twiceshy
  ```

## Bring-up

1. **Ship the engine source** (builds the image; the repo is private, so the
   LAN side pushes a bundle — the host never holds an engine credential).
   On the brain:

   ```sh
   git -C ~/twiceshy bundle create /tmp/twiceshy.bundle main
   scp /tmp/twiceshy.bundle root@<host>:/opt/twiceshy-public-alpha/
   ```

   On the host:

   ```sh
   sudo mkdir -p /opt/twiceshy-public-alpha
   cd /opt/twiceshy-public-alpha
   git clone -b main twiceshy.bundle repo && rm twiceshy.bundle
   sudo chown -R root:twiceshy /opt/twiceshy-public-alpha && sudo chmod -R g+rX /opt/twiceshy-public-alpha
   cd repo/deploy/public-alpha
   ```

   To update later: create a fresh bundle on the brain, scp it over, and
   `git -C /opt/twiceshy-public-alpha/repo pull /path/to/twiceshy.bundle main`.

2. **Domain check.** `Caddyfile` in the repo carries the real domains
   (`twiceshy.app` / `api.twiceshy.app`) and ACME email as deployed — if you
   are standing up a different host/domain, substitute here.

3. **Secrets + knobs.** Generate the operator token and copy the env template:

   ```sh
   cp .env.example .env
   chmod 600 .env
   printf 'TWICESHY_TOKEN=%s\n' "$(openssl rand -hex 32)" >> .env
   # optionally also set TWICESHY_ALERT_URL / NTFY_TOKEN in .env (topic-qualified — #0093)
   ```

   Keep the printed `TWICESHY_TOKEN` value — it is the operator credential
   (full pull access + `GET /statz`); it is not shown again. Also set
   `TWICESHY_TRUSTED_PROXIES` to the compose network's subnet so the signup
   per-IP cap keys on the real visitor IP behind Caddy, not the proxy's (#0131).

4. **Build + start:**

   ```sh
   docker compose build
   docker compose up -d
   docker compose ps        # both containers should show "healthy"/"running"
   ```

   The first `twiceshy` start indexes an **empty** corpus (nothing has been
   pulled yet), so `/readyz` will read 503 until step 5 completes — this is
   expected, not a failure.

5. **Seed the corpus** (first pull, before the timer takes over). First set up
   the scoped credential — on the NAS, create the read-only account once
   (`FORGEJO_ADMIN` password in the secrets store may be stale; the container
   CLI always works):

   ```sh
   docker exec -u 1000 forgejo forgejo admin user create --username corpus-pull \
     --email corpus-pull@radulescu.app --random-password --must-change-password=false
   docker exec -u 1000 forgejo forgejo admin user generate-access-token \
     --username corpus-pull --token-name vps-corpus-pull --scopes read:repository
   # then add corpus-pull as a READ collaborator on claude/twiceshy-corpus (API or UI)
   ```

   On the host, store it and seed:

   ```sh
   sudo install -d -m 750 -o root -g twiceshy /etc/twiceshy
   # /etc/twiceshy/corpus-pull.env (root:twiceshy 0640):
   #   TWICESHY_CORPUS_REPO=https://corpus-pull:<token>@git.radulescu.app/claude/twiceshy-corpus.git
   sudo -u twiceshy git clone "https://corpus-pull:<token>@git.radulescu.app/claude/twiceshy-corpus.git" \
     /opt/twiceshy-public-alpha/corpus-src
   sudo -u twiceshy env TWICESHY_CORPUS_CLONE=/opt/twiceshy-public-alpha/corpus-src \
     TWICESHY_CORPUS_REPO="https://corpus-pull:<token>@git.radulescu.app/claude/twiceshy-corpus.git" \
     ./corpus-refresh.sh
   ```

   `corpus-refresh.sh` mirrors `experience/` into the `twiceshy-data` volume
   and SIGHUP-reloads the server; `docker compose ps` should now show
   `twiceshy` healthy with a non-zero record count.

   **Volume-permission trap (exp-0004's lesson, hit again here):** a freshly
   created named volume is root-owned, and the distroless service runs as uid
   65532 — SQLite fails with `unable to open database file (14)` until you
   `docker run --rm -v twiceshy-data:/data alpine chown -R 65532:65532 /data`.
   Verify as the target UID (`docker run --user 65532 … touch /data/.t`),
   never as root.

   The timer picks up the credential via a systemd drop-in
   (`/etc/systemd/system/corpus-refresh.service.d/token.conf`):

   ```ini
   [Service]
   EnvironmentFile=/etc/twiceshy/corpus-pull.env
   ```

6. **Install the corpus-refresh timer** (see below) so subsequent corpus
   updates on the LAN's `main` reach this host automatically.

## Smoke tests

Run these after step 5, with the real domain substituted for the
placeholders (mirrors the PR #512/#516 manual verification: signup → token →
MCP initialize → search returns cards; bad token 401):

```sh
# 1. liveness / readiness (unauthenticated by design)
curl -fsS https://api.twiceshy.app/healthz
curl -fsS https://api.twiceshy.app/readyz   # {"status":"ready","records":N}

# 2. signup roundtrip (same-origin path on the marketing domain)
curl -fsS -X POST https://twiceshy.app/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"smoke-test@example.com","accept_terms":true}'
# -> {"token":"tok_…", …} — export it below (never paste the literal value into a command)
export TWICESHY_TOKEN=tok_your_token_here

# 3. MCP initialize (captures the Mcp-Session-Id header the SDK issues)
SESSION=$(curl -fsS -D - -o /dev/null https://api.twiceshy.app/ \
  -H "Authorization: Bearer $TWICESHY_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke-test","version":"0"}}}' \
  | tr -d '\r' | sed -n 's/^[Mm]cp-[Ss]ession-[Ii]d: //p')

# 4. search_experience returns cards
curl -fsS https://api.twiceshy.app/ \
  -H "Authorization: Bearer $TWICESHY_TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -H 'MCP-Protocol-Version: 2025-06-18' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_experience","arguments":{"query":"sqlite fts5"}}}'

# 5. bad token -> 401
export WRONG_TOKEN=anything
curl -s -o /dev/null -w '%{http_code}\n' https://api.twiceshy.app/ \
  -H "Authorization: Bearer $WRONG_TOKEN"   # expect 401
```

- [ ] `/healthz` 200, `/readyz` 200 with `records > 0`.
- [ ] Signup returns a `tok_…` token (once per email/IP/day — see the XFF
      caveat below).
- [ ] `initialize` returns a session id; `search_experience` returns hits with
      the new token.
- [ ] A bad/missing token gets 401.
- [ ] `curl -fsS https://twiceshy.app/docs` and `/terms` serve the landing
      page's docs/terms pages (once PR #514 has merged the corresponding
      `docs.html`/`terms.html` — until then these 404, which is expected).

## Install the corpus-refresh timer

```sh
sudo cp deploy/public-alpha/corpus-refresh.service deploy/public-alpha/corpus-refresh.timer \
  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now corpus-refresh.timer
systemctl list-timers corpus-refresh.timer   # confirm the next run is scheduled
```

Runs every 30 minutes (`OnCalendar=*:0/30`, randomized ±2m), same cadence as
the LAN's `twiceshy-corpus-sync.timer`. A tick is a no-op if the corpus tree
SHA already matches the volume marker. Pause by disabling the timer:
`sudo systemctl disable --now corpus-refresh.timer`.

## Monitoring

- **Uptime (external):** point UptimeRobot (or similar) at
  `https://api.twiceshy.app/healthz` and `https://api.twiceshy.app/readyz` —
  both bypass auth **by design** (ADR-0030 §consequences) so an external
  monitor never needs a token. Alert on `/healthz` for outage, `/readyz` for
  "serving but empty".
- **Disk space (85% threshold, the brain's ntfy disk-alarm pattern adapted):**

  ```sh
  # /etc/cron.d/twiceshy-disk-alarm (or a systemd timer, same idea)
  */30 * * * * root df -P / | awk 'NR==2 && $5+0 >= 85 {print $5" used"}' \
    | xargs -r -I{} curl -fsS -m 10 -d "twiceshy-disk: {} on public-alpha" "$TWICESHY_ALERT_URL"
  ```

- **Application alerts:** `serve`'s own `TWICESHY_ALERT_URL` (fatal index
  build/bind failures) and `corpus-refresh.sh`'s alerts (failed reload/mirror)
  both fire to the same ntfy topic set in `.env` — no separate wiring needed.
- **Backups (daily, sqlite state, offsite pull):**

  The VPS cron takes a crash-consistent snapshot with `sqlite3 .backup`
  (transactionally safe against the live db, unlike a plain `cp`). As
  deployed, verbatim:

  ```sh
  # Nightly crash-consistent sqlite snapshot INTO the volume (03:00); the brain
  # pulls it offsite at 04:30. sqlite .backup is transactionally safe against a
  # live db, unlike a plain cp of the file.
  0 3 * * * root docker run --rm -v twiceshy-data:/data alpine:3.20 sh -c 'apk add -q sqlite && sqlite3 /data/twiceshy.db ".backup /data/backup-$(date +\%Y\%m\%d).db" && find /data -maxdepth 1 -name "backup-*.db" -mtime +7 -delete'
  ```

  The brain pulls the latest snapshot offsite nightly at 04:30
  (`twiceshy-backup-pull.timer`), runs `PRAGMA integrity_check` on each pull,
  keeps 14 days of retention; the restore path is tested.

  > [!WARNING]
  > **DURABLE TENANT REGISTRY WARNING**
  > The `twiceshy-data` volume's database (`twiceshy.db`) holds the **TENANT REGISTRY** (hash-only token credentials). Deleting or recreating the volume or the database file revokes every issued token irrecoverably; it is **NOT** a rebuildable cache. Cache rebuilds (via `twiceshy index`) do NOT touch or restore these tables ([ADR-0034](adr/ADR-0034-tenant-registry-is-not-derived-state.md)).

## Known limitation at launch — signup rate limit behind the proxy (#0131)

The signup per-IP daily cap (3/day) keys on `RemoteAddr`, which behind this
Caddy reverse proxy is **always Caddy's own IP** — so until trusted-proxy
`X-Forwarded-For` handling lands server-side (tracked in #0131), the cap is
effectively **global-3/day for the whole public instance**, not per visitor.
Caddy forwards the real client IP via `X-Forwarded-For` already (the default
`reverse_proxy` behavior); the server just does not read it yet. Acceptable
for an alpha launch; do not represent the cap as per-visitor until #0131
ships.

## Teardown

```sh
cd /opt/twiceshy-public-alpha/repo/deploy/public-alpha
docker compose down                 # stops + removes both containers
sudo systemctl disable --now corpus-refresh.timer
sudo rm /etc/systemd/system/corpus-refresh.service /etc/systemd/system/corpus-refresh.timer
sudo systemctl daemon-reload

# Only if the instance is being decommissioned for good (destroys the token
# store, corpus clone, and any local backups — irreversible):
docker volume rm twiceshy-data twiceshy-caddy-data twiceshy-caddy-config
sudo rm -rf /opt/twiceshy-public-alpha
```

Terminate/reclaim the VPS itself out-of-band with the chosen provider once the
above is confirmed clean.

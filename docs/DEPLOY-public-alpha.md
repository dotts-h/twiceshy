# DEPLOY-public-alpha.md — bringing up the hosted public alpha

Runbook for standing up the public alpha instance described in
[ADR-0030](adr/ADR-0030-public-alpha-hosted-multitenant-mcp.md) and issue
[#0129](issues/0129-off-homelab-public-deployment-isolated-host-tls-own-corpus-clone-and-secrets.md):
one isolated host, its own corpus clone and token store, TLS, and the
watchdog/backup set. Nothing on this host may reach the LAN
(ADR-0030's blast-radius rule) — the only data link is a **pull**: the corpus
sync in the other direction.

## OPEN DECISIONS — Horia to make before bring-up

1. **Provider.** No provider is chosen yet. Default suggestion: **Hetzner
   CX22** (2 vCPU / 4 GB / 40 GB NVMe, ~€4.35/mo at time of writing) — cheap,
   EU-based, has a Debian 12 image. Any Docker-capable VPS/VM works equally
   well (the stack is two containers behind Caddy); nothing below is
   Hetzner-specific.
2. **Domain.** No domain is chosen yet. This runbook and the shipped
   `Caddyfile` use **`twiceshy.dev`** (marketing/signup) and
   **`api.twiceshy.dev`** (the MCP endpoint) as **placeholders** — substitute
   the real domain(s) everywhere below and in `deploy/public-alpha/Caddyfile`
   before deploy.

Everything else in this document is mechanical once those two are decided.

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
  the host clones/pulls the public corpus repo
  (`https://git.radulescu.app/claude/twiceshy-corpus.git`, already public via
  the NAS's Cloudflare tunnel) over HTTPS on a timer, mirrors it into the
  `twiceshy-data` volume, and SIGHUP-reloads the server in place (#0060). No
  SSH key to the LAN exists anywhere on this host.

## Prerequisites

- A VPS/VM per the OPEN DECISION above, with a public IPv4/IPv6.
- Docker Engine + the `docker compose` plugin installed
  (`curl -fsSL https://get.docker.com | sh`).
- DNS: `A`/`AAAA` records for both hostnames pointing at the host (Caddy needs
  them resolvable **before** it can issue Let's Encrypt certs):

  ```
  twiceshy.dev.      A     <host-ip>
  www.twiceshy.dev.  A     <host-ip>
  api.twiceshy.dev.  A     <host-ip>
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

1. **Clone the engine repo** (builds the image; this is the AGPL source, not
   the corpus):

   ```sh
   sudo mkdir -p /opt/twiceshy-public-alpha
   sudo chown "$USER" /opt/twiceshy-public-alpha
   git clone https://git.radulescu.app/claude/twiceshy.git /opt/twiceshy-public-alpha/repo
   cd /opt/twiceshy-public-alpha/repo/deploy/public-alpha
   ```

2. **Domain substitution.** Replace the `twiceshy.dev` / `api.twiceshy.dev`
   placeholders in `Caddyfile` with the real domain from the OPEN DECISION
   above (including the ACME `email` in the global options block).

3. **Secrets + knobs.** Generate the operator token and copy the env template:

   ```sh
   cp .env.example .env
   chmod 600 .env
   printf 'TWICESHY_TOKEN=%s\n' "$(openssl rand -hex 32)" >> .env
   # optionally also set TWICESHY_ALERT_URL / NTFY_TOKEN in .env (topic-qualified — #0093)
   ```

   Keep the printed `TWICESHY_TOKEN` value — it is the operator credential
   (full pull access + `GET /statz`); it is not shown again.

4. **Build + start:**

   ```sh
   docker compose build
   docker compose up -d
   docker compose ps        # both containers should show "healthy"/"running"
   ```

   The first `twiceshy` start indexes an **empty** corpus (nothing has been
   pulled yet), so `/readyz` will read 503 until step 5 completes — this is
   expected, not a failure.

5. **Seed the corpus** (first pull, before the timer takes over):

   ```sh
   sudo -u twiceshy git clone https://git.radulescu.app/claude/twiceshy-corpus.git \
     /opt/twiceshy-public-alpha/corpus-src
   sudo -u twiceshy TWICESHY_CORPUS_CLONE=/opt/twiceshy-public-alpha/corpus-src \
     ./corpus-refresh.sh
   ```

   `corpus-refresh.sh` mirrors `experience/` into the `twiceshy-data` volume
   and SIGHUP-reloads the server; `docker compose ps` should now show
   `twiceshy` healthy with a non-zero record count.

6. **Install the corpus-refresh timer** (see below) so subsequent corpus
   updates on the LAN's `main` reach this host automatically.

## Smoke tests

Run these after step 5, with the real domain substituted for the
placeholders (mirrors the PR #512/#516 manual verification: signup → token →
MCP initialize → search returns cards; bad token 401):

```sh
# 1. liveness / readiness (unauthenticated by design)
curl -fsS https://api.twiceshy.dev/healthz
curl -fsS https://api.twiceshy.dev/readyz   # {"status":"ready","records":N}

# 2. signup roundtrip (same-origin path on the marketing domain)
curl -fsS -X POST https://twiceshy.dev/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"smoke-test@example.com","accept_terms":true}'
# -> {"token":"tok_…", …} — export it below (never paste the literal value into a command)
export TWICESHY_TOKEN=tok_your_token_here

# 3. MCP initialize (captures the Mcp-Session-Id header the SDK issues)
SESSION=$(curl -fsS -D - -o /dev/null https://api.twiceshy.dev/ \
  -H "Authorization: Bearer $TWICESHY_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke-test","version":"0"}}}' \
  | tr -d '\r' | sed -n 's/^[Mm]cp-[Ss]ession-[Ii]d: //p')

# 4. search_experience returns cards
curl -fsS https://api.twiceshy.dev/ \
  -H "Authorization: Bearer $TWICESHY_TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -H 'MCP-Protocol-Version: 2025-06-18' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_experience","arguments":{"query":"sqlite fts5"}}}'

# 5. bad token -> 401
export WRONG_TOKEN=anything
curl -s -o /dev/null -w '%{http_code}\n' https://api.twiceshy.dev/ \
  -H "Authorization: Bearer $WRONG_TOKEN"   # expect 401
```

- [ ] `/healthz` 200, `/readyz` 200 with `records > 0`.
- [ ] Signup returns a `tok_…` token (once per email/IP/day — see the XFF
      caveat below).
- [ ] `initialize` returns a session id; `search_experience` returns hits with
      the new token.
- [ ] A bad/missing token gets 401.
- [ ] `curl -fsS https://twiceshy.dev/docs` and `/terms` serve the landing
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
  `https://api.twiceshy.dev/healthz` and `https://api.twiceshy.dev/readyz` —
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
- **Backups (daily, sqlite state):**

  ```sh
  # /etc/cron.d/twiceshy-backup — snapshot the token/telemetry db INTO the
  # same volume (cheap, but co-located — see the open offsite question below).
  0 3 * * * twiceshy docker run --rm -v twiceshy-data:/data alpine \
    sh -c 'cp /data/twiceshy.db "/data/backup-$(date +\%Y\%m\%d).db" && \
           find /data -maxdepth 1 -name "backup-*.db" -mtime +7 -delete'
  ```

  **Open question (not resolved here):** this backs up onto the same volume,
  so it does not survive a lost/corrupted disk. An offsite copy (e.g.
  `scp`-less: push to an S3-compatible bucket, or pull from the brain like the
  corpus sync but in reverse) is deliberately left as a follow-up — flag it
  before this instance holds tokens anyone would be upset to lose.

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

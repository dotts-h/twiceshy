# Consuming twiceshy — client onboarding

> **Internal reference.** twiceshy is a **hosted service** (the repo is private;
> consumers never clone it). Consumers connect their agent to *our* running
> instance on the NAS/brain. This doc is how we wire a consumer up today; if/when
> we go public, a web page delivers the same steps + a per-tenant token. See
> [ADR-0001](adr/ADR-0001-architecture.md), [DEPLOY.md](DEPLOY.md) (server side).

A deployed twiceshy that no agent knows to call is dead weight. Making it actually
get consulted is **two consumer-side config steps**, and neither ships in the repo:

## What a consumer needs (issued by us)

| Thing | Example | Notes |
|-------|---------|-------|
| Endpoint URL | `http://192.168.50.244:8722` | LAN-only today; off-LAN access is Tier-B (#0010) |
| Bearer token | `TWICESHY_TOKEN` | one per consumer; never commit it (ours lives in `secrets.env`) |

## Step 1 — register the MCP server

twiceshy speaks **streamable HTTP** MCP (single endpoint; not SSE — exp-0003).
Pick your client. (The Claude Code and Cursor recipes are validated end-to-end; the
Antigravity recipe uses the binary-verified config format — verify it with `/mcp` in
the agy TUI.)

**Claude Code** (user scope → available in every session, any directory):
```sh
claude mcp add twiceshy --scope user --transport http \
  http://192.168.50.244:8722/ --header "Authorization: Bearer $TWICESHY_TOKEN"
```

**Cursor** (`cursor-agent`) — `~/.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "twiceshy": {
      "url": "http://192.168.50.244:8722/",
      "headers": { "Authorization": "Bearer ${TWICESHY_TOKEN}" }
    }
  }
}
```
Then approve it: `cursor-agent mcp enable twiceshy` (verify: `cursor-agent mcp list-tools twiceshy`).

**Antigravity** (`agy`) — `~/.gemini/config/mcp_config.json`:
```json
{
  "mcpServers": {
    "twiceshy": {
      "httpUrl": "http://192.168.50.244:8722/",
      "headers": { "Authorization": "Bearer ${TWICESHY_TOKEN}" }
    }
  }
}
```
Verify in the agy TUI with `/mcp` (lists active servers + their tools). Note: MCP loads in
Antigravity's **interactive** TUI/IDE, not in headless `agy -p` print mode — so consult twiceshy
from an interactive agy session. (If your build doesn't expand `${TWICESHY_TOKEN}` in headers,
inline the literal token.)

## Step 2 — harden the affordance (so the agent reaches for it)

Registering the server is not enough — the agent must *know to consult it*. Put a
short pointer in the agent's **always-loaded** context (paid once, cache-friendly,
zero per-prompt cost; the agent decides *when* to call, using judgment):

- **Claude Code:** a section in `~/.claude/CLAUDE.md` (global → every project).
- **A repo:** an `AGENTS.md` section (see this repo's `AGENTS.md`).
- **Anything else:** the system prompt.

Drop-in snippet:
```markdown
## Experience store — twiceshy (consult it, on demand)
The MCP server `twiceshy` holds validated engineering traps/fixes/dead-ends.
Before debugging an unfamiliar error or retrying an approach that just failed,
call `search_experience` (verbatim error / short symptom); `get_experience` for
the full record; `record_experience` to propose a new lesson. An empty result is
a valid answer — don't force a near-miss.
```

**Do not restate the detailed usage rules.** The *when/how* lives in the MCP tool
descriptions, which the agent sees automatically on connect — single-source it
(one fact, one home). The snippet above is just the pointer that makes the tool
impossible to miss.

## Push channel (optional, deferred)

The `UserPromptSubmit` push hook ([PUSH_HOOK.md](PUSH_HOOK.md)) injects trap cards
on every prompt. It is **deferred by design**: the relevance floor is not yet
precise enough (it injects on near-any prompt), and the pull path above is
self-targeting and cheaper. Do not enable push until the #0005 eval shows pull
alone leaves traps on the table.

# twiceshy hooks

Claude Code hook clients for twiceshy's two agent-facing channels. Both are
**fail-open** (any error → `exit 0`, never blocking the agent) and read their config
from the environment.

| hook | event | what it does |
|------|-------|--------------|
| [`twiceshy-push.sh`](twiceshy-push.sh) | `UserPromptSubmit` | pull channel: POSTs the prompt to `/push` and injects matching trap cards as `additionalContext` (ADR-0001 §5). |
| [`twiceshy-error-pull.sh`](twiceshy-error-pull.sh) | `PostToolUse` | **error-scoped pull (#0087, prototype):** on the *second* appearance of an error signature in tool output, queries `/push` with the verbatim error line — the reliable, high-signal trigger that per-prompt push and judgment-based pull both miss. Dedupes per session+signature; `TWICESHY_ERROR_PULL_ON_FIRST=1` fires on the first occurrence. |
| [`session-retro.sh`](session-retro.sh) | `SessionEnd` | capture channel: ships the bounded session transcript to `/retro` so the off-pool analyzer can extract quarantined trap drafts (#0065, [ADR-0018](../docs/adr/ADR-0018-session-retro-capture.md)). |

## Environment

| var | default | used by |
|-----|---------|---------|
| `TWICESHY_TOKEN` | *(required)* | both — bearer token; absent → the hook no-ops |
| `TWICESHY_URL` | `http://192.168.50.244:8722` | both — server base URL |
| `TWICESHY_RETRO_MAX_BYTES` | `200000` | `session-retro` — transcript tail size, kept under the 256 KiB body cap |

`session-retro.sh` also screens the transcript client-side with `twiceshy screen`
(the same tested content screen the server uses) when the `twiceshy` binary is on
`PATH`: a transcript that trips the **secret** check is not sent at all. The server
re-screens at the edge regardless, so an absent binary is safe.

## Registration (settings.json)

Hooks register in `~/.claude/settings.json` (user-wide), `.claude/settings.json`
(project, shareable), or `.claude/settings.local.json` (project, local). Point the
command at this repo's absolute path:

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "hooks": [
          { "type": "command", "command": "/home/ori/twiceshy/hooks/session-retro.sh" }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "/home/ori/twiceshy/hooks/twiceshy-push.sh" }
        ]
      }
    ]
  }
}
```

`SessionEnd` fires once per session. To capture only true exits (not `/clear` or
`/resume`), add a `"matcher"` on the end reason, e.g. `"matcher": "logout|prompt_input_exit"`.
The server's quarantine + dedup make capturing every reason harmless, so the default
(all reasons) is fine for a single-tenant deployment.

## Server side

`session-retro.sh` only delivers transcripts; nothing is analyzed on the request
path. Run the off-pool drain to turn spooled transcripts into quarantined drafts:

```sh
twiceshy serve -retro-queue /var/lib/twiceshy/retro ...      # enable the endpoint
twiceshy retro-intake -corpus <repo> -queue /var/lib/twiceshy/retro \
  -analyzer-model gpt-oss:20b                                 # drain (TWICESHY_RETRO_URL set)
```

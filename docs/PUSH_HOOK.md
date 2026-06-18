# Push hook — trap cards at prompt time

The **push channel** injects 1–3 validated trap cards into Claude Code when your
prompt matches a high-confidence experience record. The hook is **fail-open**: if
twiceshy is down, the token is wrong, or nothing clears the relevance floor, the
prompt proceeds unchanged — the hook never blocks you.

## Prerequisites

- `curl` and `jq` on `PATH`
- A running twiceshy server with `POST /push` deployed
- `TWICESHY_TOKEN` — bearer token the server accepts (same as MCP `Authorization`)

## Environment

| Variable | Required | Default |
|----------|----------|---------|
| `TWICESHY_TOKEN` | yes | — |
| `TWICESHY_URL` | no | `http://192.168.50.244:8722` |

Export the token in your shell profile or point Claude Code at a secrets file.
Example (adjust path to your install):

```bash
export TWICESHY_TOKEN="$(grep '^TWICESHY_TOKEN=' ~/.config/brain/secrets.env | cut -d= -f2-)"
export TWICESHY_URL="http://192.168.50.244:8722"   # optional
```

## Wire into Claude Code

Add a `UserPromptSubmit` hook in `~/.claude/settings.json`. The hook reads the
prompt JSON from stdin, POSTs `{"query":"<prompt>"}` to `$TWICESHY_URL/push`, and
on a match emits `additionalContext` trap cards.

Copy-paste snippet (replace the script path with your checkout):

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "TWICESHY_TOKEN=$TWICESHY_TOKEN TWICESHY_URL=$TWICESHY_URL bash /home/ori/twiceshy/hooks/twiceshy-push.sh"
          }
        ]
      }
    ]
  }
}
```

If you keep the token in a file instead of the environment, inline it in the
`command` string or wrap the script in a small launcher that sources your secrets.

## Behavior

1. Claude Code invokes the script with hook JSON on stdin (`prompt` field).
2. The script POSTs the prompt text to `/push` with `Authorization: Bearer …`.
3. **Match** (`count > 0`): stdout is hook output JSON with `additionalContext`
   set to the server-rendered trap cards.
4. **No match** (`count == 0`) or **any error** (missing deps, bad token,
   non-200, malformed JSON): stdout is empty, exit code 0.

Only **validated** records reach the push channel; quarantined records never
inject. Weak matches are suppressed by design — injecting nothing is correct.

## Manual smoke test

```bash
export TWICESHY_TOKEN="…"   # from your secrets file

# Expect hook JSON with non-empty additionalContext:
echo '{"prompt":"FTS5 syntax error near \".\""}' \
  | TWICESHY_TOKEN=$TWICESHY_TOKEN bash hooks/twiceshy-push.sh

# Expect no output, exit 0 (fail-open):
echo '{"prompt":"FTS5 syntax error near \".\""}' \
  | TWICESHY_TOKEN=bogus bash hooks/twiceshy-push.sh
```
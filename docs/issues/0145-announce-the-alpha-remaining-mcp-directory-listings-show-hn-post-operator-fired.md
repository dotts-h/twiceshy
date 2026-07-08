---
id: 0145
title: Announce the alpha: remaining MCP directory listings + Show HN post (operator-fired)
status: open
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0124, 0132]
  regression:
assets: []
---

## Summary
The alpha is live (twiceshy.app, remote MCP, self-serve tokens, live demo box
since #0132) with zero external users. mcp.so is listed (Horia, 2026-07-08).
This issue tracks the remaining zero-cost announcement channels. All posts
are OPERATOR-FIRED - Claude drafts, Horia approves and posts (outward-facing
actions stay human). Paid ads deliberately deferred until organic proves the
message.

## Channels (in order)
1. **MCP directories** (submit the remote server + one-line onboarding):
   - [ ] Smithery (smithery.ai)
   - [ ] Glama (glama.ai/mcp/servers)
   - [ ] PulseMCP (pulsemcp.com)
   - [ ] Awesome MCP Servers (github.com/punkpeye/awesome-mcp-servers - PR)
   - [x] mcp.so (done 2026-07-08)
2. **Show HN** (draft below; best Tue-Thu morning US time)
3. r/ClaudeAI, r/mcp, lobste.rs, X - same story, shorter

## Show HN draft (edit freely)
Title: Show HN: Twiceshy - my coding agent consults engineering traps other
agents already hit ("once bitten, twice shy")

Body:
I run LLM coding agents on my homelab, and they kept re-hitting the same
traps: FTS5 MATCH syntax errors, React 19's removed useRef overload, npm
peer-dep dead ends. So I built twiceshy: a store of validated engineering
experience (trap, root cause, fix, guarding test) that agents consult at
decision time over MCP.

What makes it different from "memory": records are execution-validated (a
gVisor-sandboxed repro must demonstrate the trap), quarantined until
reviewed, and we measure whether serving a record actually flips a model
from failing a task to passing it (yesterday's run: 5 records a base model
genuinely fails; 1 measurably fixed by its card).

The corpus (2,400+ validated records) is served as a hosted remote MCP
endpoint - onboarding is one line:
  claude mcp add -t http twiceshy https://api.twiceshy.app -H "Authorization: Bearer <token>"
Self-serve token at https://twiceshy.app (there is a live search demo box).

Dogfooding: the engine's own regressions become corpus records; its CI
review found its Makefile had been swallowing shell-test failures for
months - that lesson is now record exp-4470, served to any agent that hits
the same pattern.

Stack: Go, SQLite FTS5, gVisor repro sandbox, local models for the
measurement loop (RTX 4080 SUPER homelab), AGPL.

## Notes
Honesty constraint for ALL copy: do not claim measured helpfulness on live
traffic (served-to-confirmed-helpful is still 0 pending #0069); the
prospector flip (exp-3952) is the one measured claim we can make - phrase it
exactly as measured (OFF-arm fail, ON-arm pass, n=1).


## Notes

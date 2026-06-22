# ADR-0008: Persistence is a trusted-CLI concern — the MCP server never holds push credentials

- **Status:** **Superseded by [ADR-0019](ADR-0019-write-path-is-the-autonomous-validation-loop.md)** (2026-06-22)
  for §2–4 (the `twiceshy propose` + `internal/publish` / `Publisher` per-record write
  path — never implemented; overtaken by the ADR-0013/0016 autonomous loop + the
  ADR-0011 corpus-level import). Originally Accepted (2026-06-17). **§1 (server
  read-only, no forge creds) still holds** and is carried forward verbatim by ADR-0019.
- **Deciders:** horia
- **Related:** [ADR-0001 §5–6](ADR-0001-architecture.md) (pull channel + the
  quarantined-only write invariant — **locked**); [issue #3](../../) (Phase 3
  write path); [CONTEXT.md](../CONTEXT.md) (push/pull channel, quarantine, "a new
  experience record IS a pull request"); the seam is registered in
  [CONTRACTS.md](../CONTRACTS.md) per [ADR-0005](ADR-0005-stable-seams.md).

## Context

`record_experience` ([record.go](../../internal/server/record.go)) builds a
draft, dedups it ([ADR-0004]/[ADR-0007]), forces `status: quarantined`, and
**returns** the marshaled markdown plus an allocated `exp-NNNN` id. It does not
persist. Issue #3's open box is: "Persistence as a git branch + PR against the
corpus repo (one record per PR), **never a direct write to the validated
store.**"

The tempting implementation — have the MCP handler create the branch, push, and
open the PR — would give the **agent-facing, network-exposed service** write
credentials to the corpus repo. That collides with the exact threat model this
project is built around ([issue #3], the research §6): **MINJA shows query-only
attackers can talk an agent into proposing poisoned memory**, so "only my agent
writes" is not a defense. A service that both (a) accepts untrusted agent input
and (b) holds a push/PR token is a strictly larger attack surface than one that
only reads. The quarantine + human-merge gate still protects *validated* state,
but giving the request-handling service push power is privilege it does not need.

The corpus repo is already a git working tree wherever the operator runs
twiceshy (`-corpus`); git credentials and a forge already exist **in that
trusted local context** — the developer's machine, CI, or the session host. The
persistence step belongs there, not inside the server's request path.

## Decision

1. **The MCP server stays read-only over the network.** `record_experience` is
   unchanged: dedup, quarantine, return the draft markdown + id. No git, no push,
   no forge token in `internal/server`. The server's only write is the derived
   index it already owns.

2. **`twiceshy propose` is the persistence step, a trusted CLI subcommand.** It
   takes a quarantined draft (the markdown `record_experience` returns, via stdin
   or `-file`), and against the local corpus working tree: creates a branch
   (`quarantine/exp-NNNN-slug`), writes `experience/YYYY/NNNN-slug.md`, commits,
   pushes, and opens **one PR per record** via the forge API. It runs where git
   credentials already live; the token it needs can push branches and open PRs
   but **does not merge** — the PR is the trust boundary ([ADR-0001 §6]).

3. **Git is `os/exec`, the forge is `net/http` — no new dependencies.** Branch /
   commit / push shell out to the `git` already required to have a working tree;
   the PR call is a plain HTTP POST to the configured forge
   (`TWICESHY_FORGE_*` env). Both sit behind a small `publish.Publisher` seam so
   `propose` is unit-testable with a fake (no network, [ARCHITECTURE.md] "testable
   without a network"). The concrete git+forge implementation is the only place
   those edges are touched.

4. **The Stop-hook drafter calls `propose`, not a privileged server path.** The
   end-of-session drafter (issue #3; depends on Phase 2 plumbing, [#2]) drafts
   candidates, may call `search_experience`/`record_experience` to dedup, and
   shells `twiceshy propose` to open the PR — again, in the trusted local
   context. It is a separate, smaller change tracked after this one.

## Options

- **A — server-side publish** (handler creates the branch+PR). Rejected: gives
  the untrusted-input, network-facing service push/PR credentials for the corpus
  repo — the privilege escalation the cited threat model warns against — and
  forces the server to own a writable working tree and forge token. One-step for
  the agent, but the trust cost is not worth it.
- **B — trusted CLI (`twiceshy propose`) + Stop-hook (chosen).** The server
  never holds write creds; persistence runs where git already trusts the caller.
  Cleanly testable behind a `Publisher` seam, no new deps. Cost: two steps
  (propose is a distinct invocation), and the agent doesn't get the PR URL in the
  tool result — acceptable, since opening the PR is a human/trusted-context act
  by design.
- **C — a separate privileged "writer" sidecar service.** A second service that
  the server hands drafts to. Rejected as premature: it is option A's credential
  surface in a different process, with added deployment complexity, for no Phase-3
  benefit.

## Consequences

- The agent-facing surface cannot cause a push or a PR; only a trusted local
  invocation can. An injected/poisoned draft is inert until a human opens *and*
  merges its PR — quarantine end-to-end.
- `internal/publish` (the `Publisher` seam + git/forge edge) and the
  `twiceshy propose` subcommand are the implementation, test-first, in the next
  PR; `Publisher` joins the [CONTRACTS.md] seam registry.
- **Out of scope, deferred:** the promotion workflow (guard sandbox fail-to-pass
  + merge flips `validated`/`validated_at`) and supersede linkage on merge are
  Phase 4 / the doctors ([#4]); `propose` only creates the quarantined PR.
- Concurrent proposals can allocate the same `exp-NNNN` (NextID reads the index
  max). This is benign — two quarantined PRs collide visibly and a human resolves
  the id at merge — and is noted, not solved, here; durable allocation is a
  promotion-time concern.
- If no forge is configured, `propose` still creates the local branch+commit and
  prints the push/PR command, so the path degrades to "prepare the PR" rather
  than failing — keeping a first run useful offline ([ARCHITECTURE.md] failure
  modes).

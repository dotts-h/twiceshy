# ADR-0012: CI/CD trust posture — self-merge gate + an isolated runner for twiceshy CI

- **Status:** Accepted (deciders: claude, with horia's go-ahead for the CI/CD
  hardening sprint). The multi-tenant / Tier-B implications (Consequences) are
  flagged for revisit, not yet decided.
- **Related:** ADR-0001 (trust boundary = git/PR), ADR-0008 (the CLI holds
  credentials, not the server), ADR-0011 §4 (the gVisor execution sandbox — the
  same muscle, reused here). Grounding: SECURITY_ANALYSIS Facet 4 (supply chain).

## Context

twiceshy's branch protection requires 3 green checks and **0 approvals** — the
agent (claude) opens, CI-greens, and **self-merges** its own PRs. That autonomy is
deliberate, but it makes one fact load-bearing: **CI is the only gate.** Nothing
else stands between a change and `main`, so the gate's *completeness* and
*integrity* are the whole security story.

An audit of the CI runner found a critical exposure. The Forgejo runner (a docker
container on the brain VM, shared by ~6 repos) mounts the **host docker socket
read-write into every job** (`docker_host: automount`), with `valid_volumes:
['**']` and **unrestricted LAN egress** (live-proved reach to the NAS). So **any
code that runs in CI — a malicious dependency's `go test`, a compromised action —
is one step from root on the brain and the whole `192.168.50.0/24`.** Today only
claude + horia can open PRs (private repos), so it is bounded, but the
supply-chain vector is real, and it is the exact threat ADR-0011's harness exists
to contain.

The other repos on that runner (chat, brain-chat, …) **use the socket as their
deploy mechanism** (`docker build/push`, then `docker run` the prod container from
CI) and label `runs-on: docker`. **twiceshy is the only one that needs no docker**
— its CI is pure Go (lint / test / govulncheck / gitleaks). Runner config is
runner-global, so one runner cannot socket-off only twiceshy.

## Options considered

- **A — reconfigure the shared runner** (socket off / runsc). Rejected: breaks the
  5 trusted deploy repos whose CI *is* `docker …`, and risks 6 repos at once.
- **B — accept the exposure** (private repos, trusted authors). Rejected: the
  supply-chain vector is real now, and it contradicts the whole point of the sprint.
- **C — a dedicated, repo-scoped, hardened runner for twiceshy (chosen).** Additive
  (zero risk to the other repos), and the correct long-term shape: untrusted-code CI
  belongs on a least-privilege runner.

## Decision

1. **The self-merge gate is intentional; therefore the gate must be complete and
   tamper-resistant.** `make ci` is made to reproduce **all** required CI checks
   locally (lint + race-tests + coverage **+ govulncheck + gitleaks**), so "green
   locally" implies "green in CI" — closing the gap that let a secret-shaped
   literal pass `make ci` yet fail the required gitleaks gate.

2. **twiceshy CI runs on a dedicated, repo-scoped, hardened runner**
   (`brain-twiceshy-secure`, label `twiceshy-ci`): jobs run under **gVisor
   (runsc)**, with **no host docker socket** (`docker_host: "-"`), **no host bind
   mounts** (`valid_volumes: []`), and (follow-up) **LAN egress blocked except the
   Forgejo instance/registry**. The runner container holds the socket only to
   *spawn* jobs; the untrusted boundary — the job — cannot reach the host daemon or
   pivot onto the LAN. Additive: the shared socket-runner is untouched.

3. **The shared socket-runner keeps serving the trusted docker-deploy repos** —
   accepted *with rationale*: their socket use is first-party deploy automation by
   the sole authors, not untrusted third-party code. This is **not** acceptable
   once any repo on it takes external/multi-tenant contributions (see Consequences).

4. **Supply-chain integrity** (follow-up PRs, same sprint): pin third-party actions
   to commit SHAs and `govulncheck` to a version (not `@latest`); add `go mod
   verify`; require branches **up-to-date with `main`** before merge (a stale-green
   branch can break `main`).

## Consequences

- twiceshy's CI is isolated from the host: a hostile dep/action in CI is confined
  by gVisor with no socket and no host mounts. The biggest single risk (PR → root)
  is closed for twiceshy.
- A second runner to maintain. Its egress allowlist must permit the **internet**
  (setup-go downloads, the Go module proxy, the vuln DB, the golangci-lint install)
  and the **Forgejo instance** at `192.168.50.244:3030` (checkout, release upload);
  only RFC-1918 LAN lateral movement is denied.
- `make ci` is slower (it runs the security scans) and the tools must be installed
  to enforce locally (skip-with-warning otherwise; the brain has them).
- **Revisit at Tier B / external contributions (ADR-0010):** the shared
  socket-runner becomes unacceptable the moment any repo on it accepts code from a
  party that isn't trusted — every such repo must then move to a socketless,
  isolated runner. Tracked under epics 0009 / 0010.
- Extends ADR-0001's "the PR is the trust boundary" with "and the machine that
  evaluates the PR must itself be least-privilege."

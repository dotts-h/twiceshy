# Platform & adoption: hosting, sandboxed revalidation, scaling, access, monetization — and the mid-2026 literature delta

- **Date:** 2026-06-12
- **Method:** 5-angle research fan-out (5 search agents, ~200 sources
  consulted) + 2 adversarial verification agents over the 22 load-bearing
  claims (20 confirmed, 2 corrected — corrections folded in, details in the
  appendix). Companion to
  [EXPERIENCE_SERVICE_RESEARCH.md](EXPERIENCE_SERVICE_RESEARCH.md) (June
  2026, 250 sources) — this document covers what that one does not:
  the post-Q1-2026 literature delta, the D3 sandbox design, scaling shape,
  external exposure, monetization, and repo topology.
- **Status relative to ADRs:** nothing here requires breaking a locked
  decision. Every roadmap delta in §8 is tagged with the ADR-0001/0002
  decision it touches; all are *extensions* or *confirmations*.

---

## 0. The headline answers, in one paragraph each

**Hosting** stays exactly as locked (ADR-0001 §9): one Go binary, one SQLite
file, Docker on the NAS, Tailscale now, Cloudflare Tunnel later — every
claim under that decision survived verification, and the cloud escape hatch
is a single ~€4/mo VM running the identical image. **Scaling**: the
"service per language" instinct is right for the *runner* path (per-ecosystem
sandbox images pulling jobs from a queue) and wrong for the *query* path
(the corpus is one index; ecosystem is a `WHERE` clause — splitting it would
break cross-ecosystem retrieval). **RPCs**: no gRPC, no new protocols; the
locked surface (MCP streamable HTTP + plain HTTP hook) is sufficient,
*provided the server is stateless* — which the MCP spec's 2026-07-28 release
candidate now all but mandates. **Access**: static API keys are first-class
in every client that matters (Claude Code, Cursor, VS Code); OAuth is a thin
later add-on only if claude.ai-web users are wanted. **Rate limiting** must
live in the Go process (Cloudflare's free tier can't key on headers).
**Monetization**: GitHub Sponsors now (0% fees, no VAT event), Polar.sh as
merchant of record when a paid tier ships — its built-in license-key service
can replace the entire key-issuance backend. **Cookbook** stays a separate
repo; the Claude Code plugin-marketplace mechanism *is* the loose link with
graceful degradation, for free.

---

## 1. Literature delta (Q4 2025 → June 2026): the bets are confirmed, the window is open

### 1.1 Independent confirmation of the locked invariants

- **Near-miss injection hurts — now benchmarked on real coding tasks.**
  [SWE Context Bench](https://arxiv.org/abs/2602.08316) (Feb 2026; 1,100
  base + 376 related tasks, 51 repos, 9 languages, built from real GitHub
  issue/PR reference chains): well-selected prior experience significantly
  improves resolution and cuts runtime/token cost, especially on hard tasks
  — while poorly selected or unfiltered context yields "limited or negative
  benefits." This is the strongest external evidence yet for the k≤3 cap and
  the relevance floor (ADR-0001 §3). *Verified against the arXiv abstract.*
- **The poisoning threat model is now demonstrated against coding agents
  specifically.** [MemoryGraft](https://arxiv.org/abs/2512.16962) (Dec
  2025): benign-looking ingestion artifacts cause a coding agent (MetaGPT
  DataInterpreter + GPT-4o) to self-construct a poisoned experience store;
  a handful of poisoned records dominate retrieval and persist across
  sessions. The paper offers **no defense** — twiceshy's repro-gated
  quarantine (ADR-0001 §6) is a direct answer. Memory poisoning is also now
  a named OWASP category:
  [ASI06 "Memory & Context Poisoning"](https://genai.owasp.org/2025/12/09/owasp-top-10-for-agentic-applications-the-benchmark-for-agentic-security-in-the-age-of-autonomous-ai/)
  in the OWASP Top 10 for Agentic Applications (Dec 2025). Both belong in
  ADR-0001's citation trail.
- **Staleness is the field's stated #1 production gap — and the market
  leader has no answer.** Mem0's own
  [State of AI Agent Memory 2026](https://mem0.ai/blog/state-of-ai-agent-memory-2026)
  lists staleness among five open gaps and contains *no* discussion of
  validation, quarantine, or poisoning. The doctors roster (ADR-0001 §7) is
  aimed at exactly the gap the incumbent names and doesn't address.
- **Itemized records beat rewritten stores.**
  [ACE](https://arxiv.org/abs/2510.04618) (ICLR 2026) names "context
  collapse" and "brevity bias" as failure modes of LLM-rewritten memory and
  shows incrementally updated, itemized playbooks win (+10.6% on agent
  tasks) — independent support for discrete records + supersede-never-delete
  + delta-only doctors.
- **The taxonomy converged on our shape.** The 47-author survey
  [Memory in the Age of AI Agents](https://arxiv.org/abs/2512.13564)
  (Dec 2025) canonizes **"experiential memory"** as a first-class category
  (vs factual/working). Worth adopting the term in CONTEXT.md as the
  literature-facing name for what records are.

### 1.2 "CI for memories" is still unclaimed — but the window is closing

The nearest academic neighbors as of June 2026:

- [GLOVE](https://arxiv.org/abs/2601.19249) (Jan 2026) — *probe-based*
  detection of contradictions between stored memories and fresh
  observations under environment drift. Reactive, not scheduled; no repro
  execution.
- [SSGM](https://arxiv.org/abs/2603.11768) (Mar 2026) — "verify before
  consolidate" governance for evolving memory. Purely conceptual, no
  implementation or eval.
- The skill-library lineage (MUSE-Autoskill, SAGE, arXiv 2602.20867 SoK)
  independently converged on "test against validation cases before
  admitting to the library" — the same instinct as the guard, without the
  fail-to-pass discipline or the revalidation schedule.

**Nobody does scheduled sandboxed re-execution of repro scripts against a
memory store.** D3 remains the novel, citable contribution — and given that
two adjacent papers landed in Q1 2026 alone, the publish-or-be-scooped
clock is running. The same applies to the trap-avoidance eval (ADR-0001 §8):
new 2026 benchmarks (MemoryAgentBench, AMA-Bench, BEAM) still measure
recall, not trap avoidance.

### 1.3 What changed in the host-agent landscape (positioning, not architecture)

- **Claude Code now ships auto memory on by default** (v2.1.59+,
  [official docs](https://code.claude.com/docs/en/memory)): per-project
  `MEMORY.md` + topic files under `~/.claude/projects/<project>/memory/`,
  first 200 lines / 25 KB auto-injected every session. A consolidation
  feature (community name "Auto Dream", `/dream`) reportedly merges and
  *deletes* contradicted facts — **officially undocumented**; treat the
  details as community lore. OpenAI Codex shipped
  [Memories](https://developers.openai.com/codex/memories) (off by default,
  not yet in the EEA): auto-extracted, unvalidated, "generated state."
- **Letta pivoted to coding** —
  [Context Repositories](https://www.letta.com/blog/context-repositories/)
  (Feb 2026): git-backed memory with auto-commits and periodic reflection,
  plus a "memory-first coding agent" app. This is the closest commercial
  neighbor. What it lacks — execution-gated validation, fingerprint+lexical
  hot path, push-channel discipline — is precisely the moat. **Watch this
  project.**
- Consequence for positioning: every host agent now has *unvalidated,
  private, self-written* memory. The pitch sharpens from "agents need
  memory" to **"agents need validated, team-shared, execution-proven
  memory"** — and the delete-on-contradiction behavior of vendor
  consolidation is the philosophical foil to supersede-never-delete.
- One controlled memory-on/off coding benchmark exists
  ([vendor-run, single system](https://medium.com/@mrsandelin/the-first-controlled-benchmark-of-ai-memory-in-coding-agents-8e0bb776d39e),
  Mar 2026 — treat numbers cautiously): memory did **not** improve code
  quality but cut cost 22–32% and turns 28–40% on complex tasks, and was
  counterproductive on trivial ones. Two lessons: (a) sell trap-avoidance
  and cost/turns, not quality scores; (b) "inject nothing" on simple tasks
  is a feature — the relevance floor again.

---

## 2. D3 sandboxed revalidation: the concrete design

All claims verified against primary sources (Intel ARK, gVisor docs,
Firecracker SPECIFICATION.md, vendor pricing pages).

### 2.1 Hardware reality

The Pentium Gold 8505 supports VT-x/VT-d
([Intel ARK](https://www.intel.com/content/www/us/en/products/sku/226262/intel-pentium-gold-processor-8505-8m-cache-up-to-4-40-ghz/specifications.html)),
so **no sandbox technology is ruled out** — KVM-backed microVMs included
(confirm VT-x is enabled in the UGREEN BIOS; it ships enabled for the
vendor's own VM app). The real constraint is **5 weak cores, not RAM**:
toolchain runs (cargo, JVM, `go test`) want 1–2 vCPU each, so the practical
concurrency is **3–4 runs** under *any* isolation tech. Pick on security and
operational cost, not density.

### 2.2 Technology verdict

| Tech | Isolation | Ops cost on a NAS | Verdict |
|---|---|---|---|
| runc (plain Docker) | namespaces+seccomp | zero | insufficient alone for promotion-gating runs — Judge0's namespace-only judges have public escape CVEs (CVE-2024-28185/-28189) |
| **gVisor runsc (Systrap)** | userspace kernel, no KVM needed | one `daemon.json` line | **the pick**: drop-in Docker runtime, drastic kernel-attack-surface cut, ~1.3–2× syscall-heavy slowdown is irrelevant overnight |
| Firecracker | microVM (≤125 ms boot, ~5 MiB VMM overhead at the 1 vCPU/128 MiB reference config) | high: no OCI images — you build a rootfs/kernel/snapshot pipeline | overkill now; the upgrade path if the threat model hardens |
| Kata + Cloud Hypervisor | microVM with OCI compat | medium-high | documented fallback if gVisor compatibility bites |
| nsjail / bubblewrap / isolate | namespaces, hand-rolled policy | you own the policy | wrong altitude — building blocks, not a runtime |
| WASI | n/a | n/a | excluded: real toolchains need fork/exec, dynamic linking, JIT |

Notably, both agent vendors converged on the same egress pattern twiceshy
needs: Anthropic's
[sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime)
(bubblewrap on Linux, network namespace removed, egress only via a
host-side proxy) and
[OpenAI Codex sandboxing](https://developers.openai.com/codex/concepts/sandboxing)
(Landlock + seccomp, sockets blocked). They use namespace-class isolation
because their code is *assistive*; promotion-gating runs on semi-trusted
records justify the gVisor step up.

### 2.3 The design (proposed as ADR-0003 when D3's phase opens)

1. **Runtime:** Docker + `runsc` (Systrap) for repro containers only; one
   ephemeral container per run, never reused. Kata documented as the
   hardening path.
2. **Images:** one per ecosystem — `repro-go`, `repro-node`, `repro-python`,
   `repro-rust`, `repro-jvm` — pinned by digest, rebuilt weekly.
3. **No internet egress, ever.** Sandboxes sit on an internal-only Docker
   network with per-ecosystem registry proxies
   ([Athens](https://docs.gomods.io/) for Go, Verdaccio for npm, devpi for
   PyPI, sparse-registry/Nexus mirrors for cargo/Maven). Dependency fetches
   work; exfiltration and call-home don't.
4. **Two run modes per record:** *pinned* (lockfile hash recorded →
   feeds `validated`) and *latest* (dependencies floated → staleness probe).
   *Latest* runs trigger on Renovate/Dependabot-style bump signals against
   the record's `applies_to`, plus a weekly sweep — this is also D2's
   execution-grounded complement.
5. **Queue:** one SQLite table in the existing DB (id, ecosystem,
   record_id, mode, run_after, lease_until, attempts, status) with
   visibility-timeout leases — the [goqite](https://github.com/maragudk/goqite)
   pattern (12.5–18.5k msg/s — four orders of magnitude above need). Zero
   new dependencies; River, the mainstream Go queue, is Postgres-only and
   thus out of budget anyway.
6. **Scheduling:** single Go worker pool, concurrency 3, niced, overnight
   window (ADR-0001 §9's batch/overnight doctrine).
7. **Result schema:** `run(record_id, mode, image_digest, lockfile_hash,
   started_at, duration_ms, phase_fail{exit, stdout_trunc},
   phase_pass{…}, verdict)`. Verdict requires **fail-without-fix AND
   pass-with-fix** (the F2P contract, exit 75 = environment skip).
   Pinned-pass ⇒ `validated` evidence; latest-F2P-broken ⇒ `stale` flag +
   issue; sandbox error/timeout ⇒ `unverified` — **never silent
   promotion**. Logs stored truncated and treated as untrusted input.
8. **Security checklist (every run):** `--runtime=runsc`, `--network none`
   (internal proxy net attached only during the fetch phase), `--read-only`
   root + sized tmpfs workdir, `--memory 3g --cpus 2 --pids-limit 256`,
   `--cap-drop ALL`, `--security-opt no-new-privileges`, non-root UID,
   hard wall-clock timeout (10 min default, SIGKILL), userns-remap on the
   daemon, quarantined records never scheduled with elevated trust.
9. **Cloud escape hatch (verified pricing):**
   [Fly Machines](https://fly.io/docs/about/pricing/) first — same OCI
   images under Firecracker, per-second billing, ~$0.003–0.03 per run,
   stopped machines cost only rootfs storage.
   [Modal](https://modal.com/pricing) ($0.0000131/core-s + $0.00000222/GiB-s,
   gVisor) carries a **recurring $30/month free credit** — roughly 10,000
   typical runs/month free, confirmed current as of 2026-06-12.
   [E2B's infra](https://github.com/e2b-dev/infra) (Apache-2.0,
   self-hostable Firecracker) if microVM snapshots on owned metal ever
   matter.

---

## 3. Scaling shape: one index, many runners

### 3.1 The decomposition verdict

**Query path: one service, one index, forever.** Ecosystem is a `WHERE`
clause / FTS5 filter, not a deployment boundary. Splitting the corpus per
language would (a) break cross-ecosystem retrieval — a Postgres trap is
relevant to Go and Python services alike; the *generic fingerprint* exists
precisely to fire across stacks — and (b) multiply ops burden for a corpus
that fits in page cache. Prior art agrees: Zoekt (Sourcegraph's code
search) shards by *data size* (~100 MiB/shard), never by language, and runs
60 GB corpora on a single machine; twiceshy's whole corpus is below one
Zoekt shard. Fowler's
[MonolithFirst](https://martinfowler.com/bliki/MonolithFirst.html) covers
the rest.

**Runner path: per-ecosystem workers pulling from a queue.** This is where
the owner's instinct is right, recast pull-style: dumb per-language images
(§2.3) polling the core's job queue — the
[Buildkite agent model](https://buildkite.com/docs/agent/v3) (outbound-only
HTTPS polling, no inbound firewall holes, capability-tagged queues). Scale
= `docker compose up --scale runner-python=3`. The query path never changes.

### 3.2 What ~1000 querying agents actually need

- [WAL mode](https://www.sqlite.org/wal.html): unbounded concurrent
  readers + exactly one writer; readers and writers don't block each
  other. One caveat to engineer around: **checkpoint starvation** — a
  perpetually open read transaction lets the WAL grow without bound. Keep
  reads short (they are: point queries returning tiny JSON) and never hold
  long read transactions in doctors.
- Capacity: sqlite.org itself serves 400–500K req/day (~200 SQL/page) on a
  fraction of one shared VM
  ([whentouse](https://www.sqlite.org/whentouse.html)); a measured FTS5
  benchmark through a full interpreted-PHP stack did ~840 req/s on 7K
  documents. 100–1000 qps of FTS5 MATCH + fingerprint lookups on a few-MB
  corpus is comfortably inside the envelope on the 8505, in one process.
  (One 2026 paper claims ~1.2 s FTS latency at 5K records — inconsistent
  with FTS5's design and the above; likely measuring their whole pipeline.
  Resolution: add a **latency budget benchmark to `make ci`** rather than
  trusting either anecdote.)
- Driver: the
  [2026 go-sqlite-bench run](https://github.com/cvilsmeier/go-sqlite-bench)
  puts zombiezen ~2× ahead of modernc/mattn on concurrent reads — a
  2× factor, not an architecture factor. **Staying on modernc (pure Go,
  no CGO, already in go.mod) is correct**; revisit only if the CI latency
  benchmark ever fails. Use a writer connection + reader pool and
  `SetMaxOpenConns` tuning.
- When SQLite stops being enough, the accepted step is **read replicas of
  the same file** (Litestream v0.5+,
  [revamped Oct 2025](https://fly.io/blog/litestream-revamped/), absorbed
  LiteFS) — and twiceshy is luckier still: a replica is `git pull && twiceshy
  index`. No replication infrastructure at all. rqlite/Turso/Postgres only
  earn their keep with a high-volume networked *write* path, which the
  git-mediated write channel deliberately doesn't have (ADR-0001 §1, §6).

### 3.3 Orchestration verdict

Plain **Docker Compose + Caddy**, full stop. k3s idles at ~1.6 GB RAM and a
meaningful slice of a core
([k3s's own profiling](https://docs.k3s.io/reference/resource-profiling)) to
orchestrate one binary and a SQLite file; Swarm and Nomad are smaller but
still negative-value at one node. Compose already does `deploy.replicas`
and Caddy/Traefik load-balance them if ever needed — but scaling
GOMAXPROCS-internally beats scaling container-count-externally for a Go
process. The whole "platform" is: Compose, Caddy, `restart:
unless-stopped`, Litestream (or a cron `cp` — the index is rebuildable) and
the runner images.

### 3.4 Cloud path (verified pricing, post-Hetzner-increase)

Lift the same image to **one small VM or Machine** when the NAS stops being
appropriate: Hetzner CX23 (2 vCPU/4 GB x86) **€3.99/mo** or CAX11 (ARM)
€4.49/mo — note Hetzner raised cloud prices up to ~37% effective Apr 2026,
verified against
[their announcement](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/);
Fly shared-cpu-1x ≈ **$2/mo**. Cloudflare Containers (GA Apr 2026, 10 ms
billing, bundled in the $5 Workers Paid plan) is interesting for
scale-to-zero *runners*, not for the always-warm query path. Architecture
change required to move: **zero** — that was the point of ADR-0001 §1.

---

## 4. API surface: no new RPCs — but be stateless *now*

- **No gRPC.** Nothing in the client ecosystem wants it; MCP streamable
  HTTP + plain HTTP hook (ADR-0001 §5) covers pull and push; the runner
  protocol is "poll a queue over HTTP" (§3.1), and the queue itself is a
  SQLite table (§2.3). The dependency budget survives intact.
- **The stateless imperative (time-sensitive):** the current stable MCP
  revision is **2025-11-25**; the release candidate published 2026-05-21,
  targeting final publication **2026-07-28**, **removes protocol-level
  sessions and the `Mcp-Session-Id` header entirely (SEP-2567) and removes
  the `initialize` handshake (SEP-2575)** —
  [verified against the official blog](https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/)
  and [draft changelog](https://modelcontextprotocol.io/specification/draft/changelog).
  In the 2025-11-25 spec, sessions are already optional (a server that
  never issues `Mcp-Session-Id` is conformant). Therefore: **never issue a
  session ID, return plain JSON, 405 the GET/SSE leg** — and any
  session-shaped work done now is throwaway by spec decree. Track the
  go-sdk's migration to the final revision (expect a `Mcp-Method`-style
  header handling change).
- **Transport hardening already required by spec:** validate the `Origin`
  header (403 otherwise — DNS-rebinding defense), bind to the tailnet
  interface rather than 0.0.0.0, tokens only in `Authorization` headers,
  never in query strings, no token passthrough.

---

## 5. Access control & rate limiting: the three-phase plan

### 5.1 Client auth reality (verified 2026-06-12)

| Client | Static API key | OAuth |
|---|---|---|
| Claude Code | ✅ first-class: `claude mcp add --transport http … --header "Authorization: Bearer …"`, `${VAR}` expansion in `.mcp.json` | ✅ |
| Cursor, VS Code/Copilot | ✅ headers supported | ✅ |
| OpenAI API (Responses MCP tool) | ✅ `authorization` param | — |
| claude.ai web connectors | ❌ **OAuth only** ([no header field](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)) | ✅ |

MCP authorization is **OPTIONAL** in the spec; static bearer over HTTPS is
legal (because auth is out-of-band-permitted, not because the OAuth flow
allows it). The OAuth trigger is claude.ai-web users and nothing else; when
it comes: serve RFC 9728 protected-resource metadata + a 401
`WWW-Authenticate`, delegate to a hosted authorization server (never write
one), support Client ID Metadata Documents, **skip Dynamic Client
Registration** (formally deprecated in the 2026-07-28 revision). Keep API
keys forever alongside.

### 5.2 Exposure phases

1. **Personal (now):** Tailscale only, no Funnel. Bind the tailnet
   interface; require one static key anyway (defense in depth); validate
   Origin. Funnel is wrong for any public phase — 443/8443/10000 only,
   `*.ts.net` names only, non-configurable bandwidth caps
   ([verified](https://tailscale.com/kb/1223/funnel)).
2. **Friends:** Cloudflare Tunnel (free; outbound-only — home IP never
   published; capped at 1,000 tunnels/account, irrelevant) on a cheap
   domain. `cloudflared` as its own container; twiceshy on an internal
   Docker network so *nothing else on the NAS* is reachable through the
   tunnel. Cloudflare Access (free ≤50 users) **service tokens** per friend
   as an edge gate *in front of* twiceshy's own API keys.
3. **Public:** same topology; drop the Access wall on the MCP path; rely on
   twiceshy API keys with self-serve issuance (§6). Caveat accepted:
   Cloudflare terminates TLS and sees plaintext — acceptable for
   non-secret experience records; a VPS+WireGuard proxy is the documented
   alternative if that ever changes.

### 5.3 API keys & rate limiting

- **Key format** (GitHub's
  [token-format rationale](https://github.blog/engineering/platform-security/behind-githubs-new-authentication-token-formats/)):
  `tsy_live_<base62-178bit>_<crc32>` — greppable prefix, offline-checksum
  tail (secret-scanning-registrable later).
- **At rest: SHA-256, not bcrypt** — ≥128-bit random keys don't need a slow
  KDF (that's for low-entropy passwords, per OWASP), and bcrypt's ~100 ms
  would tax every request. Constant-time compare. Columns: `scopes`
  (`pull` = MCP search/get; `push` = hook endpoint; later `record`),
  `created_at`, `expires_at`, `last_used_at` (batched updates),
  `revoked_at` — **revoke = supersede, never delete** (house rule applies
  to keys too).
- **Rate limiting lives in-process.** Cloudflare's free tier allows exactly
  **1 rule, keyed by IP only** (header/API-key keying is Enterprise —
  [verified plan table](https://developers.cloudflare.com/waf/rate-limiting-rules/)),
  so the edge rule is a coarse DDoS backstop and per-key limits are
  `golang.org/x/time/rate` token buckets in a mutex-guarded map (bytes per
  key; 100K keys = megabytes) + a SQLite daily-quota counter for durable
  free-tier quotas across restarts. Suggested starting points: pull 2 rps
  burst 10; push 1 rps burst 5; ≤4 concurrent requests per key.
  *Note: `golang.org/x/*` needs an explicit dependency-budget nod
  (CONVENTIONS) — decision batch 1.*
- **Fail-open belongs to the hook, fail-closed to the server.** The Claude
  Code hook treats any non-2xx/timeout (~1–2 s budget) as "no injection,
  continue" — the agent never blocks on twiceshy. The server still 401s/
  429s unauthenticated or over-quota calls; a fail-open *server* would make
  the push path an unauthenticated abuse vector, the exact thing the
  quarantine invariant exists to prevent (ADR-0001 §6).

---

## 6. Monetization: Sponsors → Polar, $0 infrastructure until traction

### 6.1 Rails (fees verified on primary pages, 2026-06-12)

| Rail | Fee on a $5/mo sub | Notes |
|---|---|---|
| GitHub Sponsors | **$0** (personal-account sponsors; 100% to maintainer) | donation, not sale → no VAT event; Romania supported since 2021 |
| Polar.sh Starter (MoR) | $0.75 (15%) + 1.5% intl-card surcharge ≈ **$0.83** | handles all VAT; $15/dispute; Stripe payout fees passed through; **built-in license-key benefit** (§6.3) |
| Paddle (MoR) | $0.75 (15%) | sub-$10 products need sales approval — friction |
| Stripe Managed Payments (MoR, preview) | **3.5% on top of standard Stripe fees** ≈ $0.62 effective (~12.5%) | preview-stage; pricing corrected in verification — *not* fee-parity with Polar/Paddle |
| Stripe direct (not MoR) | ≈ $0.35 (7%) | cheapest, but VAT paperwork is yours |
| Lemon Squeezy | — | acquired by Stripe (2024); brand in wind-down toward Managed Payments; don't build on it |

EU/Romania reality at "cover electricity" scale: under €10K/yr cross-border
B2C digital sales you may charge Romanian VAT and skip OSS; Romania's
domestic VAT registration threshold is RON 395,000 (~€79K) — at hobby
revenue, direct Stripe is legally tractable, an MoR just deletes the
bookkeeping. (PFA-vs-SRL income-tax status is separate: one conversation
with a Romanian accountant before the first paid invoice.)

### 6.2 The ladder

1. **Now ($0 revenue):** GitHub Sponsors profile with a Healthchecks.io-style
   "Supporter — $5/mo" framing. Comparables prove the band: Healthchecks.io
   $5/mo, Miniflux $15/yr, Plausible from $9/mo, Vikunja €40/yr — all
   verified current. This alone plausibly covers NAS electricity + a domain.
2. **Paid API tier (when public):** free tier + **$5/mo individual** +
   $15–25/mo team, via Polar. The funded competitors (Mem0 $19, Zep $25
   entry) leave the indie price point wide open. AGPL is no obstacle —
   Plausible is the canonical AGPL-core-paid-cloud case (ADR-0002 §3's
   separate-process rule already shapes this correctly).
3. **Experience packs:** *not a revenue pillar.* Verified: no third-party
   marketplace for agent rules/packs exists in 2026 (Cursor's is
   first-party; community packs are free-copy culture; PromptBase sustains
   image/marketing prompts, not dev corpora). If packs sell, sell them as
   one-time Polar products with license keys through your own checkout.
   ADR-0002 §4 already licenses for this; no change needed.

### 6.3 Landing page + signup + key issuance, near-zero ops

Static landing on **Cloudflare Pages** (free), "Sign in with GitHub" (the
audience is 100% GitHub users; no email deliverability surface), a Worker +
D1/KV for account state — all inside free tiers
([verified](https://developers.cloudflare.com/workers/platform/pricing/):
100K req/day, D1 5M reads/day). **Key issuance: outsource to Polar's
license-key benefit** (`POST /v1/customer-portal/license-keys/{activate,validate}`,
per-key quotas) — twiceshy validates keys with one HTTP call and caches the
result, replacing an entire auth backend. Self-minted `tsy_` keys (§5.3)
remain for the free tier and friends phase; Polar keys gate the paid tier.
The paid surface is a separate process talking to the AGPL core over its
public API — exactly ADR-0002 §3's boundary, enforced by deployment shape.

---

## 7. Repo topology: Cookbook stays separate — the plugin marketplace *is* the loose link

`dotts-h/Cookbook` is a **Claude Code plugin-marketplace repo** (per the
owner's settings: `extraKnownMarketplaces.cookbook-dev → github:
dotts-h/cookbook`, plugin `cookbook@cookbook-dev`). That resolves the
topology question cleanly:

- **Keep it separate.** The decisive discriminators are access control and
  licensing, not content type: Cookbook is private; the engine is public
  AGPL; ADR-0002 §4 already establishes that non-engine content lives
  outside the AGPL boundary. Submodules are disqualified outright (a
  private submodule breaks every public `git clone --recursive`; plus the
  [well-documented operational pain](https://blog.timhutt.co.uk/against-submodules/));
  subtree/merge would irreversibly bake private operational history into
  public AGPL history.
- **The loose coupling already exists and degrades gracefully by
  construction:** a repo or user declares the marketplace in settings; the
  plugin loads when reachable and enabled; when absent, *nothing breaks and
  nothing is missing but the plugin's own features*. No submodules, no
  vendoring, no runtime-discovery code to write or test. twiceshy's CI
  never knows Cookbook exists.
- **Distribution insight (extends issue #2):** the Phase 2 deliverable
  "reference hook script + settings snippet" should ship as a **`twiceshy`
  Claude Code plugin** — hook config + MCP server entry (+ later a skill for
  `record_experience` etiquette) — distributable via the Cookbook
  marketplace now and via a public marketplace repo at launch. Onboarding
  becomes: add marketplace → enable plugin → paste API key. This is also
  the natural index-channel vehicle (ADR-0001 §5's generated one-liners).
- **Content flow rule:** engine-generic operational lessons that *should*
  be public migrate individually into `experience/` as records (the normal
  Phase 0 motion); private infra runbooks stay in Cookbook. If deployments
  ever need recipe snapshots, publish them as versioned artifacts (GitHub
  release tarball or [ORAS](https://oras.land/docs/) OCI artifact) and
  fetch at deploy time — never vendor.

---

## 8. Roadmap deltas (priority order)

Each tagged with the ADR decision touched and the tracked issue it extends.
None breaks a locked decision; items marked **(ADR-0003)** bundle the new
decisions that deserve their own ADR.

1. **Stateless MCP server, by test.** Never issue `Mcp-Session-Id`; plain
   JSON; 405 the SSE leg; Origin validation; bind tailnet interface. Add
   guarding tests; track the go-sdk's 2026-07-28-revision migration.
   *(ADR-0001 §5 confirmed; new small issue; do before Phase 2.)*
2. **Phase 2 hook hardening semantics** (extends #2): hook-side fail-open
   (1–2 s timeout → inject nothing), server-side fail-closed (401/429);
   per-key token bucket + SQLite daily quotas; `tsy_`-format keys, SHA-256
   at rest, scopes `pull`/`push`, revoke=supersede. *(ADR-0001 §5–6;
   dependency-budget ask: `golang.org/x/time/rate`.)*
3. **Ship the Phase 2 client as a Claude Code plugin** (extends #2, §7):
   hook config + MCP entry, distributed via marketplace; degrade
   gracefully when twiceshy is unreachable. *(ADR-0001 §5 index channel.)*
4. **Latency budget benchmark in `make ci`** (extends #2's load smoke
   test): FTS5 MATCH + fingerprint lookup p99 against a 1K-record corpus on
   the dev box; the answer to conflicting FTS5 benchmark anecdotes is our
   own number. *(ADR-0001 §3–4.)*
5. **ADR-0003: sandbox & runner architecture** (extends #4/D3): gVisor
   runsc + per-ecosystem images + no-egress registry proxies + SQLite job
   queue + concurrency 3 overnight + pinned/latest dual mode + F2P result
   schema feeding validated/stale. The runner protocol is queue-polling —
   no gRPC, no new deps. *(Extends ADR-0001 §7, §9 — deployment grows
   runner containers; queue table lives in the existing SQLite file.)*
6. **Exposure phases** (new issue, pre-public): Tunnel + Access service
   tokens for friends; public keys later; never Funnel. cloudflared in its
   own container on an internal network. *(ADR-0001 §9 confirmed and
   detailed.)*
7. **Eval substrate upgrade** (extends #5): adopt SWE Context Bench-style
   related-task pairing for the trap-avoidance suite; score avoidance +
   steps/tokens (not quality); keep the abstention category. **Publish the
   methodology and name "CI for memories" — the scoop window is open but
   closing (GLOVE/SSGM landed Q1 2026).** *(ADR-0001 §8.)*
8. **Monetization prep** (new issue, cheap now): GitHub Sponsors profile;
   reserve the domain; CLA.md before first external PR (ADR-0002
   consequence, already tracked); Polar org + landing page only when Phase
   2/3 make the service usable by strangers. *(ADR-0002 §3–4 confirmed.)*
9. **Docs deltas** (this PR or next): CONTEXT.md gains "experiential
   memory" as the literature-facing synonym; ADR-0001's citation trail
   gains MemoryGraft + OWASP ASI06 + SWE Context Bench; competitive watch
   note on Letta Context Repositories. *(Docs only.)*
10. **Explicitly rejected** (recording the negative result): per-language
    query services; gRPC; k3s/Swarm/Nomad on the NAS; session-stateful MCP;
    bcrypt for API keys; Lemon Squeezy; experience-pack marketplace as a
    revenue pillar; Tailscale Funnel for public exposure; WASI sandboxes.

## 9. Decision batches for the owner

Go over these in order; each is independently decidable. Recommendations
inline — all are "smallest thing that preserves the invariants."

**Batch 1 — unblocks Phase 2 (decide first):**
- a. Approve `golang.org/x/time/rate` into the dependency budget
  (recommend: yes — x-repo, no transitive deps).
- b. Key scheme: self-minted `tsy_` keys now, Polar license keys only for
  the future paid tier (recommend: yes).
- c. Hook timeout budget: 1 s or 2 s (recommend: 1 s — injection is a
  bonus, never a tax).

**Batch 2 — D3/ADR-0003 (decide when Phase 4 nears):**
- a. gVisor vs Kata as the promotion-gating runtime (recommend: gVisor;
  Kata documented as upgrade path).
- b. Registry-proxy set to operate (recommend: start Athens + Verdaccio +
  devpi only — match the ecosystems the corpus actually contains).
- c. Staleness-trigger source: Renovate on a deps manifest vs native
  `applies_to` polling (recommend: start with the weekly sweep, add bump
  triggers when D2 lands).

**Batch 3 — exposure (decide before first external user):**
- a. Buy the domain (recommend: yes, now — cheap, unblocks Tunnel + email).
- b. Friends-phase gate: Access service tokens in front of API keys
  (recommend: yes, both layers).
- c. Accept Cloudflare TLS termination (recommend: yes — records aren't
  secrets; revisit if private corpora ever onboard).

**Batch 4 — money (decide whenever):**
- a. Sponsors profile now (recommend: yes — zero cost, zero VAT).
- b. MoR: Polar vs direct Stripe (recommend: Polar — the 8-point fee
  premium buys all VAT/bookkeeping plus the license-key backend).
- c. Price points: $5 individual / $15–25 team (recommend: anchor at $5,
  decide team tier after first ten users).
- d. Accountant conversation re PFA/SRL before first paid invoice
  (recommend: yes).

**Batch 5 — positioning (decide soon; time-sensitive):**
- a. Publish the "CI for memories" name + methodology (blog/preprint)
  before or with Phase 4 (recommend: with the eval baseline from #5 — the
  GLOVE/SSGM adjacency says don't sit on it for a year).
- b. Adopt "experiential memory" vocabulary in docs (recommend: yes, as a
  synonym, keeping the house terms).

---

## Appendix: verification notes

Two adversarial verifier agents re-checked the 22 load-bearing claims
against primary sources (2026-06-12). Outcomes: 20 confirmed exactly
(including: Polar fee table to the cent; Modal's $30/mo credit confirmed
*recurring*; Fly/E2B pricing exact; all four arXiv IDs, author counts, task
counts and finding directions; OWASP ASI06 naming; Cloudflare free-tier
rate-limit table; Tailscale Funnel constraints; Claude Code `--header`
syntax verbatim; WAL semantics incl. checkpoint starvation; go-sqlite-bench
numbers). Corrections folded into the text above:

1. **Stripe Managed Payments pricing**: 3.5% *on top of* standard Stripe
   fees (effective ≈ 6.4% + $0.30 US) — the researched "5% + $0.50" was
   wrong; it is *not* fee-parity with Polar/Paddle.
2. **Hetzner**: increase runs up to ~37% (not 30–35%); cheapest 2 vCPU/4 GB
   is now CX23 €3.99 (x86), CAX11 €4.49 (ARM); a second dedicated-server
   adjustment lands 2026-06-15.
3. Precision flags: initialize-handshake removal is SEP-2575 (sessions are
   SEP-2567); Firecracker's ≤5 MiB overhead is the 1 vCPU/128 MiB reference
   config; Cloudflare Tunnel is capped at 1,000 tunnels/account; static
   bearers are legal because MCP auth is *optional*, not within the OAuth
   flow; "Auto Dream" consolidation details are community-reported, not in
   Anthropic's docs (auto memory itself, v2.1.59+, is official).

Known soft spots, flagged inline: the Stompy memory-on/off benchmark is
vendor-run on one system; the contradicting FTS5 latency paper (arXiv
2603.02240) was not reproduced (mitigated by roadmap delta #4, our own CI
benchmark); Lemon Squeezy signup-closure status unverifiable (their blog
403s — moot, since the recommendation avoids it); Cookbook's contents were
not inspected (repo outside session scope) — the topology recommendation
rests on its role as a plugin marketplace, which the owner's settings
snippet establishes.

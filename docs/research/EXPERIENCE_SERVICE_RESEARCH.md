# Experience-as-a-Service: a deep dive on feeding hard-won engineering experience to LLM coding agents

*Research report, 2026-06-12. Produced by a 5-angle web research fan-out (≈250 sources
searched, ~60 fetched) followed by an adversarial verification pass on the 18
load-bearing claims (16 confirmed against primary sources, 2 corrected — corrections
are folded in below and flagged where they matter). This document proposes; it does
not change anything in this repo.*

---

## 0. The thesis, in one paragraph

LLMs trained on StackOverflow know *answers*; they do not have *experience* — the
project-shaped, version-shaped, time-shaped knowledge of "we tried X here and it broke
Y, and here is the test that proves the fix." That knowledge is exactly what this
repo's `docs/REGRESSIONS.md` already records by hand (symptom → root cause → fix →
guarding test), and the 2025–2026 research wave shows that serving such records back
to agents measurably improves coding outcomes — *if and only if* retrieval is precise,
the records are kept fresh and validated, and the injection channel is deterministic
rather than "hope the agent asks." The proposed system — working name **Loreline** in
this doc; pick your own — is: git-backed markdown experience records as source of
truth, a derived hybrid (lexical + vector) index, served over a streamable-HTTP MCP
server on the NAS, injected client-side via Claude Code hooks, and kept honest by
"doctor" services that dedup, staleness-check against live docs, and *re-execute*
recorded reproductions in a sandbox. The last part — CI for memories — appears to be
genuinely novel: no published system wires repro scripts into a memory-revalidation
loop.

---

## 1. The problem is real and measured (not just a feeling)

Your intuition — "agents on autopilot trust their training too much and make bugs a
simple DB query would have prevented" — is quantified in the literature:

- **Deprecated-API generation.** Across 7 code LLMs and 8 evolving Python libraries,
  deprecated-API usage rates run **25–38% overall** — and critically, **70–90% when
  the prompt continues outdated code** vs 9–18% on current code: models mirror the
  context they're given, stale context in → stale code out. A simple
  deprecated→replacement *mapping table* fixed >85% of cases.
  ([ICSE 2025, arXiv:2406.09834](https://arxiv.org/abs/2406.09834) — verified)
- **Knowledge conflict under API evolution.** On a benchmark of 270 real API updates,
  LLM code was only **~43% executable** without current docs; injecting updated
  documentation lifted it to ~66%. API *modifications* (signature changes) are the
  hardest category. ([arXiv:2604.09515](https://arxiv.org/html/2604.09515))
- **Default statelessness.** Anthropic's memory-tool launch states plainly that agents
  previously could not "build up knowledge over time… or reference previous learnings"
  across sessions ([context management announcement](https://www.anthropic.com/news/context-management));
  Cursor shipped Memories for the same reason. The canonical practitioner story —
  correct the agent's deprecated pattern today, a fresh session reintroduces it
  tomorrow — is everywhere in 2025–2026 writeups.
- **This repo is a live specimen.** `REGRESSIONS.md` entries are literally phrased as
  pre-emptive counters to the *plausible-but-wrong* fix an agent would make on
  autopilot: "don't 'fix' a zeroed-after-restart row by pointing it back at
  `s.meter.Totals()`", "do not read `config.DefaultAgent` inside `recordUsage` — that
  inverts the lock order." Each is a trap with a documented escape. Today they only
  help if the file happens to be in context.

So the gap is not knowledge creation — you already create it — it's **delivery at
decision time**.

---

## 2. What the research says works (and what is marketing)

### 2.1 The taxonomy that stuck

The field converged on the CoALA-style split: **episodic** memory (what happened —
trajectories, incidents), **semantic** memory (facts about the world/project), and
**procedural** memory (how to do things — workflows, skills, strategies). Two surveys
anchor this ([arXiv:2512.13564](https://arxiv.org/abs/2512.13564),
[arXiv:2603.07670](https://arxiv.org/abs/2603.07670)).

The load-bearing finding for your idea: **strategy-level memory distilled from both
successes and failures is what moves coding benchmarks**, not conversational fact
memory:

| System | What it stores | Verified result |
|---|---|---|
| Reflexion (NeurIPS 2023) | per-task verbal self-reflections on failures | 91% pass@1 HumanEval in a retry loop ([arXiv:2303.11366](https://arxiv.org/abs/2303.11366)) |
| Voyager (2023) | executable skills, committed only after execution-verified success | 15.3× faster milestones in Minecraft ([arXiv:2305.16291](https://arxiv.org/abs/2305.16291)) |
| AWM (2024) | induced reusable workflows from successful trajectories | +24.6%/+51.1% relative on Mind2Web/WebArena ([arXiv:2409.07429](https://arxiv.org/abs/2409.07429)) |
| ExpeL (AAAI 2024) | raw success/failure trajectories **plus** abstracted cross-task insights | beats ReAct across 3 domains ([arXiv:2308.10144](https://arxiv.org/abs/2308.10144)) |
| **ReasoningBank** (ICLR 2026) | strategy items (title/description/reasoning) distilled from successes **and failures**, incl. "counterfactual pitfalls" | **+8.3pp WebArena, +4.6pp SWE-bench-Verified, ~3 fewer steps/task** (Gemini-2.5-flash config; gains vary +4.6–8.3 by model — verified against the paper) ([arXiv:2509.25140](https://arxiv.org/abs/2509.25140), [Google blog](https://research.google/blog/reasoningbank-enabling-agents-to-learn-from-experience/)) |
| SWE-Exp (2025) | multi-level experience bank of repair attempts, reused across issues | 73.0% pass@1 SWE-bench-V w/ Claude 4 Sonnet ([arXiv:2507.23361](https://arxiv.org/abs/2507.23361)) |
| MemGovern (2026) | GitHub issues/PRs governed into "experience cards" | +4.65% SWE-bench-V ([arXiv:2601.06789](https://arxiv.org/pdf/2601.06789)) |
| Memp (2025) | procedural memory with Build/Retrieve/Update policies | strong-model-built memory **transfers to weaker models** ([arXiv:2508.06433](https://arxiv.org/abs/2508.06433)) |

Two caveats to hold onto:

1. **SWE-bench contamination.** Claude-family models locate gold files ~3× better on
   SWE-bench-Verified than on uncontaminated equivalents
   ([arXiv:2512.10218](https://arxiv.org/abs/2512.10218)) — treat absolute SWE-bench
   deltas as directional, not gospel.
2. **Retrieval quality is the binding constraint.** SWE Context Bench finds
   well-retrieved prior experience improves accuracy and cuts cost *especially on hard
   tasks*, but **indiscriminate retrieval brings limited gains and can hurt**
   ([arXiv:2602.08316](https://arxiv.org/abs/2602.08316)). Bad memory is worse than no
   memory.

### 2.2 The benchmark wars: discount vendor numbers

The commercial memory layer (mem0, Zep, Cognee…) is evaluated on conversational-QA
benchmarks whose numbers are mutually contested. Verified shape of the dispute: Mem0's
paper scored Zep at **~66%** on LoCoMo; Zep's rebuttal alleges misconfiguration and
reports **75.14%**; a separate leg challenges Zep's own 84% claim (~58% in
replication); and — most damning — **a full-context baseline (~73%) beats Mem0's best
configuration (~68%) on Mem0's own headline benchmark**
([Zep rebuttal](https://blog.getzep.com/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/),
[zep-papers issue #5](https://github.com/getzep/zep-papers/issues/5)). Lesson: don't
buy a "memory platform" on its benchmark; run your own task-grounded eval (§8).

### 2.3 What the frontier labs actually ship

Both labs converged on **plain, human-auditable files the agent edits itself** — not
vector databases: Anthropic's memory tool is a file directory the agent CRUDs
([docs](https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool);
internal evals: +39% with memory + context editing, vendor-reported); Claude Code uses
CLAUDE.md + auto-memory; OpenAI Codex layers background-generated
`~/.codex/memories/` over AGENTS.md
([docs](https://developers.openai.com/codex/memories)). **This validates your repo's
markdown-first instinct.** The experience store should be files in git; everything
else is a derived index.

---

## 3. The most important design decision: the injection channel

This is where most "memory MCP server" projects quietly fail. The evidence is
unusually consistent:

- **Agents do not reliably call knowledge tools unprompted.** Even Anthropic's own
  reference memory MCP server ships a suggested system prompt forcing the model to
  begin every chat with retrieval ("Remembering…") — verified in the README
  ([modelcontextprotocol/servers, src/memory](https://github.com/modelcontextprotocol/servers)).
  The When2Tool benchmark (in "LLM Agents Already Know When to Call Tools",
  [arXiv:2605.09252](https://arxiv.org/abs/2605.09252)) finds agents call tools
  *indiscriminately* and that prompt-only steering suppresses necessary calls along
  with unnecessary ones — miscalibrated in both directions.
- **Even injected instructions degrade.** Claude Code's harness frames CLAUDE.md as
  "may or may not be relevant"; documented issues show models violating explicit
  NEVER/ALWAYS rules they have read; adherence decays as context fills and across
  compaction (issues [#5502](https://github.com/anthropics/claude-code/issues/5502),
  [#15443](https://github.com/anthropics/claude-code/issues/15443)).
- **Every commercial product that must be reliable does retrieval-side injection.**
  Sourcegraph Cody (multi-retriever + rerank, assembled *before* the model call,
  [architecture](https://sourcegraph.com/blog/how-cody-understands-your-codebase)) and
  Greptile (graph traversal outward from the diff) never wait for the model to ask.
- **Naive MCP tool exposure is also token-expensive**: Anthropic measured a 98.7%
  token reduction (150k→2k) replacing loaded tool definitions with code execution over
  filesystem-presented APIs ([engineering post](https://www.anthropic.com/engineering/code-execution-with-mcp) — verified).

So the reliability ladder, best to worst:

> **per-turn deterministic injection (hooks/middleware) > session-start injection >
> static memory file > model-judged index (skills) > hoping the agent calls a tool**

### The recommended hybrid: trap-driven push + index-driven pull

1. **Push channel (deterministic, high-precision, tiny).** A Claude Code
   **`UserPromptSubmit` hook** (and/or `PreToolUse` on Edit/Write) calls the
   experience service with the prompt + changed-file paths + stack fingerprint, and
   injects **at most 1–3 short "trap cards"** via `additionalContext` — only on a
   high-confidence match (fingerprint or near-exact symptom hit). Hooks are verified
   to support exactly this: stdout/`additionalContext` from `UserPromptSubmit` and
   `SessionStart` is added to context every turn
   ([hooks docs](https://code.claude.com/docs/en/hooks)). This is how a recorded trap
   interrupts autopilot *at the moment it matters*.
2. **Pull channel (on-demand, broad).** An MCP **tool** (`search_experience`,
   `get_experience`, `record_experience`) for when the agent is debugging and chooses
   to research — with the tool *description* carefully engineered, since description
   text alone has produced SOTA-level swings in tool-use quality
   ([Anthropic, writing tools for agents](https://www.anthropic.com/engineering/writing-tools-for-agents)).
3. **Index channel (cheap ambient awareness).** A generated **skill** /
   `AGENTS.md`-style index — one line per high-value lesson category — so the model
   knows the store exists and what's in it, Skills-style progressive disclosure
   (verified: only name+description loads at startup;
   [skills docs](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview)).
4. **Write path:** a `Stop`/`PostToolUse` hook (or end-of-session prompt) proposes new
   experience candidates from the session → they land in **quarantine**, not the
   trusted store (§6).

On **SSE**: the standalone HTTP+SSE MCP transport is deprecated; the current spec
defines stdio + **Streamable HTTP** (which may stream via SSE internally)
([spec](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) —
verified). And push-to-a-live-agent isn't a working pattern in today's clients (open
request: [claude-code #36665](https://github.com/anthropics/claude-code/issues/36665));
"live" should mean *event-triggered agent runs* (your doctors) plus per-turn hook
queries — not a server pushing into a conversation.

---

## 4. The data model: the Experience Record

Grounded in four verified bodies of prior art — postmortem schemas (Google
SRE/Atlassian/PagerDuty), **Sentry's grouping architecture**, the **OSV vulnerability
schema**, and agent-memory write pipelines (Mem0) — plus what `REGRESSIONS.md` already
does.

```yaml
# experience/2026/0142-pgvector-hnsw-maintenance-work-mem.md (YAML frontmatter + body)
id: exp-0142
kind: trap | fix | dead-end | convention | workflow   # episodic vs procedural
status: quarantined | validated | stale | superseded
title: "HNSW index build silently 10-50x slower when graph exceeds maintenance_work_mem"

symptom:                      # ── the retrieval surface (symptoms cluster; fixes diverge)
  summary: "..."              # free text, embedded
  error_signatures:           # normalized messages, exact-match surface
    - "checkpoints are occurring too frequently"
  fingerprints:               # Sentry-style: deterministic first, fuzzy second
    - app: "sha256:..."       #   in-app frames / repo-specific
    - generic: "sha256:..."   #   stack-generic (cross-repo)

applies_to:                   # ── OSV-style stack fingerprint → cross-repo reuse
  - ecosystem: "PyPI"
    package: "pgvector"
    versions: { introduced: "0.5.0", fixed: null }
  - runtime: { postgres: ">=14" }

resolution:
  root_cause: "..."           # 2-5 contributing factors, not one blame line
  fix: "..."                  # the change that worked
  dead_ends:                  # ← the part StackOverflow never captures
    - tried: "raising shared_buffers"
      why_it_failed: "..."
  guard:                      # ── executable knowledge (§7)
    repro: "repro/0142.sh"    # fails before fix, passes after (F2P discipline)
    guarding_test: "TestHnswBuildMemory"

provenance:                   # ── poisoning defense + trust (§6)
  source: { session: "...", pr: "...", author: "horia" }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12    # last sandbox re-execution
  valid: { from: 2026-06-12, until: null }   # bi-temporal, Graphiti-style
  superseded_by: null         # supersede, never delete
  usage: { retrieved: 14, confirmed_helpful: 5, last_hit: 2026-06-10 }
```

Why each piece is evidence-backed:

- **Symptom-side dedup, fix-side divergence.** In StackOverflow duplicate pairs,
  ~20–25% of question text and ~40% of tags overlap but only 5–6% of *answer* text
  ([Multi-Factor DQD](http://www.mysmu.edu/faculty/davidlo/papers/jcst-duplicateqns.pdf)) —
  deduplicate on symptoms, never merge by fix similarity.
- **Two fingerprints per record** (app-specific + stack-generic) is Sentry's verified
  design — it's what lets a lesson recorded in repo A fire in repo B
  ([Sentry grouping](https://develop.sentry.dev/backend/application-domains/grouping/)).
- **OSV-style `{ecosystem, package, version-range}`** is a ready-made, ecosystem-aware
  applicability schema ([OSV](https://ossf.github.io/osv-schema/)); combined with
  Graphiti-style bi-temporal validity (true *for a version range and a time range* —
  [arXiv:2501.13956](https://arxiv.org/abs/2501.13956)), you get something no
  published system has: version-axis × time-axis validity on engineering experience.
- **Dead-ends as first-class data** is the differentiator ReasoningBank/SWE-Exp proved
  out: failure-derived "counterfactual pitfalls" are where the gains come from.
- **Keep narrative alongside structure** (the markdown body below the frontmatter) —
  the incident-analysis tradition (Howie/Jeli) is adamant that learning lives in
  narrative, and your regression log's prose entries confirm it.
- Periodically **induce abstractions over episodes** (ExpeL/AWM pattern): a doctor job
  that distills 5 related trap records into one convention card. Both layers beat
  either alone.

---

## 5. Retrieval: fingerprint-first, hybrid second, tiny top-k always

The verified pipeline, in precedence order (mirroring Sentry):

1. **Exact**: fingerprint / error-signature hash lookup. Free, deterministic, no
   false positives. This alone covers the "looping on a known bug" case.
2. **Lexical**: BM25/FTS5 over title+symptom+signatures. Dense retrieval **fails
   silently on exact identifiers** (`torch.nn.functional.cross_entropy` embeds "near
   PyTorch docs", not near documents containing that literal); error-message corpora
   are identifier-heavy.
3. **Dense**: small local embedding (see §9) over symptom summaries for semantic
   matches. *Correction from verification:* CodeRAG-Bench finds dense models
   frequently **surpass** BM25 for code retrieval
   ([arXiv:2406.14497](https://arxiv.org/html/2406.14497v2)) — so dense is not
   optional garnish; BEIR's BM25-robustness finding
   ([arXiv:2104.08663](https://arxiv.org/abs/2104.08663)) says lexical is not
   optional either. **Hybrid with RRF (k=60) is the answer**, fused, then filtered
   and boosted by stack-fingerprint match.
4. **Hard cap on k.** Inject 1–3 records, ranked, with a relevance threshold below
   which you inject *nothing*. Verified basis: a single semantically-related-but-wrong
   retrieved document can cut accuracy by up to ~25pp (peak, SIGIR 2024
   ["Power of Noise"](https://arxiv.org/abs/2401.14887) — notably *random* noise was
   harmless-to-helpful; it's **near-miss documents that hurt**, and an experience DB
   is near-miss-dense by construction); Chroma's context-rot study shows degradation
   with length and even one distractor across 18 models
   ([context-rot](https://www.trychroma.com/research/context-rot)); mid-context
   placement is worst ([lost-in-the-middle](https://arxiv.org/abs/2307.03172)).

The near-miss hazard deserves emphasis because it's *this* system's #1 failure mode:
the most plausible retrieval for "budget row resets after restart" might be a
similar-looking entry about a different meter — and §1 says agents amplify whatever
context they're given. Mitigations: stack-fingerprint filters, the relevance floor,
and trap cards that carry their *applicability conditions* in the card text.

---

## 6. Security: an experience store is an attack surface

Verified, and sobering:

- **AgentPoison** (NeurIPS 2024): poisoning **<0.1%** of a memory/RAG store achieves
  **≥80% attack success** with ≤1% benign impact
  ([arXiv:2407.12784](https://arxiv.org/abs/2407.12784)).
- **MINJA** (NeurIPS 2025): memory injection with **query-only access** — the attacker
  never writes; the agent is manipulated into writing the poison itself — 98.2%
  injection success ([arXiv:2503.03704](https://arxiv.org/abs/2503.03704)). So
  "only my agent writes to its own memory" is **not** a defense.
- Memory poisoning is now OWASP-classified (Agentic Top 10, ASI06); Simon Willison's
  "lethal trifecta" framing applies directly: a shared experience store read by many
  agents is a cross-session injection *amplifier*.

Design consequences (the consensus defense stack, plus what your setup makes easy):

1. **Quarantine tier with explicit promotion.** Agent-proposed records enter
   `status: quarantined`; only sandbox validation (§7) + human glance (a PR!) promotes
   to `validated`. Git gives you this for free: **a new experience record is a pull
   request**. Diffable, signed, revertable, blame-able.
2. **Provenance on every record** (source session/PR/author/timestamps) — retrieval
   can weight by trust tier.
3. **Quarantined records never enter the push channel** — at most surfaced to pull
   queries, labeled.
4. Single-user NAS + Tailscale (§9) shrinks the writer set, but MINJA means even your
   own agent sessions are an injection path (a malicious web page the agent read can
   talk its way into a memory proposal). The PR gate is the real boundary.

---

## 7. The doctors: curation, freshness, and CI-for-memories

Your "doctor/analysis services" instinct matches exactly where the literature points —
and where it stops short.

**Doctor 1 — Dedup/Reconcile (write path).** Mem0's verified pipeline is the recipe:
new candidate → top-k similar existing records → LLM arbitrates
**ADD / UPDATE / SUPERSEDE / NOOP** ([arXiv:2504.19413](https://arxiv.org/abs/2504.19413)).
Two amendments from the evidence: supersede = close the validity interval and link
`superseded_by` — *never delete* (Graphiti's verified invalidation model); and
curate by **incremental deltas, never whole-store rewrites** — ACE documents "context
collapse," where an LLM curator that rewrites the playbook monolithically erodes it
([arXiv:2510.04618](https://arxiv.org/abs/2510.04618)).

**Doctor 2 — Staleness (scheduled).** For each `validated` record, re-check
`applies_to` against the world: bump-check package versions; cross-check named APIs
against live docs (Context7-style fetch of version-specific documentation —
[upstash/context7](https://github.com/upstash/context7) — plus `llms.txt` where it
exists, treated as a locator not a guarantee). On drift: flag `stale`, open an issue.
Given §1's numbers (70–90% deprecated-API rate when fed outdated context), **a stale
experience store is worse than none** — it becomes the outdated-code prompt.

**Doctor 3 — Revalidation (the novel one).** Every record carries a repro script with
SWE-bench's verified **fail-to-pass discipline**: the repro must fail before the fix
and pass after, and pass-to-pass guards must hold. Doctor 3 re-executes repros in a
sandbox (container on the NAS or a desktop runner) on a schedule and on
dependency-bump events; a repro that stops reproducing → candidate `stale`; a fix that
stops working → reopen. Components all exist separately (Voyager's
execution-verified skill commits; SWT-Bench/Otter auto-generating repro tests from
issue text — [arXiv:2406.12952](https://arxiv.org/html/2406.12952v3),
[arXiv:2502.05368](https://arxiv.org/pdf/2502.05368)), but **no published system
wires them into a memory-store revalidation loop** — verified gap, and the single
most differentiating thing this project could build. Your repo already enforces
"every regression entry names its guarding test"; this is that rule, mechanized.

**Doctor 4 — Lifecycle (usage signals).** Reinforce retrieved-and-confirmed-helpful
records; decay accessibility of never-hit ones (archive, don't delete). Beware the
verified **"allergy problem"**: pure LRU evicts rare-but-critical entries — a trap hit
once a year may matter most. Salience class > recency. (MemoryBank's
Ebbinghaus model, [arXiv:2305.10250](https://arxiv.org/abs/2305.10250); Mem0's
eviction guidance.)

**Doctor 5 — Abstraction (periodic).** ExpeL/AWM-style: induce convention/workflow
cards from clusters of related episodes; the episodic records remain as evidence
links.

---

## 8. Evals: how you'll know it works

The field's verified template (ReasoningBank): same task suite, memory on/off,
measure success rate **and steps/tokens** (experience should make agents *faster*
even when success ties).

The eval this project should pioneer — because, verified, **no published suite
measures it**: **trap-avoidance regression evals**. For each recorded trap, a small
task that walks an agent toward the trap; score whether the store's injection
prevented the known failure. Your `REGRESSIONS.md` is a ready-made seed set —
e.g. "render `my_var_name` in agent prose" (entry 12), "make the budget row survive
restart" (the meter/ledger trap). Run A (no service) vs B (service): did B avoid the
documented dead-end? Harness-wise this is plain A/B with graded outcomes
(Anthropic's [evals guidance](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents));
LongMemEval's *knowledge-update* and *abstention* categories
([arXiv:2410.10813](https://arxiv.org/abs/2410.10813)) are the academic anchors for
"newer fact supersedes older" and "inject nothing when nothing applies."

Also instrument the live path: log every injection; let the agent (or you) mark
`confirmed_helpful`; that signal feeds Doctor 4. Treat any vendor memory benchmark
(LoCoMo/DMR) as marketing until independently replicated (§2.2).

---

## 9. Running it on the NAS

All facts below verified or flagged.

**Topology.** Source of truth = a git repo of markdown experience records (mirrored
off-NAS — that's your backup). Index = **derived and rebuildable**; index backup is
an optimization, not a requirement. This is Basic Memory's architecture and Qdrant's
own production guidance.

**Store.** For a personal corpus (10³–10⁵ records), skip the heavy vector DBs:
- **SQLite + FTS5 + sqlite-vec** in one file is the best NAS fit: pure C, runs on
  ARM/Pi, metadata columns, int8/binary vectors; brute-force search is fine at this
  scale (ANN landed in v0.1.10-alpha, Mar 2026)
  ([sqlite-vec](https://github.com/asg017/sqlite-vec) — verified; sqlite-vss is
  deprecated). Backup = copy one file.
- Alternative if you want a search UI: **Meilisearch** (LMDB, memory-mapped, runs
  with RAM ≪ dataset, ~100 MB resident; native hybrid) over Typesense (whole index
  in RAM). Qdrant works (135 MB–1.2 GB modes) but wants local SSD and **does not
  support NFS-backed storage** (per its installation guide — verified); ARM64 images
  have a history of page-size crashes on NAS hardware. DuckDB VSS persistence is
  explicitly not production-safe.
- **RAM envelope for the whole stack: ~1–1.5 GB** (MCP service 100–300 MB +
  Meilisearch ~100 MB if used + Tailscale ~30 MB) — fits a 2 GB Synology "+" model.
  Caveats: Docker is "+"-(Intel)-model-only on Synology; J-series Celerons lack AVX
  (some images SIGILL — test on the exact CPU); QNAP ARM boxes cap ~4 GB.

**Embeddings.** Do bulk embedding on your desktop (or API) at index-build time; on
the NAS, embed only the query string — fastembed/ONNX MiniLM-class (384-d) or
nomic-embed-text via Ollama are CPU-fine for single short strings (no published
Celeron benchmarks exist; N100 data says tens-to-hundreds of ms — acceptable,
unverified on Celeron). Matryoshka models let you run 256–512 dims to shrink the
index with graceful recall loss.

**Serving.** One container: streamable-HTTP MCP server (Go fits this repo's
toolchain) exposing `search_experience` / `get_experience` / `record_experience` +
the hook endpoint (plain HTTP POST returning trap cards; hooks don't need MCP).
Claude Code connects natively:
`claude mcp add --transport http loreline https://nas.tailnet.ts.net/mcp --header "Authorization: Bearer …"`
(verified syntax; [MCP docs](https://code.claude.com/docs/en/mcp)); `mcp-remote`
bridges stdio-only clients. **Tailscale Serve** gives TLS on a tailnet name with zero
port-forwarding (verified). Note claude.ai web connectors only do OAuth — Claude
Code/CLI is the practical client. Container updates: Watchtower is archived
(Dec 2025); use Diun/WUD in notify-only mode for stateful services.

**Doctors** run as scheduled jobs (NAS cron / CI on the mirror repo). Doctor 3's
sandbox can be a container on the NAS for cheap repros and a desktop/CI runner for
heavyweight ones.

---

## 10. Build vs. buy: what already exists

| Project | What it covers | Verdict |
|---|---|---|
| [Basic Memory](https://github.com/basicmachines-co/basic-memory) (AGPL, active — verified) | markdown SoT + SQLite index + hybrid FTS/FastEmbed search over MCP | **Closest prior art.** Covers §9's storage+serving. Does *not* have: experience schema, fingerprints, doctors, hooks push channel, evals. Candidate to build on or crib from. |
| mem0 / OpenMemory self-hosted | LLM-arbitrated memory ops | Heavy (Postgres+Neo4j), conversational-fact-shaped, benchmark-contested. Steal the ADD/UPDATE/NOOP write pipeline, not the product. |
| Zep/Graphiti | bi-temporal KG | Steal the invalidation model; full KG is overkill for few, stable entity types. |
| Context7 | live version-specific docs via MCP | Don't rebuild — it *is* Doctor 2's cross-check primitive. |
| Sentry | grouping/fingerprinting | The reference design for §4/§5; self-hosting Sentry itself is overkill. |
| Anthropic memory tool / Claude Code auto-memory / Codex memories | per-user file memory | Complementary (personal prefs), not a shared, validated, cross-repo experience store. |

**The genuinely unclaimed territory** (verified absent from the literature):
(a) sandbox **revalidation CI for memory records** (Doctor 3); (b) **trap-avoidance
regression evals** (§8); (c) **version-range × time-range validity** on engineering
experience records (OSV × Graphiti). Those three are the project.

---

## 11. Phased roadmap

**Phase 0 — corpus (a weekend).** Convert `REGRESSIONS.md` entries + ADR decisions +
retro learnings into ~50 experience records in the §4 schema, in a new git repo.
Manual curation; no service yet. (Also immediately useful as docs.)

**Phase 1 — read path.** SQLite FTS5+sqlite-vec index built from the records; Go
streamable-HTTP MCP server with `search_experience`/`get_experience`; Tailscale;
connect Claude Code. Measure: do pull-channel hits feel right?

**Phase 2 — push path.** `UserPromptSubmit`/`PreToolUse` hook → trap cards (k≤3,
relevance floor, fingerprint-gated). This is where "autopilot bug prevention"
actually starts happening.

**Phase 3 — write path + quarantine.** `record_experience` + end-of-session proposal
hook → quarantine branch → PR review gate.

**Phase 4 — doctors.** Dedup/reconcile (Mem0-style, delta-only), staleness
(Context7 cross-check), **revalidation sandbox (the novel one)**, lifecycle decay.

**Phase 5 — evals.** Trap-avoidance regression suite from the corpus; memory-on/off
A/B on steps + success; publish the methodology — nobody has.

---

## Appendix: verification notes

Claims double-checked adversarially against primary sources (2 verifier agents, 18
claims): 16 confirmed, corrections folded in above — notably (i) CodeRAG-Bench
favors dense over BM25 for code (hybrid framing corrected accordingly); (ii) the
Mem0-scored-Zep figure is ~66%, with ~58% belonging to a different leg of that
dispute; (iii) ReasoningBank headline deltas attributed to the Gemini-2.5-flash
configuration; (iv) "Power of Noise" hurt comes from *related* distractors (random
noise was harmless); (v) the 25–38% deprecated-API rate decomposes to 70–90% on
outdated-context prompts vs 9–18% on current ones; (vi) Qdrant's NFS prohibition is
in its installation guide (not the FAQ); (vii) sqlite-vec gained alpha ANN in
v0.1.10. Known soft spots flagged inline: NAS-CPU embedding latency (no Celeron
benchmarks), OpenMemory's product status, vendor-reported internal evals.

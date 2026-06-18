# Corpus growth, serving, validation & hardening — research synthesis

> **Provenance.** Synthesized 2026-06-18 from a parallel fan-out of **off-pool**
> models (kept off the Anthropic weekly pool by design): xAI **Grok** (live web +
> repo-grounded) for data-sourcing, prior-art, the validation-engine design, and
> the threat model; NVIDIA NIM (deepseek/qwen via `code-exec`) attempted for the
> enhancement angle. Claude only orchestrated + synthesized. Findings below are
> distilled facts and recommendations; verify volatile details (rate limits/ToS)
> before relying on them. Complements [PLATFORM_RESEARCH.md](PLATFORM_RESEARCH.md)
> (executor platform) and [SECURITY_ANALYSIS.md](SECURITY_ANALYSIS.md) (Tier A/B
> threats), which this confirms and extends. Decisions live in
> [ADR-0011](../adr/ADR-0011-corpus-growth-and-validation-engine.md).

## 1. What we are (the positioning that drives the rest)

twiceshy's value is **not** "a knowledge base" — an LLM already has one in weights.
The value is the three things weights can't give a stateless model:

1. **Freshness** — facts after the training cutoff (new versions, new CVEs, "fixed in vN.M").
2. **Validation** — a claim *executed and proven in a clean container*, not pattern-matched.
3. **Negative knowledge** — the dead-ends ("this error means X, don't chase Z"; "this test
   passes for the wrong reason"). Training data under-represents anti-knowledge; it's exactly
   what agents waste turns on.

One-liner: **the pre-flight landmine check for coding agents** — consulted before they pick a
design, write a test, debug an error, or call an API.

## 2. Data acquisition + licensing (what to bring in)

Today the importers emit a `//go:embed` curated *snapshot* — a static seed, not a feed.
The design (ADR-0003) always intended live external sources; these are the concrete ones,
with the licensing posture per source (facts are free — *Feist*; never copy long prose):

| Source | Endpoint / mechanism | Rate limit | License (for distilled facts) | Extract |
|---|---|---|---|---|
| **OSV.dev** | `api.osv.dev/v1/query`, `/querybatch`; GCS bulk `…/<ecosystem>/all.zip` + incremental CSV | none (32 MiB resp) | composite CC-BY/MIT/BSD; OK w/ attribution | id, package+ecosystem, **affected + fixed ranges**, aliases (CVE/GHSA), severity |
| **GitHub Advisory (GHSA)** | REST `/advisories`, GraphQL `securityVulnerabilities` | 5k/hr auth | **CC-BY-4.0** (attribution = link) | GHSA/CVE, `vulnerable_version_range`, `first_patched_version`, CVSS, CWE |
| **deps.dev** | `api.deps.dev/v3alpha/...`, batch ≤5000 | soft | Apache tooling; OK | per-version **`isDeprecated` + `deprecatedReason`**, license, OSV links |
| **NVD/CVE v2** | `services.nvd.nist.gov/rest/json/cves/2.0` | 5/30s (50 w/ key) | public domain (display NVD notice) | CVE, CVSS, CPE ranges, refs |
| **endoflife.date** | `endoflife.date/api/v1/{product}.json` | generous | MIT | EOL/support/release cycles (our D2 seam) |
| **GitHub Releases/Issues** | REST/GraphQL | 5k/hr auth | facts free | "fixed in vN", breaking-change/migration notes, closed-by-PR |
| **ecosyste.ms** | `{svc}.ecosyste.ms/api/v1` | ~5k/hr (polite pool) | OpenAPI CC-BY-SA; facts OK | package/repo/advisory metadata at scale |

**Start order:** OSV.dev → GHSA → deps.dev → endoflife.date → GitHub Releases/Issues.
OSV first: explicit *fixed* ranges (perfect for repro validation), no rate limit, bulk dumps.

**How others source it:** Renovate = OSV + GHSA + endoflife + registries; Dependabot = GHSA
(reviewed only); Snyk/Socket = proprietary curation on top of public feeds. None serve
*execution-validated* gotchas — that's our gap to own.

## 3. Prior art + serving to LLMs

- **Context7** — version-specific docs over MCP (`resolve-library-id` + `query-docs`),
  pre-processed + reranked. Unvalidated docs; our differentiator is validation + dead-ends.
- **GitChameleon** (arXiv 2507.12367) — a published benchmark of *version-conditioned*
  code-gen with executable tests. Two gifts: (a) **evidence** retrieval helps — RAG@k=3 gives
  ~10pt absolute gains, and SOTA models still fail 40%+ *even with docs* (the problem is real);
  (b) a **blueprint for our #0005 eval**.
- **Sourcegraph/Cody** — deliberately moved **away from pure embeddings to BM25 + signals** at
  scale (reliability, privacy, no external calls) → independent validation of our embedding-free
  hot path (ADR-0009).
- **Snyk / Socket** — curated security patterns; Socket leans on behavioral/static analysis.

**Serving best practices (converge on our schema):** lexical/BM25 + error **fingerprinting**
first (exact symptom match wins); dense optional; **k=3–5** (agents get "lost in the middle");
return structured records (symptom / affected-versions / gotcha / **action (imperative +
contrast)** / repro / fix / provenance / **freshness**); **tier by validation status**; present
as *hints with provenance*, not truth. Anti-FP checklist = Barnett et al. "Seven Failure Points
of RAG" (arXiv 2401.05856). Validation status + `last_verified` are the highest-value rerank
signals.

## 4. The validation engine (the moat) — confirms & refines PLATFORM_RESEARCH §2

A record reaches `validated` only when a runnable repro is **executed in clean ephemeral
containers across a version matrix**, proving fail-pre-fix → pass-post-fix and **empirically
deriving** the `applies_to[].versions.{introduced,fixed}` boundary.

- **Isolation:** **gVisor (runsc) primary** — drop-in Docker runtime, big kernel-surface
  reduction, overhead irrelevant for batch; plain runc is **not** sufficient for the promotion
  gate. Firecracker/Kata as the hardening upgrade path; nsjail/bwrap for single-process.
- **Two phases:** *prepare* (egress only to an allowlist via per-ecosystem proxies — Athens/Go,
  Verdaccio/npm, devpi/PyPI; content-addressed cache) → *execute* (`--network=none`).
- **Version matrix:** official images pinned **by digest** + per-ecosystem `repro-*` base images;
  drive the repro across versions, coarse-grid majors then refine around the transition to find
  the fail→pass boundary; cache by `(repro_hash, image_digest, version, config)`.
- **Determinism:** pinned digests, `SOURCE_DATE_EPOCH`, fixed seeds, tmpfs, minimal env
  whitelist; **multi-run consensus** for flaky handling; exit `75` = skip (never "stale").
- **Promotion (unchanged trust boundary):** the engine is **delta-only / report-only** (like
  D2); it emits a Finding + an **attestation** (repro sha, image digest, exit, output hashes,
  corpus git sha); a human flips `status`/`validated_at` in the PR. Never auto-promote.
- **Freshness:** scheduled + event-driven re-validation re-runs the fix side on new versions;
  regression → propose `stale`.

## 5. Hardening (the 3 must-haves before running ANY untrusted repro)

Confirms SECURITY_ANALYSIS; reuse the existing `claude-sandbox` broker pattern.

1. **Broker-enforced ephemeral container, hardcoded policy** — dedicated non-`ori` runner,
   `--read-only` + sized tmpfs only, `--cap-drop=ALL`, `no-new-privileges`, mem/pids/cpu hard
   limits, **`--network=none` (execute phase)**, no host Docker socket / secrets / persistent
   writable paths; repro copied in read-only.
2. **A watchdog guaranteeing termination + cleanup** — cgroups + `timeout` + `docker kill` +
   `rm -rf`, owned by the broker, survives fork bombs / ignored signals / hangs.
3. **Trust boundary is sacred** — only run repros that reached the git corpus (post-PR + ingest
   `internal/screen`, which must also screen the **repro script content**); success yields an
   attestation only; human flips `validated`. Records with `security_flags` never run and never
   promote.

## 6. Enhancing — coverage, quality, legal reach

- **The Stack Overflow reframe (the legal unlock).** SO prose is CC-BY-SA and its dump bans
  commercial/LLM use, so ADR-0003 excluded it *as a text source*. But facts aren't copyrightable
  (idea/expression; *Feist*). So: **harvest the knowledge, never the text** — use the model to
  *re-derive* the fact and **author an original test/repro from scratch**, covering the same
  problem without ingesting one byte of SO. Validation (Section 4) makes the output both *ours*
  and *correct*. This is a licensing decision for Horia (extends ADR-0002/0003) — see ADR-0011.
- **Coverage by mining LLM failures.** Run models on coding tasks; capture wrong debugging
  paths / removed-API use / tautological tests; distill into records. Coverage then grows along
  the exact axis of "what LLMs get wrong" — self-targeting at the moat.
- **Retrieval.** Keep BM25+fingerprint as the hot path; revisit dense only if the #0005 eval
  shows it lifts symptom/error recall. Dedup/cluster near-duplicates; rerank by validation
  status + version match + recency.
- **Freshness loops.** endoflife.date staleness (D2) + repro re-validation (D3) + supersede on
  new versions.

## 7. Evidence the whole thing is worth it

GitChameleon shows retrieval of version-specific knowledge gives measurable, repeatable gains
and that the failure is real and large; RAGFix/repair literature shows retrieving "known issues"
helps, most for smaller models. The bounded factor is always **retrieval quality** — hence the
emphasis on precision, validation, and freshness over raw volume.

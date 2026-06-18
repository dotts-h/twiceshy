# Security analysis — twiceshy (pre-deploy)

> Threat-model + prioritized defenses for twiceshy, the experience service.
> Research run **2026-06-18 on Gemini** (off the Anthropic pool) across four
> facets; **triaged here against twiceshy's actual code** (✅ = already done,
> ⚠️ = gap to close). Drives the pre-deploy security epic (#0009) and the
> public-release epic (#0010). The raw Gemini analysis is the input; the
> status/priority calls below are ours.

## Why this matters

twiceshy's content is **injected into downstream coding agents** (push) or
returned to them (pull). A record is therefore an *instruction-adjacent* artifact
in someone else's LLM context: a poisoned or secret-laden record is a stored
prompt-injection / data-exfiltration / secret-leak vector. The owner's principle
governs the ingestion gate: **a documented-but-quarantined hazard beats ingesting
harmful content or secrets.**

Two deploy tiers with different bars:
- **Tier A — single-tenant (the author's own deploy, near-term).** Gated on
  #0009: ingestion gate, injection-safe rendering, app hardening, maintenance.
- **Tier B — public/multi-tenant (paid, future).** Additionally gated on #0010:
  tenant isolation, per-tenant auth, trial + anti-abuse.

---

## Facet 1 — Prompt-injection / content-integrity via records

**Attacks:** malicious-instruction injection ("ignore previous instructions…"),
data-exfiltration payloads, malicious code in a `fix`/`guard`, agent-side DoS
(huge/regex-heavy records), context-overload pushing out system instructions.

| Pri | Defense | twiceshy status |
|---|---|---|
| P0 | Render injected/returned records as **data, not instructions** — fenced/delimited block, never bare prose spliced into the prompt; the consuming hook treats it as data | ⚠️ gap — define in the push-card renderer (#0002) and the pull `get/search` response shape |
| P0 | **Only `validated` records reach PUSH**; pull responses label `status` | ✅ invariant locked (ADR-0001 §6); enforce in the #0002 renderer |
| P1 | **Length/shape caps** on injected content (k≤3, per-card char cap) | ⚠️ k≤3 done in retrieval; add per-card/per-field render caps |
| P1 | **Imperative/payload heuristics** — flag records containing "ignore previous", `curl\|bash`, exfil URLs, long base64/hex; block from PUSH | ⚠️ overlaps Facet 2 ingestion gate |
| P2 | Provenance/trust-tier visible to the consumer | ✅ provenance + status are first-class fields |

## Facet 2 — Preventing ingestion of secrets & harmful content  ← owner's core concern

**Attacks:** accidental secret ingestion (API keys/tokens in `error_signatures`
or body via `record_experience`), malicious code in a `fix`/`guard`, PII, and
poisoned upstream advisory data.

| Pri | Defense | twiceshy status |
|---|---|---|
| P0 | **Ingestion-time secret scan** (gitleaks-style ruleset: known token shapes + entropy) on every `record_experience` draft **and** importer output, **before write**, in `internal/ingest` (before/at `Prepare`) | ⚠️ **gap — primary P0** (gitleaks runs in CI on the repo, not at ingest) |
| P0 | **Harmful-code heuristics** on `resolution`/`guard`/body (`curl\|bash`, `nc`, reverse-shell, `/dev/tcp`, base64→sh) | ⚠️ gap |
| P1 | **PII detection** (emails, hostnames, IPs) in text fields | ⚠️ gap |
| P0 | **Policy on hit = reject or quarantine-with-flag**, never silently ingest; add a `provenance.security_flags` field so a hit is *documented* and blocked from promotion | ⚠️ gap — additive schema field + gate |

The gate lives in the ingest pipeline (`internal/ingest`), shared by both the
agent write-path and the importer, so nothing reaches disk unscreened.

## Facet 3 — Application security (the Go MCP service)

| Pri | Defense | twiceshy status |
|---|---|---|
| P0 | Timing-safe bearer compare; no unauthenticated mode | ✅ `crypto/subtle.ConstantTimeCompare`; `TWICESHY_TOKEN` required |
| P0 | FTS5 `MATCH` input escaped (query-language injection) | ✅ done (exp-0001, `TestSearchQuoteEscapesFTS5Input`) |
| P1 | **Request body size cap** on `record_experience` (`http.MaxBytesReader`) | ⚠️ gap |
| P1 | FTS query `LIMIT` + `context` timeouts + result caps; **rate limiting** | ⚠️ partial (`maxQueryBytes` + k≤3 cap exist; add timeouts/rate-limit) |
| P1 | **Path-write safety** — record paths are derived (`buildPath`+`slugify`, `exp-NNNN`), not user-supplied; add a defensive "stays under corpus root" assertion | ⚠️ low risk (paths derived), add belt-and-suspenders |
| P2 | Error-message hygiene (no internal paths/stack to clients); container non-root + read-only FS | ⚠️ gap — deploy hardening |
| P0 (Tier B) | **Tenant isolation** — `tenant_id` on every query + per-tenant storage; per-tenant tokens | ⚠️ **#0010** (not needed for single-tenant Tier A) |

## Facet 4 — Ongoing security maintenance / supply chain

| Pri | Defense | twiceshy status |
|---|---|---|
| ✅ | `govulncheck` + `gitleaks` in CI | ✅ done (`.forgejo/workflows/security.yml`) |
| P1 | **Automated dep updates** (Renovate/Dependabot for `go.mod`) → PRs → CI | ⚠️ gap |
| P1 | **Dogfood OSV/GHSA self-monitoring** — match twiceshy's own `go.mod` against the advisory data it ingests; alert on a hit (uses the product on itself) | ⚠️ gap — neat, leverages #0007 |
| P2 | SBOM (syft/cyclonedx) + release signing (cosign/Sigstore) | ⚠️ gap — release hardening |
| P2 | A short **security checklist in the PR template** (new endpoint? secrets/PII? file I/O? deps? tenant?) | ⚠️ gap |

---

## Pre-deploy checklist

**Tier A — before the single-tenant (own) deploy → epic #0009**
1. Ingestion secret-scan gate (P0) — `internal/ingest`, before write.
2. Harmful-code heuristics at ingestion (P0).
3. On-hit policy: quarantine-with-flag via additive `provenance.security_flags` (P0).
4. Injection-safe rendering/framing for pull responses + the push-card renderer (P0; lands with #0002).
5. App-hardening gaps: request body cap, query timeouts, rate limiting, path-under-root assertion, error hygiene, non-root container (P1).
6. Maintenance: Renovate + OSV self-dogfooding + PR security checklist (P1).

**Tier B — before any public/multi-tenant release → epic #0010**
7. Tenant isolation (`tenant_id` on all queries + per-tenant storage) — non-negotiable.
8. Per-tenant auth (scoped tokens, not one static bearer).
9. Free-trial window + anti-abuse (prevent multi-account bypass).
10. SBOM + release signing.

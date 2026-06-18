# twiceshy — Contribution Terms (inbound licensing)

> **STATUS: DRAFT — NOT legal advice; requires review by a qualified lawyer before
> it is used or presented to any contributor.** Authored 2026-06-18 to capture intent
> so the knowledge isn't lost (per the build discussion). It is the basis for two
> things: the open-source **CLA** required by
> [ADR-0002](../adr/ADR-0002-licensing-strategy.md) (code), and the **contribution
> clause of the hosted-service Terms of Service** required by
> [ADR-0011 §7](../adr/ADR-0011-corpus-growth-and-validation-engine.md) (records via
> the MCP service). The broader consumer ToS (acceptable use, liability, privacy,
> SLA) is a separate document for go-live. **Placeholders in [brackets] are legal's.**

## Why this exists

twiceshy's commercial viability depends on the corpus staying **license-clean** so
records can be included in **separately-licensed commercial packs**
(ADR-0002). Once dirty or encumbered content is *shipped* in a commercial pack the
damage is irreversible (distributed copies + possible copyleft attachment). Both code
and record contributions are therefore a licensing-cleanliness vector and must be
gated by a clear inbound grant + originality representations **before** acceptance.

## 0. Definitions

- **"Project" / "We"** — [legal entity TBD] operating twiceshy.
- **"You"** — the individual or entity submitting a Contribution (including via an
  automated agent acting on your behalf).
- **"Contribution"** — anything You submit to the Project: **Code** (PRs to the
  AGPL repository) and/or **Records** (experience records proposed via the
  `record_experience` MCP tool, the hosted service, or PRs to the corpus).

## 1. Scope

These terms apply to every Contribution. For Records submitted through the hosted
service, acceptance of these terms is a condition of submission (presented in the
service ToS / at submission time). For Code, You agree to these terms before your
first PR is merged (the CLA gate of ADR-0002).

## 2. License grant

You grant the Project a **perpetual, worldwide, non-exclusive, royalty-free,
irrevocable, transferable, and sublicensable** license to use, reproduce, modify,
adapt, prepare derivative works of, publicly display, distribute, and **relicense**
Your Contribution and derivatives, **including under different terms in commercial
or proprietary experience packs**. You also grant a [patent license, Apache-2.0 §3
style] to any of Your patent claims necessarily infringed by Your Contribution as
incorporated. You retain ownership of Your Contribution; this is a license, not an
assignment. [Legal to confirm: license grant vs. assignment vs. dual.]

## 3. Your representations (the cleanliness gate)

You represent and warrant that:

1. **Originality / rights.** The Contribution is Your own original work, or You have
   all rights necessary to submit it under these terms.
2. **No third-party expression.** It contains **no copied or closely-paraphrased
   third-party text, prose, or code** — no Stack Overflow / documentation / blog
   prose, no non-trivial third-party snippets. Records are **distilled facts in Your
   own words** plus **original tests** authored by You (the facts-only rule of
   [ADR-0011 §5](../adr/ADR-0011-corpus-growth-and-validation-engine.md);
   ADR-0003 §4). Facts are not copyrightable; expression is — submit only the former.
3. **Not encumbered.** It is not subject to any license that would attach obligations
   to the Project or its packs (e.g. CC-BY-SA, GPL/AGPL-as-content, NC/ND), and is
   not covered by any agreement (e.g. an employer IP policy) that would prevent the
   grant in §2. [Legal: employer-CLA path.]
4. **No secrets / PII / harmful content.** It contains no credentials, secrets,
   personal data, confidential or export-controlled information, or intentionally
   harmful code.

## 4. Records — additional terms

- Records are **distilled facts + original tests**, never third-party expression (§3.2).
- Every submitted Record is **born quarantined** and is **not** treated as authoritative.
  It passes an ingestion **screen** (license/safety) and, for promotion to `validated`,
  the **execution-validation harness** + human review. We may **accept, reject, modify,
  quarantine, supersede, or remove** any Record at our sole discretion. Submission does
  not guarantee inclusion or retention.
- An automated agent may submit on Your behalf only if You have authorized it and these
  terms bind You for what it submits.

## 5. Attribution / moral rights

To keep packs clean and freely relicensable, You **waive any requirement of
attribution** for Your Contribution (we may, but need not, credit contributors), and
[to the extent permitted by law] waive moral rights in the Contribution. We will not
falsely attribute a Contribution to a third-party source.

## 6. Provenance (DCO-style)

Each Contribution must carry a sign-off certifying these terms — a Developer
Certificate of Origin–style "Signed-off-by" on Code PRs, and an explicit accept at
submission time for service Records (recorded in `provenance.source`). [Legal: exact
mechanism + record-keeping.]

## 7. Disclaimer & liability

The Contribution is provided "as is." [Warranty disclaimer + limitation of liability +
contributor indemnity for breach of §3 — legal to draft.]

## 8. Governing law & changes

[Governing law / venue — legal.] We may update these terms; the version in effect at
submission governs that Contribution.

---

*This DRAFT exists to preserve the design intent. Nothing here is in force until it is
legal-reviewed and formally adopted; accepting outside Contributions into a commercial
corpus is gated on that adoption (ADR-0011 §7).*

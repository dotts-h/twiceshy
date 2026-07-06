---
id: 0131
title: Alpha follow-ups from PR #511/#512 review: quota debit ordering, over-quota counter inflation, signup per-IP cap behind a proxy
status: open
severity: medium
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Three accepted-not-blocking findings from the #0125/#0127 reviews, plus one pre-existing test flake, batched for a single cleanup pass:
1. (PR #511 review, LOW) Daily quota is debited before the shared GLOBAL rate limiter: tenantAuth calls CountTokenCall, then withRateLimit can still 429 the request — a noisy tenant saturating the global bucket both DoSes other tenants and burns their quotas on never-served requests. Fix: move the debit after the global limiter, or partition the global limiter per tenant.
2. (PR #511 review, LOW) Over-quota requests keep incrementing token_usage.calls (count-then-check design), so calls_today inflates unboundedly past the quota for the rest of the UTC day. Blocking behavior is correct; the stored counter is misleading in ListTokens/dashboards.
3. (PR #512 review) The signup per-IP daily cap keys on RemoteAddr. Behind the #0129 reverse proxy every client shares the proxy's IP → 3 signups/day TOTAL. Needs trusted-proxy X-Forwarded-For handling (parse XFF only when RemoteAddr is the configured proxy), never blind XFF trust (spoofable).
4. (pre-existing, unrelated to those PRs) scripts/metrics-digest.test.sh fails on main: "FAIL healthy digest missing push section" — verified present with a clean tree on 2026-07-06. The script-level test drifted from the digest's current output; not part of make ci so CI stays green.
## Notes
1–3 land together in one small PR on the token/signup layer (tests first: cross-tenant quota-burn repro, counter-stops-at-quota assertion, XFF-with-trusted-proxy table). 4 is a scripts-only fix. Parent: #0124 (ADR-0030).

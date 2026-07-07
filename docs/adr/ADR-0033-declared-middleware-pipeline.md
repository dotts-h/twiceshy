# ADR-0033: The middleware chain is a declared pipeline with checked constraints

- **Status:** Accepted (2026-07-07; horia approved the 2026-07-07 architecture
  review's proposal).
- **Related:** #0131 finding 1 (the quota-debit misordering this class of
  change prevents), ADR-0032 (removed one ordering invariant; this removes the
  mechanism that breeds them), #0125 (the sprint that introduced the chain),
  issue 0138.

## Context

The HTTP middleware chain in `server.New` is built as nested closure calls
across three separate statements, and its correctness rests on at least five
ordering invariants enforced only by comments: the access log outside auth
(which required the `tenantHolder` mutable side-channel, because context
values don't flow upstream), auth before the global rate limiter (a 401 must
not burn the shared bucket), the daily-quota debit after the global limiter
(#0131 finding 1), the quota stage below auth (reordered, `TenantFromContext`
returns `""` and quota silently passes — fail open, unchecked), and `/signup`
hand-wired on the outer mux sharing the limiter closure while skipping the
timeout and body-cap stages.

The sprint history is the evidence this is fragile in practice, not in
theory: the chain was reordered three times in seven days (original #0125
order → same-day log-placement fix → the #0131 quota-placement fix), plus a
fix-on-the-fix. Nothing structural prevents reorder number four.

## Decision

The chain becomes **one ordered declaration** validated at construction.

1. **Stage list.** `New` builds the chain from a single literal slice of
   stages: `{name, requires []string, after []string, wrap func(http.Handler)
   http.Handler}`. `requires` names DATA a stage consumes that an earlier
   stage must provide (the quota stage requires `tenant`, provided by auth);
   `after` names pure ORDER constraints (`daily-quota` after `global-rate-limit`;
   `global-rate-limit` after `tenant-auth`, encoding the 401-shielding choice) —
   an `after` reference binds only when both stages are present in a chain.
2. **Checked at construction.** Composing a chain validates every `requires`
   and `after` against the stages already composed; a violation makes `New`
   return an error. A misordering becomes a startup failure caught by any
   test that constructs the server — never a silent fail-open in production.
3. **One per-request state.** A `reqState` struct (tenant id, response
   status) is seeded once at the top of the chain under a single context key;
   auth writes the tenant into it, the access logger reads it after the
   handlers return. The `tenantHolder` side-channel and its extra context key
   are deleted. `TenantFromContext` remains the public accessor.
4. **`/signup` joins the declaration** as its own chain (global rate limit →
   timeout → body cap → handler), sharing the same limiter instance and
   gaining the timeout bound it currently lacks. `/healthz`/`/readyz` stay
   bare by declaration.
5. **A chain-contract test** pins the load-bearing behaviors independent of
   the wiring: an unauthenticated request is 401'd without consuming the
   global bucket, a global 429 debits no tenant quota, an authenticated tok_
   request debits exactly once, and the access log carries the tenant for
   authed requests and logs rejected ones. Plus a construction test: a
   deliberately misordered declaration must fail `New`.

External behavior is byte-identical; this is wiring, not policy.

## Options considered

- **Keep nested calls + comments (status quo) — rejected:** the base rate is
  three reorders and two escaped bugs in one week; comments demonstrably do
  not hold this.
- **Type-state / generics-enforced ordering — rejected:** encoding "auth
  before quota" in the type system fights `net/http`'s `http.Handler` idiom
  and buys compile-time failure where startup-time failure (caught by every
  test run) is sufficient and far simpler.
- **Adopt a router/middleware framework — rejected:** dependency budget
  (CONVENTIONS.md); the need is ~60 lines of declaration + validation, not a
  framework.
- **Declared stages with checked `requires`/`after` (chosen).**

## Consequences

- Ordering intent moves from comments into a machine-checked declaration; the
  next stage added (write-path enablement will add one) states its
  constraints or fails to boot.
- The middleware functions themselves stay plain idiomatic
  `func(http.Handler) http.Handler` — only the composition changes.
- One context key and the `tenantHolder` bridge are deleted; per-request
  bookkeeping has a single home.
- Slight indirection in `New`; the declaration reads as documentation of the
  chain, replacing the comment block that used to describe it.

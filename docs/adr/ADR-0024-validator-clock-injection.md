# ADR-0024: Inject the record validator's clock instead of a package-level mutable global

- **Status:** Proposed (2026-06-23) — surfaced by the post-#0086 architecture audit;
  claude proposed and authored. Awaiting **horia**'s decision (it touches the public
  `record.Validate` surface).
- **Related:** [CONVENTIONS.md](../CONVENTIONS.md) ("No package-level mutable state;
  dependencies enter through constructors. Context-first signatures for anything that
  does I/O."); [ADR-0005](ADR-0005-stable-seams.md) (the injected-clock seam every
  sibling already follows); issue **#0050** (the validated↔stale desync guard whose
  boundary this makes testable).

## Context

`internal/record/record.go:745` declares a **package-level mutable** clock:

```go
var nowUTC = func() time.Time { return time.Now().UTC() }
```

read by `validateProvenance` (record.go:657) for the `status: validated` + past-`valid.until`
desync guard (#0050). This is the lone outlier in the codebase's clock discipline: every
sibling injects its clock through a constructor or option — `doctor.NewStaleness(src, now)`,
`repro.NewRevalidator(..., now)`, `ingest.WithEOLNow(now)`. The validator instead reaches
for a global.

Two concrete costs:

- It violates the stated convention ("no package-level mutable state").
- The `until == today` boundary — the exact instant the #0050 guard is most delicate about,
  because it must agree with the staleness doctor's raw `time.Now().UTC()` — is **untestable
  from outside the package**. A test that mutated the global to pin `now` would also race
  under `make ci`'s `-race`, so the current suite only covers far-past / far-future
  `valid.until`, never the `today` / `yesterday` / `tomorrow` edges.

Severity is realistically **minor**: the field is unexported, read once, and never mutated
at runtime — there is no live bug. The payoff is convention compliance plus the boundary
test the guard deserves, not a defect fix.

## Decision (proposed)

Remove `nowUTC`. Thread an explicit instant through the validation call chain:

- Add `ValidateAt(r *Record, now time.Time) error`; keep `Validate(r) =
  ValidateAt(r, time.Now().UTC())` so the common call site is unchanged.
- Thread `now` into `(*Record).validate(now)` → `validateProvenance(fail, now)`, replacing
  `nowUTC()` at record.go:657.
- Ship with the `until == today` / `== yesterday` / `== tomorrow` boundary test the current
  far-past/far-future cases miss (this subsumes the deferred `T-record-8` hardening item).

A smaller variant that keeps the public surface unchanged — an unexported `validate(now)`
with `Validate` passing the wall clock, and no new exported `ValidateAt` — is acceptable if
minimizing the public API is preferred. That variant needs no ADR-level decision and could
be done directly; this ADR exists because the `ValidateAt` form changes the public surface.

## Consequences

- **+** Removes the only package-level mutable state in `record`; brings the validator in
  line with the rest of the codebase's injected-clock seam.
- **+** Makes the #0050 desync boundary deterministically testable, closing a real coverage
  gap on a guard that protects against validated↔stale flip-flop.
- **−** Touches a widely-called public function (`record.Validate`); every internal caller
  stays source-compatible (it delegates), but the change is API-visible and so wants a
  decision rather than a silent edit.
- **−** Small, self-contained — low blast radius — so it can land in a single focused PR
  with its boundary test.

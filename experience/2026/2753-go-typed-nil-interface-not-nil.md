---
schema_version: 1
id: exp-2753
kind: trap
status: validated
title: "A nil pointer returned as a Go error is not nil — the typed-nil interface trap"

symptom:
  summary: >
    A Go function whose return type is `error` returns a custom error via a
    pointer (`*ValidationError`), leaves that pointer at its zero value on the
    success path, and returns it. The idiomatic caller check `if err != nil`
    then takes the error branch even though nothing went wrong: a handler 500s
    on success, a retry loop never terminates, a `defer`ed `if err != nil`
    cleanup fires spuriously. There is no panic and no error message — the value
    even prints as `<nil>` — only the wrong branch.

applies_to:
  - ecosystem: "Go"

resolution:
  root_cause: >
    An interface value in Go is a pair of a concrete *type* and a *value*, and it
    equals `nil` only when BOTH halves are nil. Assigning a nil `*ValidationError`
    to an `error` stores a non-nil type (`*ValidationError`) with a nil value, so
    the interface is non-nil. `err == nil` is therefore false — and so is
    `errors.Is(err, nil)`, which is defined as `err == nil`. `%v` formatting
    printing `<nil>` and call-site checks both mask it.
  fix: >
    Decide nil-ness at the RETURN site, not the call site: in the no-error path
    return the untyped `nil` literal (`return nil`), never a nil typed pointer. If
    a function must hold the concrete pointer in a variable, guard the return
    (`if e == nil { return nil }; return e`). Keep error-returning signatures
    typed as `error`, not `*ConcreteError`, so a typed nil can never reach a
    caller. `go vet` nilness and golangci-lint's `nilerr`/`nilnil` catch common
    shapes, but the authoring discipline is the real fix.
  dead_ends:
    - tried: "adding `if err != nil` checks at the call site"
      why_it_failed: >
        The interface is already non-nil before the caller looks at it; no
        call-site check can rescue a typed-nil return. The fix belongs at the
        return site, not the call site.
    - tried: "comparing with `errors.Is(err, nil)` instead of `err == nil`"
      why_it_failed: >
        `errors.Is(x, nil)` is defined as `x == nil`, so it is exactly as false
        for a typed nil. It unwraps nothing useful here.
    - tried: "returning the concrete pointer type (`func() *ValidationError`) and letting callers convert"
      why_it_failed: >
        It just moves the trap to every caller: the moment the result is assigned
        to an `error` variable, the typed-nil interface is recreated. The signature
        must be `error` and the return must be a literal nil.

guard:
  repros:
    - path: "experience/repro/2753-go-typed-nil-escape.sh"
      kind: positive
      label: "typed-nil error != nil (trap) while literal-nil error == nil (escape)"
    - path: "experience/repro/2753-go-typed-nil-deadend.sh"
      kind: negative
      label: "call-site nil check stays broken against a typed-nil callee"

provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-23
  validated_at: 2026-06-23
  valid: { from: 2026-06-23, until: null }
  source_license: "none (authored, internal-only)"
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

You write a function that returns `error`, model failures with a custom type via
a pointer (`*ValidationError`), and on the success path leave the pointer at its
zero value and return it. Callers do the idiomatic `if err != nil { ... }` — and
take the error branch even though nothing went wrong. A handler returns 500 on a
successful request; a retry loop never terminates; a `defer func() { if err != nil
... }()` fires its rollback spuriously. There is no panic and no error string,
and `fmt.Printf("%v", err)` prints `<nil>`, so the usual debugging reflexes find
nothing.

## Why it happens

An interface value in Go is a pair: a concrete *type* and a *value*. It is equal
to `nil` only when **both** halves are nil. Assigning a nil `*ValidationError` to
an `error` stores the type `*ValidationError` together with a nil value — the type
half is non-nil, so the interface is non-nil. Hence `err == nil` is false. The
same is true through `errors.Is(err, nil)`, which is defined as `err == nil`. The
`<nil>` you see from `%v` is the *value* half formatting itself; the interface
around it is very much not nil.

## The escape

Decide nil-ness at the **return site**, not the call site. In the no-error path
return the untyped `nil` literal (`return nil`), never a nil typed pointer. If a
function must keep the concrete pointer in a variable, guard the return:
`if e == nil { return nil }; return e`. Keep error-returning function signatures
typed as `error`, not `*ConcreteError`, so a typed nil can never escape to a
caller in the first place. `go vet`'s nilness pass and golangci-lint's `nilerr` /
`nilnil` linters catch the common shapes; treat them as a backstop, not the cure.

## Scope

This applies to **every** Go version — it is intrinsic to how interfaces
represent a value, not a defect scheduled to be fixed. The struct-pointer case is
the common one, but the same shape bites with a typed-nil slice, map, function, or
channel assigned to an interface. The companion repros prove it under the Go
matrix: the positive shows a typed-nil error comparing `!= nil` while a literal-nil
error compares `== nil`; the negative shows the tempting call-site nil check
staying broken against a typed-nil callee.

## Provenance (ADR-0011 §5)

This record is the worked example for the internal authoring path (#0024). Its
*topic* — that a typed nil is a non-nil interface — is discussed widely in public,
but the description and both tests here were re-derived from the Go language
specification and written from scratch; no third-party text or snippet was
ingested, quoted, or paraphrased. Provenance is recorded honestly as
`authored+validated`: `source_license: none (authored, internal-only)` and no
`source_url`. It is cleared for the **internal** corpus only — the pack builder
mechanically keeps it out of commercial packs until a real legal review
(ADR-0011 §5).

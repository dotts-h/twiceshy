---
schema_version: 1
id: exp-0006
kind: trap
status: validated
title: Go ServeMux "POST /path" lets other methods fall through to the catch-all, not a 405
symptom:
    summary: >
        With a Go 1.22+ http.ServeMux registering both a method-specific pattern
        ("POST /push") and a catch-all ("/"), a non-POST request to /push does
        NOT get a 405 — it fails to match the method-specific pattern and falls
        through to the "/" handler. Any in-handler `r.Method != POST` check then
        becomes unreachable dead code.
    error_signatures:
        - "405"
        - "method not allowed"
applies_to:
    - ecosystem: Go
      package: net/http
      runtime: { go: ">=1.22" }
resolution:
    root_cause: >
        Go 1.22 method-aware ServeMux matches "POST /push" only for POST. A
        GET/PUT/DELETE to /push does not match it and, because "/" also matches,
        is routed to the catch-all handler instead of returning 405.
    fix: >
        If you want a clean 405 for wrong methods on a path, register the path
        WITHOUT a method ("/push") and check the method inside the handler
        (return 405 for non-POST). That keeps the method check live and the
        contract explicit. Otherwise be aware the catch-all swallows other methods.
    dead_ends:
        - tried: 'mux.HandleFunc("POST /push", h) plus an in-handler r.Method != POST guard'
          why_it_failed: "the guard is unreachable; non-POST never reaches the handler — it routes to the catch-all and returns its (opaque) response"
guard:
    guarding_test: "TestPushNonPostReturns405: GET/PUT/DELETE /push each return 405."
provenance:
    source:
        author: claude
        session: twiceshy-0002-2026-06-18
    recorded_at: "2026-06-18"
    validated_at: "2026-06-18"
    valid:
        from: "2026-06-18"
---

Found reviewing the #0002 push endpoint. The mux had `mux.HandleFunc("POST /push", …)`
and `mux.Handle("/", mcpHandler)`; a `GET /push` fell through to the MCP catch-all
and returned an opaque protocol error instead of 405, and the handler's own
`r.Method != http.MethodPost` check was dead code. Fix: register `"/push"` (no
method) and 405 inside the handler — now the guard is live and the contract is clean.

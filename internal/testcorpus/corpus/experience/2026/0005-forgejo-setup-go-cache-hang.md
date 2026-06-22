---
schema_version: 1
id: exp-0005
kind: trap
status: validated
title: actions/setup-go cache hangs ~5min then exits on a self-hosted Forgejo/Gitea runner
symptom:
    summary: >
        On a self-hosted Forgejo/Gitea Actions runner, a job using
        actions/setup-go@v5 stalls for ~5 minutes on the cache step and then
        fails (exit 3), with no GitHub Actions cache service available. The same
        workflow passes on github.com.
    error_signatures:
        - "setup-go"
        - "Failed to restore cache"
applies_to:
    - ecosystem: Go
      package: actions/setup-go
resolution:
    root_cause: >
        actions/setup-go@v5 defaults to cache:true, which calls the GitHub
        Actions cache service. A self-hosted Forgejo/Gitea runner has no such
        service, so the cache restore blocks until it times out, then errors.
    fix: >
        Set `cache: false` on the setup-go step for self-hosted runners. Also pin
        the toolchain to a patched patch release (go-version-file: go.mod pins the
        exact patch, so a stale `go x.y.0` makes govulncheck fail on stdlib CVEs).
    dead_ends:
        - tried: "leaving setup-go defaults (cache:true) and waiting it out"
          why_it_failed: "the cache service does not exist on the self-hosted runner; it always times out (~5min) then exits"
guard:
    guarding_test: "CI green on the self-hosted runner with setup-go cache:false (no multi-minute stall on the cache step)."
provenance:
    source:
        author: claude
        session: twiceshy-ci-2026-06-17
    recorded_at: "2026-06-18"
    validated_at: "2026-06-18"
    valid:
        from: "2026-06-18"
---

Banked Forgejo runner gotcha: `actions/setup-go@v5` default `cache:true` hangs
~5min then exits 3 because there is no GitHub cache service on a self-hosted
runner. Fix: `cache: false`. Companion gotcha: `go-version-file: go.mod` pins the
*exact* patch, so a stale `go x.y.0` makes `govulncheck` fail on freshly-disclosed
stdlib CVEs — keep go.mod on a patched patch release.

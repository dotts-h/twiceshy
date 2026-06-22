---
schema_version: 1
id: exp-0017
kind: trap
status: validated
title: "`go test` fails with \"permission denied\" when TMPDIR is on a noexec mount (sandboxes/hardened /tmp)"

symptom:
  summary: >
    Running `go test` (or `go run`) inside a hardened container or sandbox whose
    `/tmp` is mounted `noexec` fails with `fork/exec
    /tmp/go-build.../<pkg>.test: permission denied` — even though the code
    compiles fine. The toolchain builds the test binary into `$TMPDIR` and then
    execs it; a noexec `$TMPDIR` makes that exec fail.
  error_signatures:
    - 'fork/exec /tmp/go-build'
    - 'permission denied'

applies_to:
  - ecosystem: "Go"
    runtime: { go: ">=1.10" }

resolution:
  root_cause: >
    `go test`/`go build` use `$TMPDIR` (default `/tmp`) as the build work
    directory: the compiled test binary is written there and then executed. When
    `/tmp` is mounted `noexec` — common in hardened containers and exactly what
    the twiceshy sandbox broker does for defense in depth — the kernel refuses to
    exec the freshly built binary, so the test phase fails with EACCES while
    compilation itself succeeds.
  fix: >
    Point `TMPDIR` at a writable AND exec-able location before invoking the
    toolchain, e.g. `export TMPDIR=/work/.tmp && mkdir -p "$TMPDIR"` where
    `/work` is a normal (exec-able) mount. Do not relax `noexec` on `/tmp` — that
    weakens the sandbox; move the build's exec into a dir that is meant to hold
    executables.
  dead_ends:
    - tried: "raising the container memory/cpu limits"
      why_it_failed: >
        The failure is not resource exhaustion; it is an exec permission denied
        from the noexec mount. More memory changes nothing.
    - tried: "setting GOCACHE/GOPATH to a writable dir but leaving TMPDIR alone"
      why_it_failed: >
        GOCACHE/GOPATH fix *write* permission for the cache, but the test binary
        is still compiled into and exec'd from `$TMPDIR` (=/tmp), which stays
        noexec — so the exec still fails. TMPDIR is the one that matters.
    - tried: "chmod +x on the built test binary"
      why_it_failed: >
        The binary already has the execute bit; `noexec` is a *mount* attribute
        enforced by the kernel regardless of file permissions, so chmod cannot
        re-enable execution from that filesystem.

guard:
  repros:
    - path: "experience/repro/0017-go-test-noexec-tmpdir.sh"
      kind: positive
      label: "noexec TMPDIR -> EACCES; exec-able TMPDIR -> pass"
  guarding_test: null

provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-18
  validated_at: 2026-06-18
  valid: { from: 2026-06-18, until: null }
  source_license: "none (facts only)"
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

You build the twiceshy execution-validation harness — a gVisor sandbox that runs
untrusted repro scripts — and harden it by mounting the container's `/tmp` as
`noexec` (a sensible default: nothing should be able to drop a binary in `/tmp`
and run it). Your first real Go repro does `go test`, and it fails:

```
fork/exec /tmp/go-build3417/b001/reprotest.test: permission denied
```

The package compiled cleanly. The test never ran. On autopilot it looks like a
broken toolchain or a missing dependency — it is neither.

## Why it happens

`go test` and `go build` use `$TMPDIR` (default `/tmp`) as their build work
directory. The test binary is compiled *into* `$TMPDIR` and then **exec'd** from
there. `noexec` is a mount attribute the kernel enforces irrespective of the
file's execute bit, so the exec is refused with `EACCES` the moment `$TMPDIR`
lives on a noexec filesystem.

## The escape

Give the toolchain a `$TMPDIR` that is both writable and exec-able — a normal
mount, not the hardened `/tmp`:

```sh
export TMPDIR=/work/.tmp && mkdir -p "$TMPDIR"
go test ./...
```

Keep `/tmp` noexec; just don't make the build exec from it. (`GOCACHE`/`GOPATH`
only fix *write* access for the cache — the binary is still exec'd from
`$TMPDIR`, so they don't help.)

## Scope

Any Go toolchain invocation that builds-then-runs a binary (`go test`, `go run`,
`go generate` of a built tool) inside an environment with a noexec `$TMPDIR` —
hardened containers, some CI sandboxes, and the twiceshy broker itself. Found
while building that broker (#0018/#0020): the first Go repro through the harness
failed exactly this way, and pointing `TMPDIR` at the exec-able `/work` volume
fixed it. This record is the harness validating a trap discovered by the harness.

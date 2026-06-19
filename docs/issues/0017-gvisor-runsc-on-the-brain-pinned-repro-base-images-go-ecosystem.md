---
id: 0017
title: gVisor/runsc on the brain + pinned repro-base images (Go ecosystem)
status: closed
severity: high
group: 0015
depends_on: []
forgejo: 107
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Install **current** runsc (gVisor) on the brain — the Debian apt build is stale
(`0.0~20240729`); use the official repo/release. Register it as a Docker runtime
(`runsc`) alongside `runc`. Build **digest-pinned `repro-base` image(s)** for the
Go ecosystem (the first ecosystem, ADR-0011 phase 1). This is the execution
substrate for the harness; **no engine code runs untrusted repros until the 3
must-haves (0018, 0019) are in place.**

## Touches

- brain host: runsc binary + `/etc/docker/daemon.json` runtime registration
- new `repro-base` Dockerfile(s) (Go toolchain), pinned by digest
- KVM platform available (`/dev/kvm`) — prefer it over ptrace for isolation/speed

## Acceptance

- [x] `runsc --version` current; `docker run --runtime=runsc` smoke-test passes
- [x] `repro-base:go-<ver>` image(s) built + digest recorded (pinned, reproducible)
- [x] non-root + read-only-rootfs smoke works with a **named volume** (exp-0004),
      never a `/volume2` bind mount
- [x] documented, reversible teardown (remove runtime + binary)

## Resolution (done 2026-06-18)

Substrate installed + verified on the brain:

- `runsc version release-20260608.0` (spec 1.2.1) — the current official release,
  not the stale apt build. Registered as a Docker runtime (`docker info` →
  `Runtimes: ... runsc`); `runc` stays default.
- Repro base for the Go ecosystem is pinned by digest:
  `golang:1.25-bookworm@sha256:bbb255b0e131db500cf0520adc97441d2260cf629c7fa7e39e025ddf53995a24`.
- KVM platform available (`/dev/kvm`, `crw-rw---- root kvm`).
- Smoke verified end-to-end under `--runtime=runsc`: kernel reports
  `4.19.0-gvisor`, container runs as `nobody` (65534), read-only rootfs rejects
  writes, `--network=none` has no resolver, and a **named volume** (never a
  `/volume2` bind, exp-0004) is the only writable mount.
- **Gotcha recorded for the broker (#0018):** a named volume is root-owned, so a
  non-root exec user cannot write to it until the populate step `chown`s the work
  dir to the exec uid. The broker bakes this in.
- Teardown is reversible: remove the `runsc` runtime stanza from
  `/etc/docker/daemon.json` + delete the binary; pull-cached images are GC'able.

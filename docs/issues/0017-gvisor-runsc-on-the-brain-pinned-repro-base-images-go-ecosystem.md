---
id: 0017
title: gVisor/runsc on the brain + pinned repro-base images (Go ecosystem)
status: open
severity: high
group: 0015
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

- [ ] `runsc --version` current; `docker run --runtime=runsc` smoke-test passes
- [ ] `repro-base:go-<ver>` image(s) built + digest recorded (pinned, reproducible)
- [ ] non-root + read-only-rootfs smoke works with a **named volume** (exp-0004),
      never a `/volume2` bind mount
- [ ] documented, reversible teardown (remove runtime + binary)

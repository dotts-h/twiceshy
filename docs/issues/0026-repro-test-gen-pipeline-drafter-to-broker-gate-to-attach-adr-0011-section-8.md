---
id: 0026
title: Repro test-gen pipeline — drafter to broker-gate to attach (ADR-0011 section 8)
status: closed
severity: high
group: 0015
depends_on: [0020]
forgejo: 116
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

ADR-0011 §8 — the link that turns a quarantined record into one the harness can
PROVE. The retrieval eval (#0005 slice 1) showed retrieval already works on the
validated corpus (recall@3 = 100%); the bottleneck is too few *validated*
records. This pipeline grows that set by **generating repros and gating them by
execution**:

    Drafter ─→ Broker gate (#0018) ─→ attach-or-reject
    (candidate repro)   (prove fail→pass, offline)   (quarantined draft → PR → human validates)

- **Drafter** (a seam, like `Broker`): produces a *candidate* repro from a
  record's structured fact + official docs. Impls: (1) a deterministic template
  drafter for the cleanest classes (Go stdlib deprecations); (2) later, a cheap
  **local model** drafter (Ollama on-demand, VM 101) for harder cases.
  **Local LLM = drafter/flagger, never judge** (standing rule).
- **Gate** = the broker. A draft that does NOT truly fail-pre / pass-post is
  auto-rejected — the execution gate is what makes a cheap drafter safe, and an
  executed original test is structurally ours (the licensing firewall).
- **Attach** = write the proven repro into the record's `guard.repros`, still
  **quarantined**; promotion to `validated` stays the human PR step (#0020).

**Validated design (de-risked 2026-06-19):** a Go deprecation repro runs in the
broker two-phase end-to-end — PREPARE (`--network=bridge`) installs `staticcheck`
into the work volume, EXECUTE (`--network=none`) runs it offline and flags the
deprecated symbol (SA1019). So deprecation records are auto-validatable.

**Scope boundary:** deprecation/OSV-derived facts are NOT §5-gated (facts, not SO
prose) — only the SO-canon (#0024) is. OSV *vulns* are facts, not behaviours, and
are a poor execution-repro fit (they'd mean running exploits); they stay validated
by version-range, not the broker. The pipeline's yield is deprecation/codemod +
behavioural records.

## Slices

- [x] **1 — gate runs dependency-bearing repros.** Teach the revalidator (#0020)
  to run a *directory* repro (`prepare.sh` networked + `repro.sh` offline, all
  files staged), driving the broker's two-phase prepare. Proven on a real
  staticcheck deprecation repro. (Closes the "prepare phase unexercised" gap.)
  **Done 2026-06-19:** `Revalidator.stage` handles a directory repro — stages
  every file, runs `prepare.sh` (networked) then `repro.sh` (offline); a failed
  prepare is reported `broken`, never a false `holds`. The revalidator sets
  `TMPDIR=/work` (exec-able) by default so the Go toolchain's compile-then-exec
  doesn't re-hit the noexec-/tmp trap (exp-0017). Integration-tested end-to-end on
  real runsc: a directory repro installs `staticcheck` in prepare and proves the
  `io/ioutil` SA1019 deprecation (trap flagged, `os` replacement clean) offline.
  Authoring gotcha banked: `staticcheck .` resolves a self-contained module per
  package — a parent module with sub-packages / trailing-slash patterns fails
  offline; the drafter emits one module per trap/fix package.
- [x] **2 — deterministic drafter** for Go stdlib deprecations: from the curated
  fact, emit the candidate module + `prepare.sh`/`repro.sh`, gate it, attach on
  pass. **Done 2026-06-19:** new `internal/drafter` seam — `Drafter` interface +
  `GoDeprecationDrafter`, a template drafter driven by a *catalog* keyed by
  `applies_to` package (the generalization, not per-package special-casing). Each
  entry holds twiceshy's own minimal trap (uses the deprecated symbol) + fix (uses
  the stdlib replacement) Go source — the executed original test is the licensing
  firewall. It emits two self-contained modules (slice-1 gotcha) + `prepare.sh`
  (installs pinned `staticcheck`) + a `repro.sh` keyed on the SA-code, all with
  `TMPDIR=/work` (exp-0017). A `Pipeline` composes draft → broker-gate → attach:
  a holding attestation attaches `Repro{kind:"positive"}` into `guard.repros`
  (record stays **quarantined** — promotion is the human PR step, #0020); a
  rejected draft is detached and its orphan files removed. Catalog covers the
  cleanest class only (stdlib→stdlib: `io/ioutil`, `math/rand`); a drifted
  diagnostic or uncataloged package is refused (`ErrUnsupported` → skipped, left
  for the model drafter). Integration-proven on real runsc: a drafter-*generated*
  `io/ioutil` repro installs staticcheck in prepare and holds offline, attaching
  into the guard. Fixes needing a third-party module (e.g. `strings.Title` →
  `golang.org/x/text`) are deliberately out of scope here.
- [x] **3 — model drafter** (Ollama on-demand) for cases templates can't cover;
  same execution gate; frontier/human judge survivors. **Done 2026-06-20:** new
  `internal/drafter` `ModelDrafter` asks an off-pool local model
  (qwen2.5-coder:14b, VM 101) for only the moving parts (trap/fix/check + optional
  third-party require) and assembles them with the SAME proven script scaffolding
  (`emitGoDeprecationRepro`) — the model never writes the fragile offline scripts.
  Wired as a fallback after the deterministic drafter: the `Pipeline` now tries a
  drafter chain (deterministic first, model for what it can't cover) and stamps the
  winning drafter's name on the attached repro, so a model-origin repro is auditably
  distinct. Env-gated (`TWICESHY_DRAFTER_URL`/`TWICESHY_DRAFTER_MODEL`); absent it,
  drafting stays deterministic-only (CI/bare checkout unchanged). Unit-tested with a
  stubbed Ollama endpoint; **proven live on the brain** — qwen drafted an uncataloged
  `os.SEEK_SET`→`io.SeekStart` repro (`os.Stdin.Seek(0, os.SEEK_SET)` trap) that the
  real-runsc gate proved (fail-pre SA1019 / pass-post clean) and attached. Also added
  in slice 2.5: a deterministic third-party-fix class (strings.Title → x/text, PR #190).

## Notes

Depends on #0018 (broker) + #0020 (revalidator), both shipped. Pairs with #0023
(deprecation importer) — that supplies the records, this makes them validatable.

**First real-corpus run (2026-06-19):** slices 1+2 were run against the live
corpus for the first time. `twiceshy ingest go` seeded three quarantined
stdlib-deprecation records (exp-0043 io/ioutil, exp-0044 strings.Title, exp-0045
math/rand); the new `twiceshy draft` CLI then ran `drafter.Pipeline` over them on
the brain (docker+runsc). It drafted → gated → attached **proven** repros into
exp-0043 (io/ioutil) and exp-0045 (math/rand); strings.Title (exp-0044) was
correctly left unsupported (its fix needs `golang.org/x/text`, out of the
deterministic catalog). The records stay **quarantined** — promotion is the human
PR step (#0020). This is the first drafter-generated corpus growth: the engine has
now produced validatable records, which is the epic's whole point.

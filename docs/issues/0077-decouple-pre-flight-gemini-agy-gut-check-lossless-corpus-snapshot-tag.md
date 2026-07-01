---
id: 0077
title: Decouple pre-flight: gemini+agy gut-check + lossless corpus snapshot tag
status: closed
severity: medium
group: 0076
depends_on: []
forgejo: 450
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Pre-flight gate (ADR-0021 phase 0): run the OWED gemini+agy frontier gut-check on the decouple plan (endpoints were down at ADR authoring), and tag origin/main:experience as a lossless snapshot so the move is provably lossless. Gates the cut-over.

## Outcome (2026-06-22) — DONE

**Gut-check (gemini + agy):** both off-pool endpoints were reachable on the execution
session and ran against the full ADR-0021 brief. **Both confirm decision B** — each probed
for a simpler option (agy: orphan `corpus` branch; gemini: validate the trust boundary
in-repo first), and neither dislodged the split because the **multi-tenant requirement
(#0010) is decisive** (N tenants = N corpus stores; an in-repo branch can't give that). The
"simpler" paths both reduce to **option D**, already adopted as the interim step.

**Four execution guards** were surfaced and folded into ADR-0021's migration plan (see its new
"Pre-flight gut-check (2026-06-22)" section + the amended phases):
1. Quiesce needs a **hard write-lock** on `experience/` (not just stop-timers+drain); the
   authoritative snapshot is taken last, under that lock — Phase 3.
2. The schema contract must **fail loud** and be checked against the *deployed* engine
   version — Decision §2 + #0079.
3. **CI dependency inversion**: corpus CI runs the engine's *pinned CLI artifact*, not a
   build-from-source — Phase 2 + #0080.
4. **id-allocator** initialises at `max(exp-NNNN) + gap`, asserted pre-restart — Phase 6 + #0082/#0083.

**Snapshot tag:** `corpus-snapshot-pre-decouple-20260622` (annotated) → commit `7f4fe26`,
`experience/` tree `cac6dfc`, 2694 records. Pushed to origin. This is a **baseline** recovery
point; recover losslessly with `git archive corpus-snapshot-pre-decouple-20260622 experience/`.
Per guard 1, the *authoritative* cutover snapshot is re-taken at quiesce.

ADR-0021 status flipped **Proposed → Accepted** (the owed gut-check is the gate it named).

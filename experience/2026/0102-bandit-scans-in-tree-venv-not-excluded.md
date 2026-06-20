---
schema_version: 1
id: exp-0102
kind: trap
status: quarantined
title: bandit -r . scans a runtime-created in-tree venv unless excluded with a path-matching form
symptom:
    summary: A bandit SAST CI gate fails on third-party dependency code (pytest exec, aiohttp SHA1, marshal) because `-x .venv,tests` (bare names) does not exclude a venv created inside the scan root.
    error_signatures:
        - '[B324:hashlib] Use of weak SHA1 hash for security. Consider usedforsecurity=False  Location: ./.venv/lib/python3.13/site-packages/aiohttp/client.py:1289'
        - '[B102:exec_used] Use of exec detected.  Location: ./.venv/lib/python3.13/site-packages/_pytest/_py/path.py:1153'
        - '[B302:blacklist] Deserialization with the marshal module is possibly dangerous.  Location: ./.venv/lib/python3.13/site-packages/_pytest/assertion/rewrite.py:393'
applies_to:
    - ecosystem: python
      package: bandit
resolution:
    root_cause: bandit's --exclude/-x matches discovered file paths by fnmatch; a bare directory name like `.venv` does NOT match the nested `./.venv/lib/.../*.py` files it walks. When a CI security gate creates the scanner/deps venv INSIDE the scanned dir (`cd pkg && python3 -m venv .venv && .venv/bin/bandit -r . -x .venv,tests`), bandit scans all of site-packages and fails on dependency code (weak hashes in aiohttp, exec/marshal in pytest) — none of which is the project's source. The gate looks like it found real issues but is misconfigured.
    fix: 'Use a path-matching exclude form: `bandit -r . -ll -x ''./.venv,./tests''` (the leading `./` makes fnmatch match the nested tree). Cleaner still: create the scanner venv OUTSIDE the scanned tree (e.g. `python3 -m venv /tmp/sectools`) so `-r .` never sees it — bandit is static analysis and never needs the project''s deps installed (only the scanner binaries). Always confirm the exclude actually scoped to source by checking that no finding''s `Location:` is under `.venv/`.'
guard:
    repro: null
    guarding_test: 'In a working tree that contains a populated `.venv`, run the gate and assert bandit''s output contains zero `Location: ./.venv/` lines (it scanned only first-party source). Equivalently: `bandit -r . -x ''./.venv,./tests'' | grep -c ''/.venv/''` must be 0.'
provenance:
    source:
        author: claude
        session: memory-vault + recipe build, 2026-06-20
        pr: null
    recorded_at: "2026-06-20"
    validated_at: null
    valid:
        from: "2026-06-20"
        until: null
    superseded_by: null
---

## What happened
A new `security` CI gate (cookbook `security` recipe) on a Python service went red on the `bandit` SAST job. The findings were all real bandit rules (B324 weak SHA1, B102 exec, B302 marshal, B307 eval) — but **every `Location:` was under `./.venv/lib/python3.13/site-packages/`** (aiohttp, pytest, anyio), i.e. installed dependencies, not the project's own code.

The gate's invocation was `cd gateway && python3 -m venv .venv && .venv/bin/bandit -r . -ll -x .venv,tests`. Because the venv is created *inside* the scanned root and `-x .venv,tests` (bare names) doesn't match the nested `./.venv/...` paths, bandit walked the entire dependency tree and failed on third-party code.

## How to tell it's this trap (not a real finding)
Look at the `Location:` of each finding. If they're under `.venv/`/`site-packages/`/`node_modules/`, the exclude is broken — it's scanning vendored code, not yours. `pip-audit` being clean while `bandit` is red on "scary" rules in well-known libs is a strong tell.

## Fix
- Minimal: `-x './.venv,./tests'` (leading `./` so fnmatch matches the nested tree). Verified to scope back to first-party source.
- Robust: put the scanner venv outside the scanned tree (`/tmp`), since bandit needs no project deps installed.

After fixing the scope, triage the *real* first-party findings normally; suppress true false positives narrowly with `# nosec <ID>  # why` (never repo-wide).

## Related
Same class as other "bare exclude doesn't match nested paths" gotchas. Pairs with the recipe rule: a SAST gate must scan your source, not your dependencies.

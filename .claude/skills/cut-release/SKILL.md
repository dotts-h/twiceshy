---
name: cut-release
description: Cut a tagged forgejo release (twiceshy artifacts + checksums + notes) via the release workflow, verifying the resolved version end-to-end so a misconfigured workflow can't publish a mis-tagged release. Use when asked to cut, tag, publish, or ship a release (e.g. "release v0.1.0", "tag and publish"), especially in a sandbox where direct `git push` of a tag is blocked.
allowed-tools: Read, Bash, Grep
---

# Cutting a release (with version verification)

A release is **outward-facing and hard to reverse** — a published release is indexed and downloaded
the moment it exists. The failure mode this skill prevents: the release workflow resolves the *wrong*
version and publishes a mis-tagged release (e.g. tagged after the branch, `main`, instead of
`v0.1.0`). Verify before and after; never fire blind. The full rules + the verify-after checklist
live in `docs/RELEASING.md`; the workflow itself is the recipe's installed
`.forgejo/workflows/release.yml`.

## Pre-flight (before publishing)

1. **The release must be authorized** — a release is outward-facing, so proceed only on an explicit
   request for *this* version. Confirm the exact tag (e.g. `v0.1.0`) and the commit (normally the
   current default-branch tip).
2. **Audit the version resolution.** Read the release workflow's version step. It MUST prefer the
   dispatch input and fall back to the ref name:
   ```yaml
   run: echo "version=${{ github.event.inputs.tag || github.ref_name }}" >> "$GITHUB_OUTPUT"
   ```
   The broken form `${GITHUB_REF_NAME:-inputs.tag}` resolves to the **branch** ("main") on a manual
   dispatch, because the ref-name env var is non-empty — the `:-` fallback never fires. If you see it,
   fix the workflow first (its own PR), then release. (`recipes/release/scripts/doctor.sh` flags both
   this and a release build step still left as a TODO placeholder.)
3. **The build step is real, not a stub.** The workflow's build step must actually produce artifacts
   in `dist/` (`CGO_ENABLED=0 go build -o dist/twiceshy ./cmd/twiceshy`). If it still writes a `BUILD_TODO.txt` placeholder, the
   release would ship nothing — backfill the real build command first.
4. Gates are green on the commit you're tagging (CI passed on the default branch).

## Publishing

- **Preferred (tag push):** `git push forgejo <tag>` (the canonical remote — `origin` is the
  read-only GitHub mirror). The `push: tags: ["v*"]` trigger runs the release workflow; the ref
  name is the tag, so the version is correct.
- **Sandbox fallback (tag push blocked / HTTP 403):** trigger the workflow's `workflow_dispatch` with
  the tag as input (the forge's "run workflow" API/MCP, on the default branch,
  `inputs: {tag: "<tag>"}`). The workflow's own `contents: write` token creates the tag + release
  server-side — no local tag push needed. **This path REQUIRES the step-2 fix**, or it tags the
  release after the branch.

## Verify (after publishing) — non-negotiable

Confirm the published release matches the request before reporting success:
- The workflow run completed `success`.
- The release for the exact tag exists with `tag_name == "<tag>"` (NOT the branch name), and the
  asset names embed the version (`twiceshy-<tag>-<os>-<arch>`) alongside `checksums.txt`.
- If the tag is wrong (e.g. `main`): the workflow version resolution was buggy. Fix it, re-cut, and
  **surface the mis-tagged release for deletion** — a stray release often can't be removed via the
  read-only tools, so it needs the user's hand.

## Lesson (why this skill exists)

Triggering a release without auditing the version step published a release tagged `main`; the stray
`main` tag then collided with the `main` branch and corrupted `git fetch origin main`. Outward-facing
actions get verified, not assumed.

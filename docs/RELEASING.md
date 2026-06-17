# RELEASING.md — twiceshy release playbook

> A release is **outward-facing and hard to reverse** — a published release is
> indexed and downloaded the moment it exists. Verify before and after; never
> fire blind. The release is **tag-driven**: pushing a `v*` tag (or a manual
> `workflow_dispatch` with the tag as input) runs `.github/workflows/release.yml`.

## SemVer rules

Versions are `vMAJOR.MINOR.PATCH`, bumped per landed change:

- **Feature → minor.** A shipped roadmap item / epic child is a feature.
- **Bug fix → patch.**
- **Breaking change → major.** Pre-1.0, a breaking change may ride a minor
  (`0.x` is unstable by SemVer); call it out in the notes either way.
- An epic that lands several feature children closes on a **single minor** bump,
  not one bump per child.

## Pre-flight (before publishing)

- [ ] **Authorized**: an explicit request for *this* version exists. Confirm the
      exact tag and the commit (normally the current default-branch tip).
- [ ] **Audit the version resolution.** The workflow's version step MUST prefer
      the dispatch input and fall back to the ref name
      (`github.event.inputs.tag || github.ref_name`). The shell-default form
      (`GITHUB_REF_NAME` with a `:-` fallback) resolves to the **branch** on a
      dispatch and publishes a mis-tagged release — `scripts/check-workflows.sh`
      fails on it; never hand-edit that step around the guard.
- [ ] Gates are green on the commit being tagged.

## Publishing

- **Preferred:** `git push origin <tag>` — the `push: tags` trigger runs the
  workflow; the ref name *is* the tag, so the version is correct.
- **Fallback (tag push blocked):** trigger `workflow_dispatch` with the tag as
  input; the workflow's own `contents: write` token creates the tag and release
  server-side. This path *requires* the audited version step above.

## Verify (after publishing) — non-negotiable

- [ ] The workflow run completed successfully.
- [ ] Fetching the release **by the exact tag** returns it with `tag_name`
      equal to the tag (NOT a branch name).
- [ ] Asset names embed the version (`twiceshy-<tag>-…`) and
      `checksums.txt` is attached.
- [ ] If the tag is wrong: the version resolution regressed. Fix the workflow,
      re-cut, and surface the mis-tagged release for deletion by a human.

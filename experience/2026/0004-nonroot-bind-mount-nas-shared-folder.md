---
schema_version: 1
id: exp-0004
kind: trap
status: validated
title: Non-root container can't read a UGOS/Synology shared-folder bind mount — use a Docker named volume
symptom:
    summary: >
        A container running as a non-root UID (e.g. distroless nonroot uid
        65532) gets "permission denied" reading a host bind mount under a
        UGOS/Synology shared folder (e.g. /volume2/Docker/...), even when the
        files are chowned to that UID and chmod 0777 — because the shared
        folder's ACL semantics deny all non-root container access. Root
        containers work (they bypass the check), which masks the problem.
    error_signatures:
        - "permission denied"
        - "lstat /data/corpus/experience: permission denied"
applies_to:
    - ecosystem: Docker
      package: distroless
resolution:
    root_cause: >
        UGOS Pro / Synology "shared folder" volumes (/volume1, /volume2, ...)
        carry extended ACLs that restrict access for non-root UIDs regardless of
        the POSIX owner/mode shown by ls; a bind mount into such a path inherits
        that restriction, so a non-root container UID cannot traverse/read it
        even at 0777, while a root container bypasses permission checks entirely.
    fix: >
        Don't bind-mount a path under the NAS shared folder for a non-root
        container. Use a Docker named volume (docker volume create), populate it
        and chown to the container UID via a throwaway root helper container
        (e.g. alpine chown -R 65532:65532 /data), then mount the named volume.
        Reproduce the failure as the target UID with `docker run --user 65532 ...`
        (root masks it).
    dead_ends:
        - tried: "chown -R 65532:65532 + chmod 0777 on the host bind path"
          why_it_failed: "the shared-folder ACL still denies non-root; getfacl shows clean 0777 but the parent dir is drwxrwxrwx+ (ACL present)"
        - tried: "verifying access with a root alpine container"
          why_it_failed: "root bypasses permission checks, so the smoke test passes while the real non-root service still fails"
guard:
    guarding_test: "Deploy smoke: container as uid 65532 reads /data and serves; `docker run --rm --user 65532 -v <vol>:/data alpine cat /data/<file>` succeeds."
provenance:
    source:
        author: claude
        session: twiceshy-deploy-2026-06-18
    recorded_at: "2026-06-18"
    validated_at: "2026-06-18"
    valid:
        from: "2026-06-18"
---

Symptom on start: `loading corpus: lstat /data/corpus/experience: permission
denied`, repeating, even after `chown -R 65532:65532` and `chmod 0777`. A
throwaway `alpine` (root) container reads the same path fine — that mismatch is
the tell: the failure is non-root-specific, not ownership. `getfacl` on the host
shows clean 0777, but the parent shared folder (`drwxrwxrwx+`, note the `+`)
carries an ACL that blocks non-root. The fix that worked: a Docker named volume
(`twiceshy-data`), populated + chowned via a root helper container, mounted
instead of the bind. A `/opt/...` host-bind recipe does NOT work on this NAS.
Diagnose by reproducing as the exact UID (`docker run --user 65532`), since the
default root container hides it.

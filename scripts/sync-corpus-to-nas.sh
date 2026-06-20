#!/usr/bin/env bash
# sync-corpus-to-nas.sh — mirror origin/main's experience/ corpus to the twiceshy
# NAS Docker volume and restart the container ONLY when the corpus changed.
#
# Why this exists: the server rebuilds its index from /data/corpus on start, but
# nothing kept that volume in sync with the merged repo. The live corpus drifted
# (orphan records with colliding ids even crash-looped serve on a restart). This
# is the missing sync — idempotent, change-gated, fail-safe.
#
# Change detection: the git tree SHA of experience/ on origin/main (a content
# hash) is compared to a marker written on the volume; identical => no-op, no
# restart. The mirror replaces experience/ wholesale (the repo is the source of
# truth, ADR-0001 §1), so orphan/colliding records can never accumulate again.
#
# Reads origin/main via `git archive` — never touches a working tree, so it is
# safe to run from a repo checked out on any branch.
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-import}"
NAS="${TWICESHY_NAS:-Claude@192.168.50.244}"
NAS_PORT="${TWICESHY_NAS_PORT:-2222}"
VOL="${TWICESHY_VOLUME:-twiceshy-data}"
CONTAINER="${TWICESHY_CONTAINER:-twiceshy}"
NONROOT_UID="${TWICESHY_UID:-65532}" # distroless nonroot (DEPLOY.md)

ssh_nas() { ssh -p "$NAS_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$NAS" "$@"; }

git -C "$REPO" fetch -q origin main
new_sha="$(git -C "$REPO" rev-parse origin/main:experience)"
cur_sha="$(ssh_nas "docker run --rm -v $VOL:/data alpine cat /data/corpus/.experience-tree-sha 2>/dev/null" 2>/dev/null || true)"

if [ "$new_sha" = "$cur_sha" ]; then
  echo "corpus up to date ($new_sha)"
  exit 0
fi

echo "syncing corpus ${cur_sha:-<none>} -> $new_sha"
# One helper container does the whole mirror atomically: replace experience/,
# stamp the marker, fix ownership for the nonroot server uid.
git -C "$REPO" archive --format=tar origin/main experience | ssh_nas \
  "docker run --rm -i -v $VOL:/data alpine sh -c '
     rm -rf /data/corpus/experience &&
     mkdir -p /data/corpus &&
     tar xf - -C /data/corpus &&
     printf %s \"$new_sha\" > /data/corpus/.experience-tree-sha &&
     chown -R $NONROOT_UID:$NONROOT_UID /data/corpus'"

# serve rebuilds the index only on start, so a restart is required to pick up the
# new corpus. Gated on an actual change, so it is rare.
ssh_nas "docker restart $CONTAINER >/dev/null"
echo "synced + restarted $CONTAINER at corpus $new_sha"

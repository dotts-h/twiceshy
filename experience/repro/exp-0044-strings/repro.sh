#!/bin/sh
set -u
export GOTOOLCHAIN=local GOCACHE=/work/.gocache GOPATH=/work/.gopath GOBIN=/work/bin TMPDIR=/work
command -v go >/dev/null 2>&1 || { echo SKIP; exit 75; }
[ -x /work/bin/staticcheck ] || { echo 'SKIP: staticcheck not warmed'; exit 75; }
if ! (cd /work/trap && /work/bin/staticcheck .) 2>&1 | grep -q SA1019; then echo 'NOT REPRODUCED: trap not flagged'; exit 1; fi
fixout=$(cd /work/fix && /work/bin/staticcheck . 2>&1); fixrc=$?
if [ "$fixrc" -ne 0 ]; then echo "FIX BROKEN: staticcheck did not pass cleanly (rc=$fixrc): $fixout"; exit 1; fi
echo OK; exit 0

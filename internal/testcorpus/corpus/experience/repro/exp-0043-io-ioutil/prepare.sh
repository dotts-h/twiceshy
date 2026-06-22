#!/bin/sh
set -e
export GOTOOLCHAIN=local GOCACHE=/work/.gocache GOPATH=/work/.gopath GOBIN=/work/bin TMPDIR=/work
go install honnef.co/go/tools/cmd/staticcheck@2025.1

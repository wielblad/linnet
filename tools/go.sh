#!/bin/sh
# Runs the project's own Go (kept in .local, outside the repository) with its own module
# folder, without touching the network. Usage: sh tools/go.sh test ./...
HERE="$(cd "$(dirname "$0")/.." && pwd)"
WIN="$(cygpath -w "$HERE")"
cd "$HERE" || exit 1
GOPATH="$WIN\\.local\\gopath" GOCACHE="$WIN\\.local\\gocache" GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
CGO_ENABLED=0 GOTELEMETRY=off GOPROXY=off exec "$HERE/.local/toolchain/go/bin/go.exe" "$@"

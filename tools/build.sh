#!/bin/sh
# Builds build/linnet.exe and the installer. Usage: sh tools/build.sh
HERE="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$HERE/build"
sh "$HERE/tools/go.sh" build -trimpath -ldflags="-H windowsgui -s -w" -o build/linnet.exe ./cmd/mlite || exit 1
# The installer is the same program under a name that contains "setup".
cp "$HERE/build/linnet.exe" "$HERE/build/Linnet-Setup.exe"
echo "zbudowano: build/linnet.exe, build/Linnet-Setup.exe"

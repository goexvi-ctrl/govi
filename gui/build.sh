#!/bin/sh
# Build GoVi.app via gui/govi.mk (dependency-aware incremental build).
#
# Requirements: GNU make, a Go toolchain, and the Xcode command-line tools
# (swiftc, clang). Run from anywhere; paths are resolved relative to this script.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)

make -f "$here/govi.mk" -C "$root" govi-app

app="$here/build/GoVi.app"
echo "==> done: $app"
echo "Open files from the command line with the launcher:"
echo "    $here/govi <file> ...        (reuses a running GoVi.app)"
echo "Put it on your PATH, e.g.:  ln -s $here/govi /usr/local/bin/govi"
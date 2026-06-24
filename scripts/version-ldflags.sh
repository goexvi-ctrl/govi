#!/bin/sh
# Emit -ldflags for embedding git metadata in engine/version.go variables.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

date=$(git log -1 --format=%cs 2>/dev/null || echo unknown)
hash=$(git rev-parse --short HEAD 2>/dev/null || echo "")
state=""
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
	state=modified
fi

printf '%s' "-X govi/engine.versionName=govi-0.1 -X govi/engine.commitDate=${date} -X govi/engine.commitHash=${hash} -X govi/engine.treeState=${state}"
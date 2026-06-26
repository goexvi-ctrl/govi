#!/bin/sh
# Emit -ldflags for embedding git metadata in engine/version.go variables.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

commitDate=$(git log -1 --format=%cs 2>/dev/null || echo unknown)
hash=$(git rev-parse --short HEAD 2>/dev/null || echo "")
state=""
buildTime=""
# Tracked edits only; untracked files do not mark the build modified.
if ! git diff-index --quiet HEAD -- 2>/dev/null; then
	state=modified
	buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

printf '%s' "-X govi/engine.versionName=govi-0.1 -X govi/engine.commitDate=${commitDate} -X govi/engine.commitHash=${hash} -X govi/engine.treeState=${state} -X govi/engine.buildTime=${buildTime}"
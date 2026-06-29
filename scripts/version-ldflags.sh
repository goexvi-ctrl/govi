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
# diff-index exits 1 when dirty, 0 when clean, 128+ on error (treat as clean).
rc=0
if git rev-parse -q --verify HEAD >/dev/null 2>&1; then
	git diff-index --quiet HEAD -- 2>/dev/null || rc=$?
fi
if [ "$rc" -eq 1 ]; then
	state=modified
	buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

printf '%s' "-X govi/engine.versionName=govi-0.1.1 -X govi/engine.commitDate=${commitDate} -X govi/engine.commitHash=${hash} -X govi/engine.treeState=${state} -X govi/engine.buildTime=${buildTime}"
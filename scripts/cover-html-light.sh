#!/bin/sh
# Rewrite go tool cover HTML from its default dark theme to a light,
# contrast-friendly stylesheet (scripts/cover-light.css).
set -eu

html="${1:?usage: cover-html-light.sh coverage.html}"
dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
css="$dir/cover-light.css"

exec python3 - "$html" "$css" <<'PY'
import pathlib
import re
import sys

html_path = pathlib.Path(sys.argv[1])
css_path = pathlib.Path(sys.argv[2])
html = html_path.read_text()
css = css_path.read_text()
patched, n = re.subn(
    r"<style>.*?</style>",
    "<style>\n" + css + "\n\t\t</style>",
    html,
    count=1,
    flags=re.DOTALL,
)
if n != 1:
    sys.exit(f"cover-html-light: expected one <style> block in {html_path}")
html_path.write_text(patched)
PY
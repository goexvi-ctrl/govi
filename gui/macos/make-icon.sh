#!/bin/sh
# Build AppIcon.icns from a square source image (e.g. icon.jpg) for Govi.app.
set -eu

src=${1:?usage: make-icon.sh source-image output.icns}
out=${2:?usage: make-icon.sh source-image output.icns}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

iconset="$work/AppIcon.iconset"
mkdir -p "$iconset"

png() {
	size=$1
	name=$2
	sips -z "$size" "$size" -s format png "$src" --out "$iconset/$name" >/dev/null
}

png 16   icon_16x16.png
png 32   icon_16x16@2x.png
png 32   icon_32x32.png
png 64   icon_32x32@2x.png
png 128  icon_128x128.png
png 256  icon_128x128@2x.png
png 256  icon_256x256.png
png 512  icon_256x256@2x.png
png 512  icon_512x512.png
png 1024 icon_512x512@2x.png

mkdir -p "$(dirname "$out")"
iconutil -c icns "$iconset" -o "$out"
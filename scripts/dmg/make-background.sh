#!/bin/sh
# Regenerate the DMG background images from the arrows artwork.
#
#   scripts/dmg/make-background.sh
#
# Composites scripts/dmg/arrows.png (two dashed arrows on white) onto a sized
# canvas with a title, producing dmg-background.png (1x, 640x500) and
# dmg-background@2x.png (1280x1000). Requires ImageMagick (`magick`). These
# outputs are committed so a release build needs only tiffutil (always present on
# macOS), not ImageMagick.
#
# The arrow endpoints in arrows.png (1024x1024) are tail (307,190)/head (621,175)
# for the top arrow and tail (307,566)/head (621,551) for the bottom one. They
# are scaled by SCALE and offset by (OX,OY) into the canvas; the icon positions
# in settings.py must stay aligned with the result (see make-background.sh's
# computed endpoints, mirrored there).
set -eu

cd "$(dirname "$0")"

BOLD="/System/Library/Fonts/Supplemental/Arial Bold.ttf"
REG="/System/Library/Fonts/Supplemental/Arial.ttf"
SRC="arrows.png"

# Layout constants (window points); keep in sync with settings.py icon_locations.
W=640
H=500
SCALE=0.476   # arrows.png (1024) -> window scale
OX=99         # arrows x offset on the canvas
OY=60         # arrows y offset on the canvas

render() {
	# $1 = scale multiplier (1 or 2), $2 = output file
	s=$1
	out=$2
	cw=$(echo "$W * $s" | bc)
	ch=$(echo "$H * $s" | bc)
	apx=$(echo "1024 * $SCALE * $s / 1" | bc)
	ox=$(echo "$OX * $s / 1" | bc)
	oy=$(echo "$OY * $s / 1" | bc)
	title=$(echo "26 * $s / 1" | bc)
	sub=$(echo "13 * $s / 1" | bc)
	ty=$(echo "20 * $s / 1" | bc)
	sy=$(echo "52 * $s / 1" | bc)
	magick -size "${cw}x${ch}" xc:white \
		\( "$SRC" -resize "${apx}x${apx}" \) -geometry "+${ox}+${oy}" -composite \
		-gravity north \
		-font "$BOLD" -pointsize "$title" -fill "#222222" -annotate "+0+${ty}" "Install GoVi" \
		-font "$REG" -pointsize "$sub" -fill "#666666" -annotate "+0+${sy}" \
		"Drag each item onto the folder to its right" \
		"$out"
	echo "wrote $out (${cw}x${ch})"
}

render 1 dmg-background.png
render 2 "dmg-background@2x.png"

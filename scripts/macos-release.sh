#!/bin/sh
# Build a signed macOS .dmg for release, and -- given a Developer ID identity --
# notarize and staple it so it passes Gatekeeper with no user fiddling.
#
# Invoked by `make release` with these variables in the environment:
#   APP            path to the built .app bundle       (e.g. gui/build/GoVi.app)
#   CLI            path to the built CLI binary         (e.g. govi)
#   DMG            output disk image path
#   VOLNAME        mounted volume name                  (e.g. "GoVi 0.1.2")
#   IDENTITY       codesign identity, or "-" for ad-hoc
#   NOTARY_PROFILE stored notarytool keychain profile   (Developer ID only)
#
# IDENTITY="-" : ad-hoc sign only; the image is NOT notarized (dev/local use),
#   so a downloaded copy stays quarantined and must be cleared by hand.
# IDENTITY="Developer ID Application: ..." : hardened-runtime sign the app and
#   CLI, sign the image, submit it to Apple's notary service, and staple the
#   ticket -- a download then opens with no quarantine workaround.
set -eu

: "${APP:?set APP}" "${CLI:?set CLI}" "${DMG:?set DMG}"
: "${VOLNAME:?set VOLNAME}" "${IDENTITY:?set IDENTITY}"
NOTARY_PROFILE="${NOTARY_PROFILE:-govi-notary}"
BUILD_DIR="$(dirname "$DMG")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DMG_ASSETS="$SCRIPT_DIR/dmg"

# Hardened runtime and a secure timestamp need a real certificate; an ad-hoc
# signature supports neither, so only request them for a Developer ID build.
runtime=""
timestamp=""
if [ "$IDENTITY" != "-" ]; then
	runtime="--options runtime"
	timestamp="--timestamp"
else
	echo "macos-release: ad-hoc signing (no Developer ID) -- skipping notarization"
fi

# 1. Sign the CLI and the app bundle. --force replaces the linker/ad-hoc
#    signature that the Go linker and swiftc leave behind.
codesign --force $runtime $timestamp --sign "$IDENTITY" "$CLI"
codesign --force $runtime $timestamp --sign "$IDENTITY" "$APP"
codesign --verify --strict "$APP"

# 2. Build a styled disk image with dmgbuild: the app and CLI, an /Applications
#    and a /usr/local/bin symlink to drag each onto, and a background showing an
#    arrow from each item to its destination (no README needed). dmgbuild writes
#    the .DS_Store directly, so no Finder/GUI session is required.
#
#    dmgbuild is a Python tool; use it from PATH if present, otherwise provision
#    it into a local venv (python3 ships with the Xcode command line tools that a
#    Swift/codesign build already needs).
DMGBUILD="$(command -v dmgbuild || true)"
if [ -z "$DMGBUILD" ]; then
	VENV="$BUILD_DIR/dmgbuild-venv"
	if [ ! -x "$VENV/bin/dmgbuild" ]; then
		echo "macos-release: provisioning dmgbuild into $VENV"
		python3 -m venv "$VENV"
		"$VENV/bin/pip" install --quiet --upgrade pip
		"$VENV/bin/pip" install --quiet dmgbuild
	fi
	DMGBUILD="$VENV/bin/dmgbuild"
fi

# Combine the 1x and 2x backgrounds into a single Retina-aware TIFF.
BG_TIFF="$BUILD_DIR/dmg-background.tiff"
tiffutil -cathidpicheck \
	"$DMG_ASSETS/dmg-background.png" "$DMG_ASSETS/dmg-background@2x.png" \
	-out "$BG_TIFF" >/dev/null

rm -f "$DMG"
APP="$APP" CLI="$CLI" BG="$BG_TIFF" \
	"$DMGBUILD" -s "$DMG_ASSETS/settings.py" "$VOLNAME" "$DMG"
rm -f "$BG_TIFF"

# 3. Sign the disk image itself.
codesign --force $timestamp --sign "$IDENTITY" "$DMG"

# 4. With a Developer ID, notarize the image and staple the ticket.
if [ "$IDENTITY" != "-" ]; then
	echo "macos-release: submitting to the notary service (this can take a few minutes)..."
	xcrun notarytool submit "$DMG" --keychain-profile "$NOTARY_PROFILE" --wait
	xcrun stapler staple "$DMG"
	xcrun stapler validate "$DMG"
fi

echo "Wrote $DMG"

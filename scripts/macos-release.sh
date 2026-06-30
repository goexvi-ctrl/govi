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
STAGE="$(dirname "$DMG")/dmg-stage"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

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

# 2. Stage the image contents and build a compressed disk image: the app, the
#    CLI, an /Applications symlink and a /usr/local/bin symlink to drag each onto,
#    and a README. ditto (not cp) preserves the bundle's symlinks and metadata.
rm -rf "$STAGE" "$DMG"
mkdir -p "$STAGE"
ditto "$APP" "$STAGE/$(basename "$APP")"
cp "$CLI" "$STAGE/$(basename "$CLI")"
# Drag targets: GoVi.app -> Applications, and the govi CLI -> /usr/local/bin
# (already on the default PATH). A short README explains both.
ln -s /Applications "$STAGE/Applications"
ln -s /usr/local/bin "$STAGE/usr-local-bin"
cp "$SCRIPT_DIR/dmg-README.txt" "$STAGE/README.txt"
hdiutil create -quiet -volname "$VOLNAME" -srcfolder "$STAGE" -ov -format UDZO "$DMG"
rm -rf "$STAGE"

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

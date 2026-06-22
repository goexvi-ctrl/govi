#!/bin/sh
# Build Govi.app: the macOS application with the govi engine embedded in-process.
#
# Steps:
#   1. Compile the engine into a C archive (libgovi.a + libgovi.h) with cgo.
#   2. Compile the Swift/AppKit front end and link it against the archive.
#   3. Assemble a Govi.app bundle.
#
# Requirements: a Go toolchain and the Xcode command-line tools (swiftc, clang).
# Run from anywhere; paths are resolved relative to this script.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
build="$here/build"
mkdir -p "$build"

echo "==> building libgovi.a (cgo c-archive)"
( cd "$root" && go build -buildmode=c-archive -o "$build/libgovi.a" ./gui/bridge )

echo "==> compiling and linking the Swift app"
swiftc \
	-O \
	-import-objc-header "$here/macos/Bridging.h" \
	-I "$build" \
	"$here"/macos/*.swift \
	"$build/libgovi.a" \
	-framework Cocoa \
	-framework CoreFoundation \
	-framework Security \
	-o "$build/Govi"

echo "==> assembling Govi.app"
app="$build/Govi.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS"
cp "$build/Govi" "$app/Contents/MacOS/Govi"
cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>            <string>Govi</string>
	<key>CFBundleDisplayName</key>     <string>Govi</string>
	<key>CFBundleIdentifier</key>      <string>org.govi.editor</string>
	<key>CFBundleExecutable</key>      <string>Govi</string>
	<key>CFBundlePackageType</key>     <string>APPL</string>
	<key>CFBundleVersion</key>         <string>0.1</string>
	<key>CFBundleShortVersionString</key><string>0.1</string>
	<key>NSHighResolutionCapable</key> <true/>
	<key>NSPrincipalClass</key>        <string>NSApplication</string>
</dict>
</plist>
PLIST

echo "==> done: $app"
echo "Run a file with:  $app/Contents/MacOS/Govi <path>"
echo "Or open the app:  open $app --args <path>"

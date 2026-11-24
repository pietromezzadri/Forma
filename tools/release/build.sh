#!/usr/bin/env bash
# Builds a Forma release for Linux or macOS (whichever this is), as
# tools/release/build.ps1 does for Windows:
#
#     tools/release/build.sh 0.2.0
#
# Linux: dist/forma-<version>-linux-x64.tar.gz — the forma binary (it needs
# the system's OpenGL and X11 libraries, and zenity or kdialog for the file
# dialogs) with its fonts, the licence and the README.
#
# macOS: dist/forma-<version>-macos-universal.zip — Forma.app, one binary
# for Apple silicon and Intel (built for each and joined by lipo), with its
# fonts and icon, signed ad hoc (not notarised: the first time, right-click
# it and choose Open), the licence and the README beside it.
#
# Each writes dist/SHA256SUMS-<os>.txt. The release workflow
# (.github/workflows/release.yml) runs it on every v* tag.
set -euo pipefail

version=${1:?usage: build.sh <version>}
root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
mkdir -p dist
ld="-s -w -X forma/app.Version=$version"
export CGO_ENABLED=1

case "$(uname -s)" in
Linux)
	name="forma-$version-linux-x64"
	stage="dist/$name"
	rm -rf "$stage" "$stage.tar.gz"
	mkdir -p "$stage/assets"
	# X11 only: it runs on Wayland desktops too, through XWayland, and needs
	# no Wayland libraries
	go build -trimpath -tags x11 -ldflags "$ld" -o "$stage/forma" .
	cp -R assets/fonts "$stage/assets/"
	cp LICENSE README.md "$stage/"
	echo "libraries: $(ldd "$stage/forma" | awk '{print $1}' | tr '\n' ' ')"
	tar -C dist -czf "$stage.tar.gz" "$name"
	rm -rf "$stage"
	out="$name.tar.gz"
	(cd dist && sha256sum "$out" > SHA256SUMS-linux.txt)
	;;
Darwin)
	name="forma-$version-macos-universal"
	stage="dist/$name"
	app="$stage/Forma.app"
	rm -rf "$stage" "$stage.zip"
	mkdir -p "$app/Contents/MacOS/assets" "$app/Contents/Resources"
	for arch in arm64 amd64; do
		carch=$arch
		[ "$arch" = amd64 ] && carch=x86_64
		GOARCH=$arch CGO_CFLAGS="-arch $carch" CGO_LDFLAGS="-arch $carch" \
			go build -trimpath -ldflags "$ld" -o "dist/forma-$arch" .
	done
	lipo -create -output "$app/Contents/MacOS/forma" dist/forma-arm64 dist/forma-amd64
	rm dist/forma-arm64 dist/forma-amd64
	lipo -info "$app/Contents/MacOS/forma"
	# the fonts are looked for beside the program
	cp -R assets/fonts "$app/Contents/MacOS/assets/"
	# the icon, from the app's own PNGs
	iconset="dist/forma.iconset"
	rm -rf "$iconset"
	mkdir -p "$iconset"
	for s in 16 32 128 256; do
		cp "app/icons/forma_$s.png" "$iconset/icon_${s}x${s}.png"
	done
	cp app/icons/forma_32.png "$iconset/icon_16x16@2x.png"
	cp app/icons/forma_64.png "$iconset/icon_32x32@2x.png"
	cp app/icons/forma_256.png "$iconset/icon_128x128@2x.png"
	iconutil -c icns -o "$app/Contents/Resources/forma.icns" "$iconset"
	rm -rf "$iconset"
	cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Forma</string>
	<key>CFBundleDisplayName</key><string>Forma</string>
	<key>CFBundleIdentifier</key><string>br.xyz.forma</string>
	<key>CFBundleExecutable</key><string>forma</string>
	<key>CFBundleIconFile</key><string>forma</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleVersion</key><string>$version</string>
	<key>CFBundleShortVersionString</key><string>$version</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
	codesign --force --deep --sign - "$app"
	cp LICENSE README.md "$stage/"
	ditto -c -k --keepParent "$stage" "$stage.zip"
	rm -rf "$stage"
	out="$name.zip"
	(cd dist && shasum -a 256 "$out" > SHA256SUMS-macos.txt)
	;;
*)
	echo "build.sh builds on Linux or macOS; Windows has build.ps1" >&2
	exit 1
	;;
esac

echo "dist/$out  $(du -h "dist/$out" | cut -f1)"

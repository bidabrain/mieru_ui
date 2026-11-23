#!/usr/bin/env bash
# Builds macos/build/Mieru.app (universal arm64 + x86_64) and Mieru-<version>.dmg.
# Requires macOS with Xcode command line tools and Go.
#   VERSION=1.0.0 bash macos/build.sh
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-0.0.0-dev}"
BUILD_NUM="${BUILD_NUM:-$(echo "$VERSION" | sed -E 's/[^0-9.].*$//')}"
OUT=build
APP="$OUT/Mieru.app"
GEO_BASE="${GEO_BASE:-https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest}"

rm -rf "$OUT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/geo"

echo "==> mieru-core (Go)"
for arch in arm64 amd64; do
  (cd core && CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -tags with_gvisor -trimpath \
    -ldflags "-s -w -buildid= -X main.version=$VERSION" -o "../$OUT/mieru-core-$arch" .)
done
lipo -create -output "$APP/Contents/MacOS/mieru-core" "$OUT/mieru-core-arm64" "$OUT/mieru-core-amd64"

echo "==> Mieru (Swift)"
swift build -c release --arch arm64 --arch x86_64
BIN_DIR="$(swift build -c release --arch arm64 --arch x86_64 --show-bin-path)"
cp "$BIN_DIR/Mieru" "$APP/Contents/MacOS/Mieru"

echo "==> resources"
sed -e "s/__VERSION__/$VERSION/g" -e "s/__BUILD__/${BUILD_NUM:-0}/g" Resources/Info.plist > "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist"
install -m 755 Resources/install-helper.sh "$APP/Contents/Resources/install-helper.sh"
printf 'APPL????' > "$APP/Contents/PkgInfo"

swift scripts/make-icon.swift "$OUT/AppIcon.iconset"
iconutil -c icns -o "$APP/Contents/Resources/AppIcon.icns" "$OUT/AppIcon.iconset"

if [[ "${SKIP_GEO:-0}" != 1 ]]; then
  curl -fsSL --retry 3 -o "$APP/Contents/Resources/geo/GeoSite.dat" "$GEO_BASE/geosite.dat"
  curl -fsSL --retry 3 -o "$APP/Contents/Resources/geo/GeoIP.dat" "$GEO_BASE/geoip-lite.dat"
fi

echo "==> sign (ad-hoc)"
codesign --force --timestamp=none -s - "$APP/Contents/MacOS/mieru-core"
codesign --force --timestamp=none -s - "$APP"
codesign --verify --deep --strict "$APP"

echo "==> dmg"
STAGE="$OUT/dmg"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "Mieru $VERSION" -srcfolder "$STAGE" -ov -format UDZO "$OUT/Mieru-$VERSION.dmg"
(cd "$OUT" && ditto -c -k --keepParent Mieru.app "Mieru-$VERSION.zip")

ls -la "$OUT"

#!/usr/bin/env bash
# Builds android/app/libs/libcore.aar with gomobile.
# Requires: Go, ANDROID_HOME (or ANDROID_SDK_ROOT) and an NDK (ANDROID_NDK_HOME).
set -euo pipefail
cd "$(dirname "$0")"

TARGETS="${TARGETS:-android/arm64,android/arm,android/amd64}"
OUT=../app/libs/libcore.aar
mkdir -p ../app/libs

if ! command -v gomobile >/dev/null 2>&1; then
  V=$(go list -m -f '{{.Version}}' golang.org/x/mobile)
  go install "golang.org/x/mobile/cmd/gomobile@$V" "golang.org/x/mobile/cmd/gobind@$V"
  export PATH="$(go env GOPATH)/bin:$PATH"
fi

# cmfa: skip mihomo's root-only Android package manager (we use VpnService per-app rules instead)
# with_gvisor: enable the gvisor tun stack
gomobile bind -v \
  -target "$TARGETS" \
  -androidapi 21 \
  -javapkg io.github.bidabrain.mieru \
  -tags "cmfa,with_gvisor" \
  -trimpath -ldflags "-s -w -buildid=" \
  -o "$OUT" ./libcore

rm -f ../app/libs/libcore-sources.jar
echo "built $OUT"

#!/usr/bin/env bash
# Cross-compile static Linux binaries into ./dist
set -euo pipefail
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
mkdir -p dist
for arch in amd64 arm64; do
  out="dist/mieru-ui-linux-${arch}"
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X mieru-ui/config.Version=${VERSION#v}" -o "$out" .
  tar -czf "dist/mieru-ui-linux-${arch}.tar.gz" -C dist --transform "s|mieru-ui-linux-${arch}|mieru-ui|" "mieru-ui-linux-${arch}"
  echo "built $out"
done
(cd dist && sha256sum mieru-ui-linux-*.tar.gz > SHA256SUMS && cat SHA256SUMS)

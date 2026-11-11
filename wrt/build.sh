#!/usr/bin/env bash
# Builds the OpenWrt packages into wrt/dist/:
#   mieru-wrt_<ver>_<arch>.ipk   core service (one per OpenWrt package arch)
#   luci-app-mieru_<ver>_all.ipk LuCI pages
# Needs Go, curl and GNU tar. No OpenWrt SDK required: an .ipk is just a gzipped tar
# of debian-binary, control.tar.gz and data.tar.gz.
#
#   VERSION=0.1.0 bash wrt/build.sh
#   ARCHS="aarch64_generic" bash wrt/build.sh      # build a subset
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-0.0.0}"
PKG_VERSION="${VERSION}-1"
MAINTAINER="bidabrain <https://github.com/bidabrain/mieru_ui>"
# OpenWrt package arch → Go target. The same Go binary serves every CPU flavour of a family.
declare -A GOTARGET=(
  [x86_64]="amd64"
  [i386_pentium4]="386"
  [aarch64_generic]="arm64"
  [aarch64_cortex-a53]="arm64"
  [aarch64_cortex-a72]="arm64"
  [aarch64_cortex-a76]="arm64"
  [arm_cortex-a5_vfpv4]="arm:7"
  [arm_cortex-a7]="arm:7"
  [arm_cortex-a7_vfpv4]="arm:7"
  [arm_cortex-a7_neon-vfpv4]="arm:7"
  [arm_cortex-a8_vfpv3]="arm:7"
  [arm_cortex-a9]="arm:5"
  [arm_cortex-a9_vfpv3-d16]="arm:7"
  [arm_cortex-a9_neon]="arm:7"
  [arm_cortex-a15_neon-vfpv4]="arm:7"
  [arm_arm1176jzf-s_vfp]="arm:6"
  [mipsel_24kc]="mipsle:softfloat"
  [mips_24kc]="mips:softfloat"
  [riscv64_riscv64]="riscv64"
)
ARCHS="${ARCHS:-${!GOTARGET[*]}}"
GEO_BASE="${GEO_BASE:-https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest}"

TAR=(tar --owner=0 --group=0 --numeric-owner --sort=name --mtime=@0)
BUILD=build
DIST=dist
rm -rf "$BUILD" "$DIST"
mkdir -p "$BUILD/geo" "$BUILD/bin" "$DIST"

echo "==> geo data"
curl -fsSL -o "$BUILD/geo/GeoSite.dat" "$GEO_BASE/geosite.dat"
curl -fsSL -o "$BUILD/geo/GeoIP.dat" "$GEO_BASE/geoip-lite.dat"

# mk_ipk <name> <arch> <depends> <description> <rootdir> <ctrldir>
mk_ipk() {
  local name=$1 arch=$2 depends=$3 desc=$4 root=$5 ctrl=$6
  local w; w="$(mktemp -d)"
  (cd "$root" && "${TAR[@]}" -czf "$w/data.tar.gz" .)
  local size; size=$(du -sk --apparent-size "$root" | cut -f1)
  {
    echo "Package: $name"
    echo "Version: $PKG_VERSION"
    echo "Depends: $depends"
    echo "Source: https://github.com/bidabrain/mieru_ui"
    echo "License: GPL-3.0-only"
    echo "Section: net"
    echo "Architecture: $arch"
    echo "Installed-Size: $((size * 1024))"
    echo "Maintainer: $MAINTAINER"
    echo "Description: $desc"
  } > "$ctrl/control"
  (cd "$ctrl" && "${TAR[@]}" -czf "$w/control.tar.gz" .)
  echo "2.0" > "$w/debian-binary"
  (cd "$w" && "${TAR[@]}" -czf "$OLDPWD/$DIST/${name}_${PKG_VERSION}_${arch}.ipk" ./debian-binary ./data.tar.gz ./control.tar.gz)
  rm -rf "$w"
  echo "    $DIST/${name}_${PKG_VERSION}_${arch}.ipk"
}

echo "==> mieru-wrt"
for arch in $ARCHS; do
  t="${GOTARGET[$arch]:?unknown arch $arch}"
  goarch="${t%%:*}"; goarm=""; gomips=""
  if [[ "$t" == *:* ]]; then
    case "$goarch" in
      mips|mipsle) gomips="${t#*:}" ;;
      *) goarm="${t#*:}" ;;
    esac
  fi
  bin="$BUILD/bin/${t/:/-}/mieru-wrt"
  if [[ ! -f "$bin" ]]; then
    echo "    go build $t"
    (cd core && CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" GOMIPS="$gomips" \
      go build -trimpath -ldflags "-s -w -buildid= -X main.version=$VERSION" -o "../$bin" .)
  fi

  root="$BUILD/root-$arch"; ctrl="$BUILD/ctrl-$arch"
  rm -rf "$root" "$ctrl"; mkdir -p "$root/usr/bin" "$root/usr/share/mieru" "$ctrl"
  cp -r mieru-wrt/root/. "$root/"
  install -m 0755 "$bin" "$root/usr/bin/mieru-wrt"
  install -m 0644 "$BUILD/geo/"*.dat "$root/usr/share/mieru/"
  chmod 0755 "$root/etc/init.d/mieru"
  chmod 0644 "$root/etc/config/mieru"
  echo "/etc/config/mieru" > "$ctrl/conffiles"
  install -m 0755 mieru-wrt/postinst mieru-wrt/prerm "$ctrl/"
  mk_ipk mieru-wrt "$arch" "kmod-nft-tproxy, ip-full, ca-bundle" \
    "Mieru proxy client for OpenWrt (mihomo core with the official mieru library), transparent proxy for the LAN." \
    "$root" "$ctrl"
done

echo "==> luci-app-mieru"
root="$BUILD/root-luci"; ctrl="$BUILD/ctrl-luci"
mkdir -p "$root" "$ctrl"
cp -r luci-app-mieru/root/. "$root/"
find "$root" -type f -exec chmod 0644 {} +
install -m 0755 luci-app-mieru/postinst "$ctrl/"
mk_ipk luci-app-mieru all "luci-base, mieru-wrt" "LuCI pages for the Mieru proxy client (Services > Mieru)." "$root" "$ctrl"

(cd "$DIST" && sha256sum -- *.ipk > SHA256SUMS-wrt && cat SHA256SUMS-wrt)

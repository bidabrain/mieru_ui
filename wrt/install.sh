#!/bin/sh
# Install / upgrade the Mieru OpenWrt plugin from the latest wrt-v* GitHub release.
#
#   sh -c "$(wget -qO- https://raw.githubusercontent.com/bidabrain/mieru_ui/main/wrt/install.sh)"
#
# Environment: MIERU_WRT_TAG=wrt-v0.1.0 to pin a version, GH_PROXY=https://mirror/ to
# prefix GitHub download URLs (for networks where github.com is slow).
set -e

REPO=bidabrain/mieru_ui
ARCHS="x86_64 i386_pentium4 aarch64_generic aarch64_cortex-a53 aarch64_cortex-a72 aarch64_cortex-a76 arm_cortex-a5_vfpv4 arm_cortex-a7 arm_cortex-a7_vfpv4 arm_cortex-a7_neon-vfpv4 arm_cortex-a8_vfpv3 arm_cortex-a9 arm_cortex-a9_vfpv3-d16 arm_cortex-a9_neon arm_cortex-a15_neon-vfpv4 arm_arm1176jzf-s_vfp mipsel_24kc mips_24kc riscv64_riscv64"

say() { printf '\033[32m%s\033[0m\n' "$*"; }
die() { printf '\033[31m%s\033[0m\n' "$*" >&2; exit 1; }

command -v opkg >/dev/null || die "只支持使用 opkg 的 OpenWrt / ImmortalWrt（23.05 / 24.10）"

fetch() { # url dest
	if command -v curl >/dev/null; then curl -fL --retry 2 -o "$2" "$1"
	else wget -O "$2" "$1"; fi
}

# Pick the highest-priority package arch of this router that we build for.
arch=""
for a in $(opkg print-architecture | awk '{print $3, $2}' | sort -rn | awk '{print $2}'); do
	case " $ARCHS " in *" $a "*) arch=$a; break ;; esac
done
[ -n "$arch" ] || die "不支持的架构：$(opkg print-architecture | awk '{print $2}' | tr '\n' ' ')"

tag="$MIERU_WRT_TAG"
if [ -z "$tag" ]; then
	tmpj=/tmp/mieru-releases.json
	fetch "https://api.github.com/repos/$REPO/releases?per_page=30" "$tmpj" >/dev/null 2>&1 || die "无法访问 GitHub API"
	tag=$(grep -o '"tag_name": *"wrt-v[^"]*"' "$tmpj" | head -n1 | sed 's/.*"\(wrt-v[^"]*\)"/\1/')
	rm -f "$tmpj"
fi
[ -n "$tag" ] || die "没有找到 wrt-v* 发布版本"
ver=${tag#wrt-v}
base="${GH_PROXY}https://github.com/$REPO/releases/download/$tag"

say "架构 $arch，版本 $ver"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
fetch "$base/mieru-wrt_${ver}-1_${arch}.ipk" "$dir/mieru-wrt.ipk"
fetch "$base/luci-app-mieru_${ver}-1_all.ipk" "$dir/luci-app-mieru.ipk"
fetch "$base/SHA256SUMS-wrt" "$dir/SHA256SUMS-wrt"
(
	cd "$dir"
	for f in "mieru-wrt_${ver}-1_${arch}.ipk" "luci-app-mieru_${ver}-1_all.ipk"; do
		want=$(grep " \*\?$f\$" SHA256SUMS-wrt | awk '{print $1}')
		short=${f%%_*}.ipk
		got=$(sha256sum "$short" | awk '{print $1}')
		[ -n "$want" ] && [ "$want" = "$got" ] || die "$f SHA256 校验失败"
	done
)
say "SHA256 校验通过"

opkg update >/dev/null 2>&1 || say "opkg update 失败，尝试直接安装"
opkg install "$dir/mieru-wrt.ipk" "$dir/luci-app-mieru.ipk"

say "安装完成：打开 LuCI「服务 → Mieru」，在「节点」页导入 mierus:// 链接，然后在「基本设置」中启用。"

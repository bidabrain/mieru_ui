#!/bin/sh
# Packs the package trees staged by build.sh (build/root-*, build/ctrl-*) into .apk
# packages for OpenWrt / ImmortalWrt 25.12+ (apk-tools v3 ADB format). Needs `apk mkpkg`,
# which OpenWrt only builds for the host, so it runs in an Alpine edge container (apk-tools
# v3 with mkpkg), with the same options as OpenWrt's include/package-pack.mk:
#
#   bash wrt/build.sh
#   docker run --rm -v "$PWD/wrt:/w" -w /w -e VERSION=1.2.3 \
#     alpine:edge sh build-apk.sh
set -e
cd "$(dirname "$0")"

VERSION="${VERSION:-0.0.0}"
APKVER="${VERSION}-r1"
URL=https://github.com/bidabrain/mieru_ui
MAINTAINER="bidabrain <$URL>"
DIST=dist
mkdir -p "$DIST"

# mk <name> <arch|all> <depends (space separated)> <description> <rootdir> <ctrldir>
mk() {
	name=$1 arch=$2 deps=$3 desc=$4 root=$5 ctrl=$6
	tree=$(mktemp -d) scripts=$(mktemp -d)
	# Plain copy as root: files in the package must be owned by root, not the CI user.
	cp -R "$root/." "$tree/"
	mkdir -p "$tree/lib/apk/packages"

	(cd "$tree" && find . \( -type f -o -type l \) | sed 's#^\.##' | sort) > "$scripts/list"
	mv "$scripts/list" "$tree/lib/apk/packages/$name.list"
	if [ -f "$ctrl/conffiles" ]; then
		cp "$ctrl/conffiles" "$tree/lib/apk/packages/$name.conffiles"
		while read -r f; do
			[ -f "$tree$f" ] && echo "$f $(sha256sum "$tree$f" | cut -d' ' -f1)"
		done < "$ctrl/conffiles" > "$tree/lib/apk/packages/$name.conffiles_static"
	fi

	# Same wrappers OpenWrt generates: run the default package hooks, then ours.
	{
		echo '#!/bin/sh'
		echo '[ "${IPKG_NO_SCRIPT}" = "1" ] && exit 0'
		echo '[ -s "${IPKG_INSTROOT}/lib/functions.sh" ] || exit 0'
		echo '. ${IPKG_INSTROOT}/lib/functions.sh'
		echo 'export root="${IPKG_INSTROOT}"'
		echo "export pkgname=\"$name\""
		echo 'add_group_and_user'
		echo 'default_postinst'
		[ ! -f "$ctrl/postinst" ] || sed '/^#!/d' "$ctrl/postinst"
	} > "$scripts/post-install"
	{
		echo '#!/bin/sh'
		echo 'export PKG_UPGRADE=1'
		sed '/^#!/d' "$scripts/post-install"
	} > "$scripts/post-upgrade"
	{
		echo '#!/bin/sh'
		echo '[ -s "${IPKG_INSTROOT}/lib/functions.sh" ] || exit 0'
		echo '. ${IPKG_INSTROOT}/lib/functions.sh'
		echo 'export root="${IPKG_INSTROOT}"'
		echo "export pkgname=\"$name\""
		echo 'default_prerm'
		[ ! -f "$ctrl/prerm" ] || sed '/^#!/d' "$ctrl/prerm"
	} > "$scripts/pre-deinstall"

	apkarch=$arch
	[ "$arch" = all ] && apkarch=noarch
	out="$DIST/${name}_${APKVER}_${arch}.apk"
	apk mkpkg \
		--info "name:$name" \
		--info "version:$APKVER" \
		--info "description:$desc" \
		--info "arch:$apkarch" \
		--info "license:GPL-3.0-only" \
		--info "origin:$name" \
		--info "url:$URL" \
		--info "maintainer:$MAINTAINER" \
		--info "depends:$deps" \
		--script "post-install:$scripts/post-install" \
		--script "post-upgrade:$scripts/post-upgrade" \
		--script "pre-deinstall:$scripts/pre-deinstall" \
		--files "$tree" \
		--output "$out"
	rm -rf "$tree" "$scripts"
	echo "    $out"
}

echo "==> apk packages ($APKVER)"
for root in build/root-*; do
	arch=${root#build/root-}
	[ "$arch" = luci ] && continue
	mk mieru-wrt "$arch" "kmod-nft-tproxy ip-full ca-bundle" \
		"Mieru proxy client for OpenWrt (mihomo core with the official mieru library), transparent proxy for the LAN." \
		"$root" "build/ctrl-$arch"
done
mk luci-app-mieru all "luci-base mieru-wrt" "LuCI pages for the Mieru proxy client (Services > Mieru)." build/root-luci build/ctrl-luci

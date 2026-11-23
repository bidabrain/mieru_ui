#!/bin/sh
# Installs or removes the Mieru core daemon. Run as root by the app (one admin prompt).
#   install-helper.sh install <mieru-core path> <owner uid> <geo seed dir>
#   install-helper.sh uninstall
set -eu

LABEL=io.github.bidabrain.mieru.core
BIN=/Library/PrivilegedHelperTools/$LABEL
PLIST=/Library/LaunchDaemons/$LABEL.plist
DATA="/Library/Application Support/Mieru"
LOGDIR=/Library/Logs/Mieru
SOCK=/var/run/mieru-core.sock

stop_daemon() {
  launchctl bootout "system/$LABEL" 2>/dev/null || true
  i=0
  while launchctl print "system/$LABEL" >/dev/null 2>&1 && [ $i -lt 10 ]; do sleep 0.5; i=$((i + 1)); done
}

case "${1:-}" in
install)
  src="$2"; uid="$3"; seed="${4:-}"
  case "$uid" in ''|*[!0-9]*) echo "invalid uid" >&2; exit 2 ;; esac
  [ -f "$src" ] || { echo "missing $src" >&2; exit 2; }

  stop_daemon
  mkdir -p /Library/PrivilegedHelperTools "$DATA/geo" "$LOGDIR"
  cp -f "$src" "$BIN.tmp"
  chown root:wheel "$BIN.tmp"
  chmod 755 "$BIN.tmp"
  xattr -c "$BIN.tmp" 2>/dev/null || true
  mv -f "$BIN.tmp" "$BIN"
  if [ -n "$seed" ]; then
    for f in GeoSite.dat GeoIP.dat; do
      if [ -f "$seed/$f" ]; then cp -f "$seed/$f" "$DATA/geo/$f"; fi
    done
  fi

  cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$BIN</string>
    <string>daemon</string>
    <string>-owner</string>
    <string>$uid</string>
    <string>-data</string>
    <string>$DATA</string>
    <string>-seed</string>
    <string>$DATA/geo</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$LOGDIR/core.log</string>
  <key>StandardErrorPath</key><string>$LOGDIR/core.log</string>
</dict>
</plist>
EOF
  chown root:wheel "$PLIST"
  chmod 644 "$PLIST"

  # bootstrap can briefly fail while the old instance is still being torn down
  i=0
  until launchctl bootstrap system "$PLIST" 2>/dev/null; do
    i=$((i + 1))
    if [ $i -ge 6 ]; then
      launchctl bootstrap system "$PLIST"
      break
    fi
    sleep 1
  done
  # wait for the socket
  i=0
  while [ ! -S "$SOCK" ] && [ $i -lt 20 ]; do sleep 0.25; i=$((i + 1)); done
  ;;
uninstall)
  stop_daemon
  rm -f "$PLIST" "$BIN" "$SOCK"
  ;;
*)
  echo "usage: $0 install <core> <uid> <seed> | uninstall" >&2
  exit 2
  ;;
esac

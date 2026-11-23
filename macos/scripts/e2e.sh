#!/usr/bin/env bash
# End-to-end test of the core daemon as root with a real utun (CI: passwordless sudo).
# Uses direct mode, so the machine keeps its network while the tun is up.
#   bash macos/scripts/e2e.sh macos/build/Mieru.app
set -Eeuxo pipefail

APP="${1:-macos/build/Mieru.app}"
CORE="$APP/Contents/MacOS/mieru-core"
SOCK=/tmp/mc-root.sock
LOG=/tmp/mc-core.log

SVC=$(networksetup -listallnetworkservices | sed 1d | grep -v '^\*' | head -1)
networksetup -getdnsservers "$SVC" | tee /tmp/dns-before

sudo "$CORE" daemon -socket $SOCK -data /tmp/mc-root -owner "$(id -u)" -seed "$APP/Contents/Resources/geo" > $LOG 2>&1 &
for _ in $(seq 1 40); do [ -S $SOCK ] && break; sleep 0.25; done

api() {
  local body="${2:-}"
  [ -n "$body" ] || body='{}'
  curl -sS --max-time 90 --unix-socket $SOCK -X POST "http://x/$1" -d "$body"
  echo
}

FAIL=""
trap 'FAIL="line $LINENO: $BASH_COMMAND"' ERR
cleanup() {
  set +e
  echo "----- core buffered log (tail)"
  api logs '{"after":0}' | tail -c 3000
  api stop >/dev/null 2>&1
  sudo pkill -f "mieru-core daemon -socket $SOCK"
  echo "----- dig: $(cat /tmp/dig.txt 2>/dev/null | tr '
' ' ')"
  echo "----- start: $(cat /tmp/start.json 2>/dev/null)"
  echo "----- traffic: $(cat /tmp/traffic.json 2>/dev/null)"
  echo "----- curl codes: ${codes:-}"
  [ -z "$FAIL" ] || echo "FAILED AT $FAIL"
}
trap cleanup EXIT

OPTS='{"profile":{"name":"t","server":"127.0.0.1","username":"u","password":"p","bindings":[{"port":2012,"protocol":"TCP"}]},
 "routing":{"mode":"direct","bypassLan":true,"directDns":["https://doh.pub/dns-query","223.5.5.5"],"remoteDns":["https://1.1.1.1/dns-query"],"fakeIp":true,"sniff":true},
 "stack":"gvisor","logLevel":"info","mixedPort":17890,"desktop":{"perAppMode":"off","apps":[]}}'

api status
api start "{\"options\":$OPTS}" | tee /tmp/start.json
grep -q '"result":true' /tmp/start.json
api status > /tmp/status.json; grep '"running":true' /tmp/status.json
ifconfig | grep -B1 -A4 '198.18.0.1' || true
netstat -rn -f inet | head -30 || true
networksetup -getdnsservers "$SVC" > /tmp/dns-now; cat /tmp/dns-now; grep -qx 198.18.0.2 /tmp/dns-now

# fake-ip answer from the hijacked DNS, then real traffic through the tun
dig +short +time=5 +tries=2 www.gstatic.com @198.18.0.2 | tee /tmp/dig.txt
grep -q '^198\.18\.' /tmp/dig.txt
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 https://www.gstatic.com/generate_204 || true)
codes="tun=$code"
[ "$code" = 204 ]
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 -x http://127.0.0.1:17890 https://www.gstatic.com/generate_204 || true)
codes="$codes mixed=$code"
[ "$code" = 204 ]
api traffic | tee /tmp/traffic.json
grep -q '"downTotal":[1-9]' /tmp/traffic.json

api stop > /tmp/stop.json; grep -q '"ok":true' /tmp/stop.json
sleep 1
if ifconfig | grep 'inet 198.18.0.1' >/dev/null; then echo "tun still up"; exit 1; fi
networksetup -getdnsservers "$SVC" > /tmp/dns-after; diff /tmp/dns-after /tmp/dns-before
echo "E2E OK"

#!/usr/bin/env bash
# End-to-end test (CI): install the x86_64 packages into an ImmortalWrt 23.05 container,
# put a fake LAN client behind it and check its traffic is transparently proxied to a
# mieru test server. Run after `bash wrt/build.sh` (needs Docker, Go, sudo).
set -Eeuo pipefail
cd "$(dirname "$0")/.."

IMG="${IMG:-immortalwrt/rootfs:x86_64-openwrt-23.05}"
LOG=build/e2e
mkdir -p "$LOG"

note() { echo "::notice title=$1::$(printf '%s' "$2" | sed ':a;N;$!ba;s/%/%25/g;s/\r//g;s/\n/%0A/g')"; }
x() { docker exec wrt sh -c "$*"; }

STEP=init
step() { STEP="$*"; echo "== $*"; }

dump() {
  set +e
  {
    for l in "$LOG"/*.txt "$LOG"/*.json; do [ "$l" = "$LOG/dump.txt" ] || { echo "== $l"; tail -25 "$l"; }; done
    echo "== logread"; x "logread -e mieru" | tail -25
    echo "== nft"; x "nft list table inet mieru" | head -30
    echo "== testsrv"; tail -10 "$LOG/testsrv.log"
  } > "$LOG/dump.txt" 2>&1
  cat "$LOG/dump.txt"
  note "e2e diagnostics ($STEP)" "$(head -c 3500 "$LOG/dump.txt")"
}

on_exit() {
  rc=$?
  if [ $rc -ne 0 ]; then echo "::error title=e2e failed::step: $STEP (exit $rc)"; dump; fi
  kill %1 2>/dev/null || true
  docker rm -f wrt >/dev/null 2>&1 || true
  exit $rc
}
trap on_exit EXIT

# Kernel bits the container uses from the host kernel.
sudo modprobe -a nft_tproxy nf_tproxy_ipv4 nft_socket nf_socket_ipv4 nft_fib_inet nft_redir veth bridge || true

step "mieru test server"
(cd core && go build -o ../build/testsrv ./testsrv)
./build/testsrv -mirror https://downloads.immortalwrt.org > "$LOG/testsrv.log" 2>&1 &

step "boot $IMG"
# Replace the network config before procd starts: a bare br-lan "LAN" and leave Docker's
# eth0 (our WAN, with its address and default route) alone.
docker run -d --name wrt --privileged -v "$PWD/dist:/pkgs:ro" "$IMG" sh -c '
cat > /etc/config/network <<EOF
config interface "loopback"
	option device "lo"
	option proto "static"
	option ipaddr "127.0.0.1"
	option netmask "255.0.0.0"
config device
	option name "br-lan"
	option type "bridge"
	option bridge_empty "1"
config interface "lan"
	option device "br-lan"
	option proto "static"
	option ipaddr "192.168.1.1"
	option netmask "255.255.255.0"
EOF
# No WAN interface is managed here, so give dnsmasq Docker'"'"'s resolvers as upstream.
for ns in $(awk "/^nameserver/{print \$2}" /etc/resolv.conf); do
	printf "\tlist server \"%s\"\n" "$ns" >> /tmp/dns.add
done
sed -i "/^config dnsmasq/r /tmp/dns.add" /etc/config/dhcp
sed -i "/^config dnsmasq/a \toption noresolv \"1\"" /etc/config/dhcp
exec /sbin/init'
for i in $(seq 1 30); do x 'ubus call system board >/dev/null 2>&1' && break; sleep 1; done
sleep 3
# netifd does not manage eth0 here but still drops Docker's routes: restore them.
GW=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' wrt)
CIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}/{{.IPPrefixLen}}{{end}}' wrt)
x "ip link set eth0 up; ip addr add $CIP dev eth0 2>/dev/null; ip route replace default via $GW dev eth0"
x 'ip -4 addr show eth0; ip -4 route'
[ -n "$GW" ]
echo "host (mieru server) at $GW"
x "wget -q -T 10 -O /dev/null http://$GW:8080/releases/ && echo mirror reachable"

step "install packages"
# The rootfs image has no TLS-capable wget and the container's own DNS/egress is not
# reliable on CI: fetch the feeds through the test server's HTTP mirror on the host.
x "sed -i -E 's#https?://[^ ]+/(releases|snapshots)/#http://$GW:8080/\1/#' /etc/opkg/distfeeds.conf; cat /etc/opkg/distfeeds.conf"
x 'opkg update' > "$LOG/opkg-update.txt" 2>&1 || { cat "$LOG/opkg-update.txt"; false; }
if ! x 'opkg install /pkgs/mieru-wrt_*_x86_64.ipk /pkgs/luci-app-mieru_*_all.ipk' > "$LOG/opkg.txt" 2>&1; then
  cat "$LOG/opkg.txt"
  note "opkg dependencies" "$(cat "$LOG/opkg.txt")"
  # Container rootfs images lack the kernel package kmods depend on; the real router has it.
  x 'opkg install --force-depends /pkgs/mieru-wrt_*_x86_64.ipk /pkgs/luci-app-mieru_*_all.ipk'
fi
x 'mieru-wrt version'
x 'test -f /www/luci-static/resources/view/mieru/settings.js && test -f /usr/share/rpcd/acl.d/luci-app-mieru.json'
x '/etc/init.d/mieru enabled'

step "fake LAN client 192.168.1.2"
x 'ip link add veth0 type veth peer name veth1 && ip link set veth0 master br-lan && ip link set veth0 up'
x 'ip netns add client && ip link set veth1 netns client'
x 'ip netns exec client sh -c "ip addr add 192.168.1.2/24 dev veth1; ip link set veth1 up; ip link set lo up; ip route add default via 192.168.1.1"'
x 'mkdir -p /etc/netns/client && echo "nameserver 8.8.8.8" > /etc/netns/client/resolv.conf'
x 'ip netns exec client ping -c1 -W2 192.168.1.1 >/dev/null'

step "configure from a mierus:// link"
LINK="mierus://alice:testpass@${GW}?profile=ci&port=2012&protocol=TCP"
x "mieru-wrt parse-link '$LINK'" > "$LOG/parse.json" 2>&1; cat "$LOG/parse.json"
grep -q '"port":\["2012/TCP"\]' "$LOG/parse.json"
x "sid=\$(uci add mieru node)
   uci set mieru.\$sid.name=ci; uci set mieru.\$sid.server=$GW
   uci set mieru.\$sid.username=alice; uci set mieru.\$sid.password=testpass
   uci add_list mieru.\$sid.port=2012/TCP
   uci set mieru.main.node=\$sid; uci set mieru.main.mode=global; uci set mieru.main.enabled=1
   uci delete mieru.main.direct_dns; uci add_list mieru.main.direct_dns=8.8.8.8
   uci commit mieru"
x 'mieru-wrt check; uci show mieru' > "$LOG/check.txt" 2>&1; cat "$LOG/check.txt"; x 'mieru-wrt check'
x 'mieru-wrt gen' > "$LOG/gen.txt"

step "start"
x '/etc/init.d/mieru start'
for i in $(seq 1 20); do x 'nft list table inet mieru >/dev/null 2>&1' && break; sleep 1; done
x 'nft list table inet mieru' | tee "$LOG/nft.txt"
x 'ip rule | grep -q "lookup 162"'
x 'grep -q "server=127.0.0.1#7874" /tmp/dnsmasq*.d/mieru.conf'

step "delay test"
x 'mieru-wrt delay' | tee "$LOG/delay.json"
grep -q '"ms"' "$LOG/delay.json"

step "DNS from the client (hijacked to dnsmasq → core, fake-ip)"
# dnsmasq was just restarted by mieru-wrt; give it a moment.
for i in $(seq 1 10); do
  x "ip netns exec client nslookup example.com 8.8.8.8" > "$LOG/dns.txt" 2>&1 || true
  grep -q "198.18." "$LOG/dns.txt" && break
  sleep 1
done
cat "$LOG/dns.txt"
x "pgrep -a dnsmasq; nslookup example.com 127.0.0.1; ls /tmp/dnsmasq*.d" > "$LOG/dnsmasq.txt" 2>&1 || true
grep -q "198.18." "$LOG/dns.txt"

step "HTTP from the client through the transparent proxy"
: > "$LOG/http.txt"
x 'ip netns exec client wget -T 15 -qO- http://example.com/' > "$LOG/http.txt"
grep -qi 'example domain' "$LOG/http.txt"
sleep 1
grep -q 'CONNECT example.com:80' "$LOG/testsrv.log"

step "LuCI pages in headless Chrome"
x 'opkg install luci luci-theme-bootstrap' > "$LOG/opkg-luci.txt" 2>&1 || { cat "$LOG/opkg-luci.txt"; false; }
x 'uci set luci.main.mediaurlbase=/luci-static/bootstrap; uci commit luci; /etc/init.d/uhttpd restart; /etc/init.d/rpcd restart'
# eth0 (the Docker side) is in no firewall zone, so fw4 rejects input on it: open HTTP
# for the test browser.
x 'nft insert rule inet fw4 input iifname eth0 tcp dport 80 accept; pgrep -a uhttpd'
sleep 3
ROUTER=${CIP%/*}
node test/luci.mjs "http://$ROUTER" "mierus://bob:pw@${GW}?profile=imported&port=2013&protocol=TCP" > "$LOG/luci.txt" 2>&1 || { cat "$LOG/luci.txt"; note "LuCI test output" "$(tail -c 2500 "$LOG/luci.txt")"; false; }
cat "$LOG/luci.txt"

step "reload keeps working"
x 'uci set mieru.main.mode=gfw; uci commit mieru; /etc/init.d/mieru reload'
sleep 4
x 'nft list table inet mieru >/dev/null'

step "stop removes everything"
x '/etc/init.d/mieru stop'
sleep 3
if x 'nft list table inet mieru' >/dev/null 2>&1; then echo "table still present"; false; fi
if x 'ip rule | grep -q "lookup 162"'; then echo "ip rule still present"; false; fi
if x 'ls /tmp/dnsmasq*.d/mieru.conf' >/dev/null 2>&1; then echo "dnsmasq conf still present"; false; fi

note "e2e passed" "delay: $(cat "$LOG/delay.json")
dns: $(grep -A2 'Name:' "$LOG/dns.txt" | tr '\n' ' ')
testsrv: $(grep CONNECT "$LOG/testsrv.log" | tail -5)"
echo "E2E OK"

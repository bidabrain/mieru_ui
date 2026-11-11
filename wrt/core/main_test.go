package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bidabrain/mieru_ui/android/core/libcore"
)

const sample = `
# comment
config mieru 'main'
	option enabled '1'
	option node 'jp'
	option mode 'gfw'
	option block_ads "1"
	list direct_dns 'https://doh.pub/dns-query'
	list remote_dns 'https://1.1.1.1/dns-query'
	list rule 'DOMAIN-SUFFIX,example.com,PROXY'
	list rule 'IP-CIDR,1.2.3.0/24,DIRECT'
	option access_mode 'exclude'
	list access_ip '192.168.1.50'
	list access_ip '192.168.1.128/25'
	list lan_ifname 'br-lan'
	list lan_ifname 'br-guest'
	option tproxy_port '7893'
	option dns_port '7874'

config node 'other'
	option server 'b.example.com'
	option username 'bob'
	option password 'x'
	list port '2012'

config node 'jp'
	option name 'Tokyo 1'
	option server '203.0.113.7'
	option username 'alice'
	option password 'it'\''s "q" #not-a-comment'
	list port '2012/TCP'
	list port '3000-3010/udp'
	option multiplexing 'MULTIPLEXING_LOW'
`

func TestUCITokens(t *testing.T) {
	cases := map[string][]string{
		`option a 'b c'`:         {"option", "a", "b c"},
		`option a "x\"y"`:        {"option", "a", `x"y`},
		`option a 'it'\''s'`:     {"option", "a", "it's"},
		`list x plain # comment`: {"list", "x", "plain"},
		`option a '#hash' # c`:   {"option", "a", "#hash"},
		`config node`:            {"config", "node"},
		"\toption\tk\t'v'":       {"option", "k", "v"},
	}
	for in, want := range cases {
		got, err := uciTokens(in)
		if err != nil || strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %q (%v), want %q", in, got, err, want)
		}
	}
	if _, err := uciTokens(`option a 'open`); err == nil {
		t.Error("unterminated quote should fail")
	}
}

func TestBuildSettings(t *testing.T) {
	secs, err := parseUCI(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	s, err := buildSettings(secs)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Core.Profile
	if p.Name != "Tokyo 1" || p.Username != "alice" || p.Password != `it's "q" #not-a-comment` {
		t.Errorf("profile: %+v", p)
	}
	if len(p.Bindings) != 2 || p.Bindings[1].PortRange != "3000-3010" || p.Bindings[1].Protocol != "UDP" {
		t.Errorf("bindings: %+v", p.Bindings)
	}
	rt := s.Core.Routing
	if rt.Mode != "gfw" || !rt.BlockAds || len(rt.Rules) != 2 || rt.RemoteDNS[0] != "https://1.1.1.1/dns-query" {
		t.Errorf("routing: %+v", rt)
	}
	if s.Core.Router.TProxyPort != 7893 || s.Core.Router.DNSPort != 7874 || s.Fwmark != 0x162 || s.Table != 162 {
		t.Errorf("router: %+v fw=%x table=%d", s.Core.Router, s.Fwmark, s.Table)
	}
	if len(s.LanIfnames) != 2 || s.AccessMode != "exclude" || len(s.AccessIPs) != 2 || s.AccessIPs[0].String() != "192.168.1.50/32" {
		t.Errorf("access: %+v %+v", s.LanIfnames, s.AccessIPs)
	}

	// The whole thing must build a config the core accepts.
	opts, _ := json.Marshal(s.Core)
	cfg, err := libcore.PreviewConfig(string(opts))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"tproxy-port": 7893`, `"listen": "127.0.0.1:7874"`, `DOMAIN-SUFFIX,example.com,Proxy`, `"port-range": "3000-3010"`} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %s", want)
		}
	}
	if strings.Contains(cfg, `"tun"`) {
		t.Error("router config must not contain tun")
	}

	nft := nftRuleset(s)
	for _, want := range []string{
		`iifname != { "br-lan", "br-guest" } return`,
		`ip saddr @access4 return`,
		`elements = { 192.168.1.50/32, 192.168.1.128/25 }`,
		`th dport 53 return`,
		`meta mark set 0x162 tproxy ip to 127.0.0.1:7893 accept`,
		`redirect to :53`,
	} {
		if !strings.Contains(nft, want) {
			t.Errorf("nft lacks %q:\n%s", want, nft)
		}
	}
}

func TestBuildSettingsErrors(t *testing.T) {
	bad := map[string]string{
		"no node":        "config mieru 'main'\n\toption enabled '1'\n",
		"missing node":   "config mieru 'main'\n\toption node 'x'\n",
		"bad port":       "config mieru 'main'\n\toption node 'n'\nconfig node 'n'\n\toption server 's'\n\toption username 'u'\n\toption password 'p'\n\tlist port 'abc'\n",
		"include empty":  strings.Replace(strings.Replace(sample, "'exclude'", "'include'", 1), "\tlist access_ip '192.168.1.50'\n\tlist access_ip '192.168.1.128/25'\n", "", 1),
		"bad rule":       strings.Replace(sample, "'IP-CIDR,1.2.3.0/24,DIRECT'", "'IP-CIDR,1.2.3.0/24'", 1),
		"ipv6 access ip": strings.Replace(sample, "'192.168.1.50'", "'fd00::1'", 1),
	}
	bad["bad ifname"] = strings.Replace(sample, "'br-guest'", "'br guest;drop'", 1)
	for name, conf := range bad {
		secs, err := parseUCI(strings.NewReader(conf))
		if err != nil {
			continue
		}
		if _, err := buildSettings(secs); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestIncludeModeAndNoHijack(t *testing.T) {
	conf := strings.Replace(sample, "'exclude'", "'include'", 1) + "\n"
	conf = strings.Replace(conf, "option dns_port '7874'", "option dns_port '7874'\n\toption dns_hijack '0'", 1)
	secs, _ := parseUCI(strings.NewReader(conf))
	s, err := buildSettings(secs)
	if err != nil {
		t.Fatal(err)
	}
	nft := nftRuleset(s)
	if !strings.Contains(nft, "ip saddr != @access4 return") || strings.Contains(nft, "dns_hijack") || strings.Contains(nft, "th dport 53") {
		t.Errorf("unexpected nft:\n%s", nft)
	}
}

func TestNameAnonymous(t *testing.T) {
	conf := "config mieru 'main'\n\toption node 'cfg02a1b2'\nconfig node\n\toption server 'a'\nconfig node\n\toption server 'b'\n"
	secs, err := parseUCI(strings.NewReader(conf))
	if err != nil {
		t.Fatal(err)
	}
	if secs[1].Name != "@node[0]" {
		t.Fatalf("anonymous default name %q", secs[1].Name)
	}
	show := "mieru.main=mieru\nmieru.main.node='cfg02a1b2'\nmieru.cfg01c3d4=node\nmieru.cfg01c3d4.server='a'\nmieru.cfg02a1b2=node\nmieru.cfg02a1b2.server='b'\n"
	nameAnonymous(secs, "mieru", show)
	if secs[1].Name != "cfg01c3d4" || secs[2].Name != "cfg02a1b2" {
		t.Fatalf("names: %q %q", secs[1].Name, secs[2].Name)
	}
	if n := findSection(secs, "node", "cfg02a1b2"); n == nil || n.get("server", "") != "b" {
		t.Fatal("selected node not found by uci id")
	}
	// Mismatched output (e.g. file changed in between) leaves names alone.
	secs2, _ := parseUCI(strings.NewReader(conf))
	nameAnonymous(secs2, "mieru", "mieru.main=mieru\n")
	if secs2[1].Name != "@node[0]" {
		t.Fatal("mismatched show output must not rename")
	}
}

func TestNodeFromJSON(t *testing.T) {
	n, err := nodeFromJSON(`{".name":"cfg01","name":"jp","server":"203.0.113.7","username":"u","password":"p","port":["2012/TCP","3000-3010/UDP"],"mtu":"1400","multiplexing":"MULTIPLEXING_LOW"}`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := nodeProfile(n)
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "cfg01" || p.Name != "jp" || p.MTU != 1400 || len(p.Bindings) != 2 || p.Multiplexing != "MULTIPLEXING_LOW" {
		t.Errorf("got %+v / %+v", n, p)
	}
	// LuCI gives a single-element list as a plain string.
	n, _ = nodeFromJSON(`{"server":"s","username":"u","password":"p","port":"2012/TCP"}`)
	if p, err := nodeProfile(n); err != nil || len(p.Bindings) != 1 {
		t.Errorf("single port: %+v %v", p, err)
	}
}

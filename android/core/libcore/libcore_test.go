package libcore

import (
	"encoding/json"
	"github.com/metacubex/mihomo/component/geodata"
	"os"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/hub/executor"
)

const panelLink = "mierus://alice:p%40ss%3Aw%2Frd@example.com?handshake-mode=HANDSHAKE_NO_WAIT&mtu=1400&multiplexing=MULTIPLEXING_LOW&port=2012&port=3000-3010&profile=alice&protocol=TCP&protocol=UDP"

func TestParseMierusLink(t *testing.T) {
	ps, err := parseImport("garbage\n" + panelLink + "\n")
	if err != nil {
		t.Fatal(err)
	}
	p := ps[0]
	if p.Username != "alice" || p.Password != "p@ss:w/rd" || p.Server != "example.com" || p.MTU != 1400 ||
		p.Multiplexing != "MULTIPLEXING_LOW" || p.HandshakeMode != "HANDSHAKE_NO_WAIT" || p.Name != "alice" {
		t.Fatalf("bad parse: %+v", p)
	}
	if len(p.Bindings) != 2 || p.Bindings[0].Port != 2012 || p.Bindings[1].PortRange != "3000-3010" || p.Bindings[1].Protocol != "UDP" {
		t.Fatalf("bad bindings: %+v", p.Bindings)
	}
	// round trip
	again, err := parseImport(profileLink(p))
	if err != nil || again[0].Password != p.Password || len(again[0].Bindings) != 2 {
		t.Fatalf("round trip failed: %v %+v", err, again)
	}
}

func TestParseIPv6Link(t *testing.T) {
	ps, err := parseImport("mierus://u:p@[2001:db8::1]?port=443&protocol=TCP")
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].Server != "2001:db8::1" || !strings.Contains(profileLink(ps[0]), "@[2001:db8::1]?") {
		t.Fatalf("ipv6 wrong: %+v %s", ps[0], profileLink(ps[0]))
	}
}

func TestParseClientJSON(t *testing.T) {
	js := `{"profiles":[{"profileName":"bob","user":{"name":"bob","password":"pw"},
	 "servers":[{"ipAddress":"1.2.3.4","portBindings":[{"port":8964,"protocol":"tcp"}]}],
	 "mtu":1400,"multiplexing":{"level":"MULTIPLEXING_HIGH"}}],"activeProfile":"bob"}`
	ps, err := parseImport(js)
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].Server != "1.2.3.4" || ps[0].Bindings[0].Protocol != "TCP" || ps[0].Multiplexing != "MULTIPLEXING_HIGH" {
		t.Fatalf("bad: %+v", ps[0])
	}
}

func TestBadLinks(t *testing.T) {
	for _, l := range []string{
		"mierus://u@h?port=1&protocol=TCP",            // no password
		"mierus://u:p@h?port=1",                       // count mismatch
		"mierus://u:p@h?port=70000&protocol=TCP",      // port
		"mierus://u:p@h?port=1&protocol=QUIC",         // protocol
		"mierus://u:p@h?port=9-3&protocol=TCP",        // range
		"mierus://u:p@h?port=1&protocol=TCP&mtu=9000", // mtu
	} {
		if _, err := parseImport(l); err == nil {
			t.Errorf("expected error for %s", l)
		}
	}
}

func TestRules(t *testing.T) {
	rt := DefaultRouting()
	rt.BlockAds = true
	rt.Rules = []Rule{
		{Type: "domain-suffix", Value: " example.org ", Action: "direct", Enabled: true},
		{Type: "IP-CIDR", Value: "2001:db8::5/64", Action: "PROXY", Enabled: true},
		{Type: "DOMAIN", Value: "off.example", Action: "REJECT", Enabled: false},
		{Type: "DST-PORT", Value: "6881-6889/51413", Action: "DIRECT", Enabled: true},
	}
	r, err := buildRules(rt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DOMAIN-SUFFIX,example.org,DIRECT", "IP-CIDR6,2001:db8::/64,Proxy,no-resolve", "DST-PORT,6881-6889/51413,DIRECT", "GEOSITE,category-ads-all,REJECT"}
	for i, w := range want {
		if r[i] != w {
			t.Errorf("rule %d = %q want %q", i, r[i], w)
		}
	}
	if r[len(r)-1] != "MATCH,Proxy" || !contains(r, "GEOIP,CN,DIRECT") || !contains(r, "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve") {
		t.Errorf("bypass_cn rules wrong: %v", r)
	}
	rt.Mode = "gfw"
	r, _ = buildRules(rt)
	if r[len(r)-1] != "MATCH,DIRECT" || !contains(r, "GEOSITE,gfw,Proxy") {
		t.Errorf("gfw rules wrong: %v", r)
	}
	rt.Mode = "direct"
	r, _ = buildRules(rt)
	if r[len(r)-1] != "MATCH,DIRECT" || contains(r, "GEOSITE,category-ads-all,REJECT") {
		t.Errorf("direct rules wrong: %v", r)
	}
}

func TestBadRules(t *testing.T) {
	for _, r := range []Rule{
		{Type: "DOMAIN", Value: "a.com,DIRECT", Action: "PROXY"},
		{Type: "DOMAIN", Value: "a.com\nMATCH", Action: "PROXY"},
		{Type: "SCRIPT", Value: "x", Action: "PROXY"},
		{Type: "DOMAIN", Value: "a.com", Action: "SomeGroup"},
		{Type: "IP-CIDR", Value: "1.2.3", Action: "DIRECT"},
		{Type: "DST-PORT", Value: "0", Action: "DIRECT"},
		{Type: "GEOSITE", Value: "c n", Action: "DIRECT"},
	} {
		if _, err := normalizeRule(r); err == nil {
			t.Errorf("expected error for %+v", r)
		}
	}
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

func testOptions(mode string) Options {
	ps, _ := parseImport(panelLink)
	rt := DefaultRouting()
	rt.Mode = mode
	return Options{Profile: ps[0], Routing: rt, IPv6: true, TunFd: 42}
}

// TestMihomoAcceptsConfig feeds the generated config through mihomo's real parser.
// Global mode needs no geo databases; set MIERU_TEST_GEO=1 to also try the geo modes
// (downloads geoip/geosite).
func TestMihomoAcceptsConfig(t *testing.T) {
	if err := setup(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	modes := []string{"global", "direct"}
	if os.Getenv("MIERU_TEST_GEO") == "1" {
		modes = append(modes, "bypass_cn", "gfw")
	}
	for _, mode := range modes {
		buf, err := buildConfig(testOptions(mode))
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		cfg, err := executor.ParseWithBytes(buf)
		if err != nil {
			t.Fatalf("%s: mihomo rejected config: %v\n%s", mode, err, buf)
		}
		if cfg.General.Tun.FileDescriptor != 42 || !cfg.General.Tun.Enable {
			t.Errorf("%s: tun not configured: %+v", mode, cfg.General.Tun)
		}
		if got := cfg.General.Tun.Inet4Address[0].String(); got != TunAddress4 {
			t.Errorf("%s: tun address %s, VpnService uses %s", mode, got, TunAddress4)
		}
		if _, ok := cfg.Proxies["alice TCP:2012"]; !ok {
			t.Errorf("%s: proxy missing", mode)
		}
		if _, ok := cfg.Proxies["alice UDP:3000-3010"]; !ok {
			t.Errorf("%s: port-range proxy missing", mode)
		}
		if _, ok := cfg.Proxies[ProxyGroup]; !ok {
			t.Errorf("%s: group missing", mode)
		}
	}
}

func TestPublicAPI(t *testing.T) {
	js, err := ParseLinks(panelLink)
	if err != nil {
		t.Fatal(err)
	}
	var ps []Profile
	if err := json.Unmarshal([]byte(js), &ps); err != nil || len(ps) != 1 {
		t.Fatal(err, js)
	}
	pj, _ := json.Marshal(ps[0])
	if l, err := ProfileLink(string(pj)); err != nil || !strings.HasPrefix(l, "mierus://alice:") {
		t.Fatal(l, err)
	}
	if out, err := CheckRule(`{"type":"ip-cidr","value":"10.1.2.3/8","action":"direct","enabled":true}`); err != nil || !strings.Contains(out, `"10.0.0.0/8"`) {
		t.Fatal(out, err)
	}
	opts, _ := json.Marshal(testOptions("bypass_cn"))
	if _, err := PreviewConfig(string(opts)); err != nil {
		t.Fatal(err)
	}
	if err := Start(string(opts), 0); err == nil {
		t.Fatal("start with fd 0 should fail")
	}
}

func TestRouterConfig(t *testing.T) {
	if err := setup(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	o := testOptions("global")
	o.Router = &Router{TProxyPort: 7893, DNSPort: 7874, RoutingMark: 255}
	o.MixedPort = 7890
	buf, err := buildConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := executor.ParseWithBytes(buf)
	if err != nil {
		t.Fatalf("mihomo rejected router config: %v\n%s", err, buf)
	}
	g := cfg.General
	if g.Tun.Enable {
		t.Error("router mode must not enable tun")
	}
	if g.TProxyPort != 7893 || g.RoutingMark != 255 || !g.AllowLan || g.MixedPort != 7890 {
		t.Errorf("router inbound wrong: tproxy=%d mark=%d allowLan=%v mixed=%d", g.TProxyPort, g.RoutingMark, g.AllowLan, g.MixedPort)
	}
	if cfg.DNS.Listen != "127.0.0.1:7874" {
		t.Errorf("dns listen %q", cfg.DNS.Listen)
	}
	for _, bad := range []Router{{TProxyPort: 0, DNSPort: 7874}, {TProxyPort: 7893, DNSPort: 7893}, {TProxyPort: 7893, DNSPort: 70000}} {
		o.Router = &bad
		if _, err := buildConfig(o); err == nil {
			t.Errorf("expected error for %+v", bad)
		}
	}
}

// UpdateGeoFor runs in its own process on OpenWrt, so it must register the geo mode
// and URLs itself (regression: "can't download MMDB database file: Get \"\"").
func TestUpdateGeoForRegistersURLs(t *testing.T) {
	if err := setup(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	geodata.SetGeodataMode(false)
	geodata.SetGeoIpUrl("")
	o := testOptions("global")
	o.Router = &Router{TProxyPort: 7893, DNSPort: 7874}
	o.GeoxSource = "github"
	js, _ := json.Marshal(o)
	_ = UpdateGeoFor(string(js)) // the download itself may fail offline
	if !geodata.GeodataMode() {
		t.Error("geodata mode not set")
	}
	if geodata.GeoIpUrl() != geoxSources["github"]["geoip"] || geodata.GeoSiteUrl() != geoxSources["github"]["geosite"] {
		t.Errorf("urls not set: %q %q", geodata.GeoIpUrl(), geodata.GeoSiteUrl())
	}
}

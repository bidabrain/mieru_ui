package libcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	// TunAddress4 must match the address the Android VpnService assigns (mihomo derives
	// its tun address from fake-ip-range with a /30 mask).
	TunAddress4 = "198.18.0.1/30"
	TunAddress6 = "fdfe:dcba:9876::1/126"
	fakeIPRange = "198.18.0.1/16"
	tunMTU      = 9000
)

var (
	ruleValueRe = regexp.MustCompile(`^[^,\s]+$`)
	geoCodeRe   = regexp.MustCompile(`^[A-Za-z0-9!@._-]+$`)
	dnsRe       = regexp.MustCompile(`^[A-Za-z0-9:/.\[\]_%#?=&-]+$`)
	ruleTypes   = map[string]bool{
		"DOMAIN": true, "DOMAIN-SUFFIX": true, "DOMAIN-KEYWORD": true, "GEOSITE": true,
		"GEOIP": true, "IP-CIDR": true, "IP-CIDR6": true, "DST-PORT": true,
	}
	privateCIDRs = []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
		"169.254.0.0/16", "100.64.0.0/10", "224.0.0.0/4", "fc00::/7", "fe80::/10",
	}
	fakeIPFilter = []string{
		"*.lan", "*.local", "*.localdomain", "+.home.arpa", "localhost.ptlogin2.qq.com",
		"+.msftconnecttest.com", "+.msftncsi.com", "+.ntp.org", "time.*.com", "+.pool.ntp.org",
		"+.stun.*.*", "+.stun.*.*.*", "+.srv.nintendo.net", "+.xboxlive.com",
	}
)

var geoxSources = map[string]map[string]string{
	"jsdelivr": {
		"geoip":   "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip-lite.dat",
		"geosite": "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
		"mmdb":    "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
		"asn":     "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
	},
	"github": {
		"geoip":   "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip-lite.dat",
		"geosite": "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		"mmdb":    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		"asn":     "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb",
	},
}

// DefaultRouting mirrors the defaults shown in the app.
func DefaultRouting() Routing {
	return Routing{
		Mode: "bypass_cn", BypassLAN: true, BlockAds: false, AppleCNDirect: true,
		DirectDNS: []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
		RemoteDNS: []string{"https://1.1.1.1/dns-query", "https://dns.google/dns-query"},
		FakeIP:    true, Sniff: true,
	}
}

func normalizeRule(r Rule) (Rule, error) {
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	r.Value = strings.TrimSpace(r.Value)
	r.Action = strings.ToUpper(strings.TrimSpace(r.Action))
	if !ruleTypes[r.Type] {
		return r, fmt.Errorf("不支持的规则类型 %q", r.Type)
	}
	if r.Action != "PROXY" && r.Action != "DIRECT" && r.Action != "REJECT" {
		return r, fmt.Errorf("规则动作无效 %q", r.Action)
	}
	if !ruleValueRe.MatchString(r.Value) {
		return r, fmt.Errorf("规则内容无效 %q（不能为空、不能含逗号或空格）", r.Value)
	}
	switch r.Type {
	case "GEOSITE", "GEOIP":
		if !geoCodeRe.MatchString(r.Value) {
			return r, fmt.Errorf("GEO 代码无效 %q", r.Value)
		}
	case "IP-CIDR", "IP-CIDR6":
		pfx, err := netip.ParsePrefix(r.Value)
		if err != nil {
			if a, err2 := netip.ParseAddr(r.Value); err2 == nil {
				pfx = netip.PrefixFrom(a, a.BitLen())
			} else {
				return r, fmt.Errorf("IP 段无效 %q", r.Value)
			}
		}
		r.Value = pfx.Masked().String()
		if pfx.Addr().Is6() {
			r.Type = "IP-CIDR6"
		} else {
			r.Type = "IP-CIDR"
		}
	case "DST-PORT":
		for _, part := range strings.Split(r.Value, "/") {
			lohi := strings.SplitN(part, "-", 2)
			for _, s := range lohi {
				n, err := strconv.Atoi(s)
				if err != nil || n < 1 || n > 65535 {
					return r, fmt.Errorf("端口无效 %q", r.Value)
				}
			}
		}
	}
	return r, nil
}

func target(action string) string {
	if action == "PROXY" {
		return ProxyGroup
	}
	return action
}

// buildRules renders the mihomo rule list, first match wins.
func buildRules(rt Routing) ([]string, error) {
	var rules []string
	add := func(s ...string) { rules = append(rules, s...) }
	for i, r := range rt.Rules {
		if !r.Enabled {
			continue
		}
		n, err := normalizeRule(r)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条规则: %w", i+1, err)
		}
		line := n.Type + "," + n.Value + "," + target(n.Action)
		if n.Type == "GEOIP" || n.Type == "IP-CIDR" || n.Type == "IP-CIDR6" {
			line += ",no-resolve"
		}
		add(line)
	}
	if rt.Mode == "direct" {
		add("MATCH,DIRECT")
		return rules, nil
	}
	if rt.BlockAds {
		add("GEOSITE,category-ads-all,REJECT")
	}
	if rt.BypassLAN {
		add("DOMAIN-SUFFIX,local,DIRECT", "DOMAIN-SUFFIX,lan,DIRECT")
		for _, c := range privateCIDRs {
			t := "IP-CIDR"
			if strings.Contains(c, ":") {
				t = "IP-CIDR6"
			}
			add(t + "," + c + ",DIRECT,no-resolve")
		}
	}
	switch rt.Mode {
	case "bypass_cn":
		if rt.AppleCNDirect {
			add("GEOSITE,apple-cn,DIRECT", "GEOSITE,microsoft@cn,DIRECT")
		}
		add("GEOSITE,geolocation-!cn,"+ProxyGroup, "GEOSITE,cn,DIRECT", "GEOIP,CN,DIRECT", "MATCH,"+ProxyGroup)
	case "gfw":
		add("GEOSITE,gfw,"+ProxyGroup, "GEOIP,telegram,"+ProxyGroup+",no-resolve", "MATCH,DIRECT")
	case "global":
		add("MATCH," + ProxyGroup)
	default:
		return nil, fmt.Errorf("未知分流模式 %q", rt.Mode)
	}
	return rules, nil
}

func checkDNS(list []string, what string) ([]string, error) {
	var out []string
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !dnsRe.MatchString(s) {
			return nil, fmt.Errorf("%s无效: %q", what, s)
		}
		if strings.Contains(s, "://") {
			if u, err := url.Parse(s); err != nil || u.Host == "" {
				return nil, fmt.Errorf("%s无效: %q", what, s)
			}
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s不能为空", what)
	}
	return out, nil
}

func viaProxy(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		if strings.Contains(s, "#") {
			out[i] = s
		} else {
			out[i] = s + "#" + ProxyGroup
		}
	}
	return out
}

// proxyMaps renders one mihomo mieru outbound per port binding.
func proxyMaps(p Profile) ([]map[string]any, []string) {
	var proxies []map[string]any
	var names []string
	for _, b := range p.Bindings {
		port := strconv.Itoa(b.Port)
		if b.PortRange != "" {
			port = b.PortRange
		}
		name := fmt.Sprintf("%s %s:%s", p.Name, b.Protocol, port)
		m := map[string]any{
			"name": name, "type": "mieru", "server": p.Server,
			"transport": b.Protocol, "username": p.Username, "password": p.Password,
			"udp": true,
		}
		if b.PortRange != "" {
			m["port-range"] = b.PortRange
		} else {
			m["port"] = b.Port
		}
		if p.Multiplexing != "" {
			m["multiplexing"] = p.Multiplexing
		}
		if p.HandshakeMode != "" {
			m["handshake-mode"] = p.HandshakeMode
		}
		proxies = append(proxies, m)
		names = append(names, name)
	}
	return proxies, names
}

// buildConfig produces a mihomo config (JSON, which mihomo's YAML parser accepts).
func buildConfig(o Options) ([]byte, error) {
	if err := validateProfile(&o.Profile); err != nil {
		return nil, err
	}
	rt := o.Routing
	if rt.Mode == "" {
		rt.Mode = "bypass_cn"
	}
	rules, err := buildRules(rt)
	if err != nil {
		return nil, err
	}
	direct, err := checkDNS(rt.DirectDNS, "直连 DNS ")
	if err != nil {
		return nil, err
	}
	remote, err := checkDNS(rt.RemoteDNS, "远程 DNS ")
	if err != nil {
		return nil, err
	}
	stack := o.Stack
	switch stack {
	case "":
		stack = "gvisor"
	case "gvisor", "system", "mixed":
	default:
		return nil, fmt.Errorf("未知网络栈 %q", stack)
	}
	logLevel := o.LogLevel
	switch logLevel {
	case "":
		logLevel = "info"
	case "debug", "info", "warning", "error", "silent":
	default:
		return nil, fmt.Errorf("未知日志级别 %q", logLevel)
	}
	geox := geoxSources[o.GeoxSource]
	if geox == nil {
		geox = geoxSources["jsdelivr"]
	}

	proxies, names := proxyMaps(o.Profile)
	group := map[string]any{"name": ProxyGroup, "type": "select", "proxies": names}
	if len(names) > 1 {
		group = map[string]any{
			"name": ProxyGroup, "type": "fallback", "proxies": names,
			"url": "https://www.gstatic.com/generate_204", "interval": 300, "lazy": true,
		}
	}

	dns := map[string]any{
		"enable":                  true,
		"ipv6":                    o.IPv6,
		"default-nameserver":      []string{"223.5.5.5", "119.29.29.29"},
		"proxy-server-nameserver": direct,
		"direct-nameserver":       direct,
	}
	if rt.FakeIP {
		dns["enhanced-mode"] = "fake-ip"
		dns["fake-ip-range"] = fakeIPRange
		dns["fake-ip-filter"] = fakeIPFilter
	} else {
		dns["enhanced-mode"] = "redir-host"
	}
	switch rt.Mode {
	case "global":
		dns["nameserver"] = viaProxy(remote)
	case "direct":
		dns["nameserver"] = direct
	default:
		dns["nameserver"] = direct
		dns["fallback"] = viaProxy(remote)
		dns["fallback-filter"] = map[string]any{
			"geoip": true, "geoip-code": "CN", "geosite": []string{"gfw"},
			"ipcidr": []string{"240.0.0.0/4", "0.0.0.0/32"},
		}
	}

	cfg := map[string]any{
		"mode":                "rule",
		"log-level":           logLevel,
		"ipv6":                o.IPv6,
		"allow-lan":           false,
		"unified-delay":       true,
		"tcp-concurrent":      true,
		"find-process-mode":   "off",
		"geodata-mode":        true,
		"geodata-loader":      "memconservative",
		"geo-auto-update":     true,
		"geo-update-interval": 24,
		"geox-url":            geox,
		"profile":             map[string]any{"store-selected": false, "store-fake-ip": false},
		"dns":                 dns,
		"proxies":             proxies,
		"proxy-groups":        []any{group},
		"rules":               rules,
	}
	if o.MixedPort < 0 || o.MixedPort > 65535 {
		return nil, fmt.Errorf("本地代理端口无效 %d", o.MixedPort)
	}
	if r := o.Router; r != nil {
		// OpenWrt: LAN traffic arrives via nftables TPROXY, DNS via dnsmasq. Listeners
		// must accept LAN sources (allow-lan); the WAN side is closed by the firewall.
		for _, p := range []struct {
			v    int
			name string
		}{{r.TProxyPort, "TPROXY 端口"}, {r.DNSPort, "DNS 端口"}} {
			if p.v < 1 || p.v > 65535 {
				return nil, fmt.Errorf("%s无效 %d", p.name, p.v)
			}
		}
		if r.TProxyPort == r.DNSPort || r.TProxyPort == o.MixedPort || r.DNSPort == o.MixedPort {
			return nil, errors.New("TPROXY / DNS / 代理端口不能相同")
		}
		if r.RoutingMark < 0 {
			return nil, fmt.Errorf("routing-mark 无效 %d", r.RoutingMark)
		}
		cfg["allow-lan"] = true
		cfg["bind-address"] = "*"
		cfg["tproxy-port"] = r.TProxyPort
		if r.RoutingMark > 0 {
			cfg["routing-mark"] = r.RoutingMark
		}
		dns["listen"] = fmt.Sprintf("127.0.0.1:%d", r.DNSPort)
		if o.MixedPort > 0 {
			cfg["mixed-port"] = o.MixedPort
		}
	} else {
		tun := map[string]any{
			"enable":                true,
			"stack":                 stack,
			"file-descriptor":       o.TunFd,
			"dns-hijack":            []string{"any:53", "tcp://any:53"},
			"auto-route":            false,
			"auto-detect-interface": false,
			"mtu":                   tunMTU,
		}
		if o.IPv6 {
			tun["inet6-address"] = []string{TunAddress6}
		}
		cfg["tun"] = tun
		if o.MixedPort > 0 {
			cfg["mixed-port"] = o.MixedPort
			cfg["bind-address"] = "127.0.0.1"
		}
	}
	if rt.Sniff {
		cfg["sniffer"] = map[string]any{
			"enable": true, "parse-pure-ip": true,
			"sniff": map[string]any{
				"HTTP": map[string]any{"ports": []any{80, "8080-8880"}, "override-destination": true},
				"TLS":  map[string]any{"ports": []any{443, 8443}},
				"QUIC": map[string]any{"ports": []any{443, 8443}},
			},
		}
	}
	if len(proxies) == 0 {
		return nil, errors.New("节点没有端口")
	}
	return json.MarshalIndent(cfg, "", "  ")
}

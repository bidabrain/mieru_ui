package main

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bidabrain/mieru_ui/android/core/libcore"
)

const (
	configPath = "/etc/config/mieru"
	dataDir    = "/etc/mieru"
	shareDir   = "/usr/share/mieru"
)

var ifnameRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,15}$`)

// settings is everything the router side needs besides the core options.
type settings struct {
	Enabled    bool
	Core       libcore.Options
	NodeName   string
	LanIfnames []string
	AccessMode string // all | include | exclude
	AccessIPs  []netip.Prefix
	DNSHijack  bool
	Fwmark     int
	Table      int
	TestURL    string
}

func loadSettings(path string) (*settings, []*uciSection, error) {
	secs, err := loadSections(path)
	if err != nil {
		return nil, nil, err
	}
	s, err := buildSettings(secs)
	return s, secs, err
}

// loadSections parses a UCI file and gives anonymous sections the IDs uci itself uses
// (cfgXXXXXX). Those IDs are not stored in the file, but LuCI and `uci add` reference
// them (e.g. mieru.main.node), so they are taken from `uci -X show`.
func loadSections(path string) ([]*uciSection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	secs, err := parseUCI(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if out, err := exec.Command("uci", "-q", "-c", filepath.Dir(path), "-X", "show", filepath.Base(path)).Output(); err == nil {
		nameAnonymous(secs, filepath.Base(path), string(out))
	}
	return secs, nil
}

// nameAnonymous applies the section names from `uci -X show <pkg>` output, which lists
// sections in file order as lines "<pkg>.<name>=<type>".
func nameAnonymous(secs []*uciSection, pkg, show string) {
	type head struct{ name, typ string }
	var heads []head
	for _, line := range strings.Split(show, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.HasPrefix(k, pkg+".") || strings.Count(k, ".") != 1 {
			continue
		}
		heads = append(heads, head{strings.TrimPrefix(k, pkg+"."), v})
	}
	if len(heads) != len(secs) {
		return
	}
	for i, h := range heads {
		if h.typ != secs[i].Type {
			return
		}
	}
	for i, h := range heads {
		secs[i].Name = h.name
	}
}

func findSection(secs []*uciSection, typ, name string) *uciSection {
	for _, s := range secs {
		if s.Type == typ && (name == "" || s.Name == name) {
			return s
		}
	}
	return nil
}

func atoi(s *uciSection, key string, def int) (int, error) {
	v := strings.TrimSpace(s.get(key, ""))
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s 不是数字: %q", key, v)
	}
	return int(n), nil
}

// nodeProfile converts a `config node` section to a libcore profile.
func nodeProfile(n *uciSection) (libcore.Profile, error) {
	p := libcore.Profile{
		ID:            n.Name,
		Name:          n.get("name", ""),
		Server:        strings.TrimSpace(n.get("server", "")),
		Username:      n.get("username", ""),
		Password:      n.get("password", ""),
		Multiplexing:  n.get("multiplexing", ""),
		HandshakeMode: n.get("handshake_mode", ""),
	}
	if p.Name == "" {
		p.Name = p.Server
	}
	mtu, err := atoi(n, "mtu", 0)
	if err != nil {
		return p, err
	}
	p.MTU = mtu
	for _, spec := range n.Lists["port"] {
		b, err := parseBinding(spec)
		if err != nil {
			return p, fmt.Errorf("节点 %s: %w", p.Name, err)
		}
		p.Bindings = append(p.Bindings, b)
	}
	return p, nil
}

// parseBinding reads "2012/TCP" or "3000-3010/UDP" (protocol defaults to TCP).
func parseBinding(spec string) (libcore.Binding, error) {
	port, proto, _ := strings.Cut(strings.TrimSpace(spec), "/")
	proto = strings.ToUpper(strings.TrimSpace(proto))
	if proto == "" {
		proto = "TCP"
	}
	b := libcore.Binding{Protocol: proto}
	if strings.Contains(port, "-") {
		b.PortRange = port
	} else {
		n, err := strconv.Atoi(port)
		if err != nil {
			return b, fmt.Errorf("端口无效 %q", spec)
		}
		b.Port = n
	}
	return b, nil
}

func bindingSpec(b libcore.Binding) string {
	if b.PortRange != "" {
		return b.PortRange + "/" + b.Protocol
	}
	return strconv.Itoa(b.Port) + "/" + b.Protocol
}

// parseRule reads "TYPE,VALUE,ACTION" (action PROXY / DIRECT / REJECT).
func parseRule(line string) (libcore.Rule, error) {
	parts := strings.Split(line, ",")
	if len(parts) != 3 {
		return libcore.Rule{}, fmt.Errorf("规则格式应为 类型,内容,动作: %q", line)
	}
	return libcore.Rule{Type: parts[0], Value: parts[1], Action: parts[2], Enabled: true}, nil
}

func buildSettings(secs []*uciSection) (*settings, error) {
	m := findSection(secs, "mieru", "main")
	if m == nil {
		m = findSection(secs, "mieru", "")
	}
	if m == nil {
		return nil, errors.New("缺少 config mieru 'main'")
	}
	s := &settings{Enabled: m.bool("enabled", false)}

	nodeName := m.get("node", "")
	if nodeName == "" {
		return s, errors.New("还没有选择节点")
	}
	n := findSection(secs, "node", nodeName)
	if n == nil {
		return s, fmt.Errorf("所选节点 %q 不存在", nodeName)
	}
	p, err := nodeProfile(n)
	if err != nil {
		return s, err
	}
	s.NodeName = p.Name

	rt := libcore.DefaultRouting()
	rt.Mode = m.get("mode", rt.Mode)
	rt.BypassLAN = true
	rt.BlockAds = m.bool("block_ads", rt.BlockAds)
	rt.AppleCNDirect = m.bool("apple_cn_direct", rt.AppleCNDirect)
	rt.FakeIP = m.bool("fake_ip", rt.FakeIP)
	rt.Sniff = m.bool("sniff", rt.Sniff)
	if l := m.Lists["direct_dns"]; len(l) > 0 {
		rt.DirectDNS = l
	}
	if l := m.Lists["remote_dns"]; len(l) > 0 {
		rt.RemoteDNS = l
	}
	for i, line := range m.Lists["rule"] {
		r, err := parseRule(line)
		if err != nil {
			return s, fmt.Errorf("第 %d 条规则: %w", i+1, err)
		}
		rt.Rules = append(rt.Rules, r)
	}

	r := &libcore.Router{}
	if r.TProxyPort, err = atoi(m, "tproxy_port", 7893); err != nil {
		return s, err
	}
	if r.DNSPort, err = atoi(m, "dns_port", 7874); err != nil {
		return s, err
	}
	if r.RoutingMark, err = atoi(m, "routing_mark", 0); err != nil {
		return s, err
	}
	mixed, err := atoi(m, "mixed_port", 0)
	if err != nil {
		return s, err
	}
	s.Core = libcore.Options{
		Profile: p, Routing: rt, Router: r, MixedPort: mixed,
		LogLevel:   m.get("log_level", "info"),
		GeoxSource: m.get("geox_source", "jsdelivr"),
	}

	s.LanIfnames = m.Lists["lan_ifname"]
	if len(s.LanIfnames) == 0 {
		s.LanIfnames = []string{"br-lan"}
	}
	for _, ifn := range s.LanIfnames {
		if !ifnameRe.MatchString(ifn) {
			return s, fmt.Errorf("接口名无效 %q", ifn)
		}
	}
	s.AccessMode = m.get("access_mode", "all")
	switch s.AccessMode {
	case "all", "include", "exclude":
	default:
		return s, fmt.Errorf("访问控制模式无效 %q", s.AccessMode)
	}
	for _, v := range m.Lists["access_ip"] {
		pfx, err := parsePrefix4(v)
		if err != nil {
			return s, err
		}
		s.AccessIPs = append(s.AccessIPs, pfx)
	}
	if s.AccessMode == "include" && len(s.AccessIPs) == 0 {
		return s, errors.New("访问控制为“仅代理列表中的设备”，但列表为空")
	}
	s.DNSHijack = m.bool("dns_hijack", true)
	if s.Fwmark, err = atoi(m, "fwmark", 0x162); err != nil {
		return s, err
	}
	if s.Table, err = atoi(m, "route_table", 162); err != nil {
		return s, err
	}
	if s.Fwmark <= 0 || s.Fwmark == r.RoutingMark || s.Table < 1 || s.Table > 252 {
		return s, errors.New("fwmark / 路由表设置无效")
	}
	s.TestURL = m.get("test_url", "https://www.gstatic.com/generate_204")
	return s, nil
}

func parsePrefix4(v string) (netip.Prefix, error) {
	v = strings.TrimSpace(v)
	pfx, err := netip.ParsePrefix(v)
	if err != nil {
		a, err2 := netip.ParseAddr(v)
		if err2 != nil {
			return pfx, fmt.Errorf("IP 地址无效 %q", v)
		}
		pfx = netip.PrefixFrom(a, a.BitLen())
	}
	if !pfx.Addr().Is4() {
		return pfx, fmt.Errorf("目前只支持 IPv4 地址 %q", v)
	}
	return pfx.Masked(), nil
}

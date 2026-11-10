package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"mieru-ui/database"
)

// MihomoService is a geosite category users can force through the proxy.
type MihomoService struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MihomoServices is the catalogue shown in the UI.
var MihomoServices = []MihomoService{
	{"google", "Google"},
	{"youtube", "YouTube"},
	{"telegram", "Telegram"},
	{"twitter", "X / Twitter"},
	{"github", "GitHub"},
	{"category-ai-!cn", "AI 服务（ChatGPT / Claude / Gemini 等）"},
	{"netflix", "Netflix"},
	{"spotify", "Spotify"},
	{"facebook", "Facebook"},
	{"instagram", "Instagram"},
	{"discord", "Discord"},
}

const (
	autoGroupName     = "♻️ 自动选择"
	fallbackGroupName = "🔯 故障转移"
)

var (
	domainRe     = regexp.MustCompile(`^[A-Za-z0-9*+_.-]{1,253}$`)
	fakeFilterRe = regexp.MustCompile(`^[A-Za-z0-9*+_.:-]{1,253}$`)
	dnsServerRe  = regexp.MustCompile(`^[A-Za-z0-9.:/\[\]_#@%?=&+-]{1,200}$`)
	controllerRe = regexp.MustCompile(`^[A-Za-z0-9.\[\]:-]{1,100}:\d{1,5}$`)
	ruleValueRe  = regexp.MustCompile(`^[^,\s]{1,253}$`)
	ruleTypes    = set("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOSITE", "GEOIP", "IP-CIDR", "IP-CIDR6",
		"SRC-IP-CIDR", "IP-ASN", "DST-PORT", "SRC-PORT", "PROCESS-NAME")
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// cleanList trims entries, drops blanks and validates each one.
func cleanList(name string, in []string, re *regexp.Regexp, max int) ([]string, error) {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !re.MatchString(v) {
			return nil, fmt.Errorf("%s 中的条目无效: %q", name, v)
		}
		out = append(out, v)
	}
	if len(out) > max {
		return nil, fmt.Errorf("%s 最多 %d 条", name, max)
	}
	return out, nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// parseCustomRule validates "TYPE,VALUE,TARGET[,no-resolve]" and maps PROXY to the group name.
func parseCustomRule(line, group string) (string, error) {
	parts := strings.Split(line, ",")
	if len(parts) != 3 && len(parts) != 4 {
		return "", fmt.Errorf("自定义规则格式应为 类型,值,DIRECT|REJECT|PROXY[,no-resolve]: %q", line)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	typ := strings.ToUpper(parts[0])
	if !ruleTypes[typ] {
		return "", fmt.Errorf("不支持的规则类型 %q", parts[0])
	}
	if !ruleValueRe.MatchString(parts[1]) {
		return "", fmt.Errorf("规则值无效: %q", parts[1])
	}
	target := strings.ToUpper(parts[2])
	switch target {
	case "DIRECT", "REJECT":
	case "PROXY":
		target = group
	default:
		return "", fmt.Errorf("规则目标只能是 DIRECT、REJECT 或 PROXY: %q", parts[2])
	}
	out := typ + "," + parts[1] + "," + target
	if len(parts) == 4 {
		if strings.ToLower(parts[3]) != "no-resolve" {
			return "", fmt.Errorf("规则第 4 段只能是 no-resolve: %q", line)
		}
		out += ",no-resolve"
	}
	return out, nil
}

// ValidateMihomo normalizes and validates a template in place.
func ValidateMihomo(c *database.MihomoConf) error {
	if c.MixedPort < 1 || c.MixedPort > 65535 {
		return errors.New("混合端口无效")
	}
	if !set("rule", "global", "direct")[c.Mode] {
		return errors.New("运行模式无效")
	}
	if !set("silent", "error", "warning", "info", "debug")[c.LogLevel] {
		return errors.New("日志级别无效")
	}
	c.ExternalController = strings.TrimSpace(c.ExternalController)
	if c.ExternalController != "" && !controllerRe.MatchString(c.ExternalController) {
		return errors.New("外部控制器地址格式应为 IP:端口")
	}
	if len(c.Secret) > 64 || hasControl(c.Secret) {
		return errors.New("控制器密钥过长或包含非法字符")
	}
	if !set("bypass_cn", "gfw", "global")[c.RouteMode] {
		return errors.New("分流模式无效")
	}
	known := map[string]bool{}
	for _, s := range MihomoServices {
		known[s.ID] = true
	}
	svcs := []string{}
	for _, s := range c.ProxyServices {
		if !known[s] {
			return fmt.Errorf("未知服务 %q", s)
		}
		svcs = append(svcs, s)
	}
	c.ProxyServices = svcs

	var err error
	if c.DirectDomains, err = cleanList("直连域名", c.DirectDomains, domainRe, 1000); err != nil {
		return err
	}
	if c.ProxyDomains, err = cleanList("代理域名", c.ProxyDomains, domainRe, 1000); err != nil {
		return err
	}
	if c.RejectDomains, err = cleanList("拦截域名", c.RejectDomains, domainRe, 1000); err != nil {
		return err
	}

	c.GroupName = strings.TrimSpace(c.GroupName)
	if c.GroupName == "" || len([]rune(c.GroupName)) > 40 || strings.Contains(c.GroupName, ",") || hasControl(c.GroupName) {
		return errors.New("代理组名称不能为空、不能超过 40 字、不能包含逗号")
	}
	if set("DIRECT", "REJECT", "GLOBAL", "PROXY", autoGroupName, fallbackGroupName)[strings.ToUpper(c.GroupName)] ||
		c.GroupName == autoGroupName || c.GroupName == fallbackGroupName {
		return errors.New("代理组名称与保留名称冲突")
	}
	rules := []string{}
	for _, line := range c.CustomRules {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := parseCustomRule(line, c.GroupName); err != nil {
			return err
		}
		rules = append(rules, line)
	}
	if len(rules) > 1000 {
		return errors.New("自定义规则最多 1000 条")
	}
	c.CustomRules = rules

	if !set("default", "github", "jsdelivr")[c.GeoxSource] {
		return errors.New("规则数据下载源无效")
	}
	if c.GeoUpdateHours < 1 || c.GeoUpdateHours > 720 {
		return errors.New("规则数据更新间隔需在 1-720 小时之间")
	}
	if u, err := url.Parse(c.TestURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("测速地址必须是 http(s) 链接")
	}
	if c.TestIntervalSecs < 30 || c.TestIntervalSecs > 86400 {
		return errors.New("测速间隔需在 30-86400 秒之间")
	}

	if !set("fake-ip", "redir-host")[c.EnhancedMode] {
		return errors.New("DNS 模式无效")
	}
	if _, _, err := net.ParseCIDR(c.FakeIPRange); err != nil && c.EnhancedMode == "fake-ip" {
		return errors.New("fake-ip 地址段格式无效")
	}
	if c.DefaultNameservers, err = cleanList("默认 DNS", c.DefaultNameservers, dnsServerRe, 20); err != nil {
		return err
	}
	if c.Nameservers, err = cleanList("主 DNS", c.Nameservers, dnsServerRe, 20); err != nil {
		return err
	}
	if c.Fallback, err = cleanList("备用 DNS", c.Fallback, dnsServerRe, 20); err != nil {
		return err
	}
	if c.FakeIPFilter, err = cleanList("fake-ip 过滤", c.FakeIPFilter, fakeFilterRe, 500); err != nil {
		return err
	}
	if c.DNSEnable && len(c.Nameservers) == 0 {
		return errors.New("启用 DNS 时至少需要一个主 DNS")
	}
	return nil
}

// ---------- YAML document ----------

type mProxy struct {
	Name          string `yaml:"name"`
	Type          string `yaml:"type"`
	Server        string `yaml:"server"`
	Port          int    `yaml:"port,omitempty"`
	PortRange     string `yaml:"port-range,omitempty"`
	Transport     string `yaml:"transport"`
	UDP           bool   `yaml:"udp"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	Multiplexing  string `yaml:"multiplexing,omitempty"`
	HandshakeMode string `yaml:"handshake-mode,omitempty"`
}

type mGroup struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Proxies  []string `yaml:"proxies"`
	URL      string   `yaml:"url,omitempty"`
	Interval int      `yaml:"interval,omitempty"`
}

type mFallbackFilter struct {
	GeoIP     bool     `yaml:"geoip"`
	GeoIPCode string   `yaml:"geoip-code"`
	IPCIDR    []string `yaml:"ipcidr"`
}

type mDNS struct {
	Enable                bool             `yaml:"enable"`
	IPv6                  bool             `yaml:"ipv6"`
	EnhancedMode          string           `yaml:"enhanced-mode"`
	FakeIPRange           string           `yaml:"fake-ip-range,omitempty"`
	FakeIPFilter          []string         `yaml:"fake-ip-filter,omitempty"`
	DefaultNameserver     []string         `yaml:"default-nameserver,omitempty"`
	Nameserver            []string         `yaml:"nameserver"`
	ProxyServerNameserver []string         `yaml:"proxy-server-nameserver,omitempty"`
	Fallback              []string         `yaml:"fallback,omitempty"`
	FallbackFilter        *mFallbackFilter `yaml:"fallback-filter,omitempty"`
}

type mGeoX struct {
	GeoIP   string `yaml:"geoip"`
	GeoSite string `yaml:"geosite"`
	MMDB    string `yaml:"mmdb"`
}

type mDoc struct {
	MixedPort          int      `yaml:"mixed-port"`
	AllowLan           bool     `yaml:"allow-lan"`
	BindAddress        string   `yaml:"bind-address"`
	Mode               string   `yaml:"mode"`
	LogLevel           string   `yaml:"log-level"`
	IPv6               bool     `yaml:"ipv6"`
	ExternalController string   `yaml:"external-controller,omitempty"`
	Secret             string   `yaml:"secret,omitempty"`
	UnifiedDelay       bool     `yaml:"unified-delay"`
	TCPConcurrent      bool     `yaml:"tcp-concurrent"`
	GeodataMode        bool     `yaml:"geodata-mode"`
	GeoAutoUpdate      bool     `yaml:"geo-auto-update"`
	GeoUpdateInterval  int      `yaml:"geo-update-interval"`
	GeoxURL            *mGeoX   `yaml:"geox-url,omitempty"`
	DNS                *mDNS    `yaml:"dns,omitempty"`
	Proxies            []mProxy `yaml:"proxies"`
	ProxyGroups        []mGroup `yaml:"proxy-groups"`
	Rules              []string `yaml:"rules"`
}

func geox(source string) *mGeoX {
	var base string
	switch source {
	case "github":
		base = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/"
	case "jsdelivr":
		base = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/"
	default:
		return nil
	}
	return &mGeoX{GeoIP: base + "geoip.dat", GeoSite: base + "geosite.dat", MMDB: base + "country.mmdb"}
}

// MihomoProxies returns one mieru proxy entry per port binding.
func MihomoProxies(u database.User, host string, sc database.ServerConf) []mProxy {
	var out []mProxy
	for _, pb := range sc.PortBindings {
		label := strconv.Itoa(pb.Port)
		if pb.PortRange != "" {
			label = pb.PortRange
		}
		p := mProxy{
			Name:          fmt.Sprintf("mieru-%s-%s-%s", u.Name, label, pb.Protocol),
			Type:          "mieru",
			Server:        host,
			Transport:     pb.Protocol,
			UDP:           true,
			Username:      u.Name,
			Password:      u.Password,
			Multiplexing:  sc.Multiplexing,
			HandshakeMode: sc.HandshakeMode,
		}
		if pb.PortRange != "" {
			p.PortRange = pb.PortRange
		} else {
			p.Port = pb.Port
		}
		out = append(out, p)
	}
	return out
}

// MihomoRules builds the rule list for a template.
func MihomoRules(c database.MihomoConf) []string {
	g := c.GroupName
	rules := []string{}
	for _, line := range c.CustomRules {
		if r, err := parseCustomRule(line, g); err == nil {
			rules = append(rules, r)
		}
	}
	for _, d := range c.RejectDomains {
		rules = append(rules, "DOMAIN-SUFFIX,"+d+",REJECT")
	}
	for _, d := range c.DirectDomains {
		rules = append(rules, "DOMAIN-SUFFIX,"+d+",DIRECT")
	}
	for _, d := range c.ProxyDomains {
		rules = append(rules, "DOMAIN-SUFFIX,"+d+","+g)
	}
	if c.PrivateDirect {
		rules = append(rules, "GEOSITE,private,DIRECT", "GEOIP,private,DIRECT,no-resolve")
	}
	if c.BlockAds {
		rules = append(rules, "GEOSITE,category-ads-all,REJECT")
	}
	for _, s := range c.ProxyServices {
		rules = append(rules, "GEOSITE,"+s+","+g)
		if s == "telegram" {
			rules = append(rules, "GEOIP,telegram,"+g+",no-resolve")
		}
	}
	if c.AppleCNDirect {
		rules = append(rules, "GEOSITE,apple-cn,DIRECT")
	}
	if c.MSCNDirect {
		rules = append(rules, "GEOSITE,microsoft@cn,DIRECT")
	}
	switch c.RouteMode {
	case "bypass_cn":
		rules = append(rules, "GEOSITE,cn,DIRECT", "GEOIP,CN,DIRECT", "MATCH,"+g)
	case "gfw":
		rules = append(rules, "GEOSITE,gfw,"+g, "MATCH,DIRECT")
	default: // global
		rules = append(rules, "MATCH,"+g)
	}
	return rules
}

// BuildMihomo renders a complete mihomo config for one user.
func BuildMihomo(u database.User, host string, sc database.ServerConf, c database.MihomoConf) (string, error) {
	proxies := MihomoProxies(u, host, sc)
	if len(proxies) == 0 {
		return "", errors.New("服务端没有配置任何端口")
	}
	names := make([]string, len(proxies))
	for i, p := range proxies {
		names[i] = p.Name
	}

	main := mGroup{Name: c.GroupName, Type: "select"}
	var extra []mGroup
	if c.AutoGroup {
		main.Proxies = append(main.Proxies, autoGroupName)
		extra = append(extra, mGroup{Name: autoGroupName, Type: "url-test", Proxies: names, URL: c.TestURL, Interval: c.TestIntervalSecs})
	}
	if c.FallbackGroup {
		main.Proxies = append(main.Proxies, fallbackGroupName)
		extra = append(extra, mGroup{Name: fallbackGroupName, Type: "fallback", Proxies: names, URL: c.TestURL, Interval: c.TestIntervalSecs})
	}
	main.Proxies = append(main.Proxies, names...)
	main.Proxies = append(main.Proxies, "DIRECT")

	doc := mDoc{
		MixedPort:          c.MixedPort,
		AllowLan:           c.AllowLan,
		BindAddress:        "*",
		Mode:               c.Mode,
		LogLevel:           c.LogLevel,
		IPv6:               c.IPv6,
		ExternalController: c.ExternalController,
		Secret:             c.Secret,
		UnifiedDelay:       c.UnifiedDelay,
		TCPConcurrent:      c.TCPConcurrent,
		GeodataMode:        true,
		GeoAutoUpdate:      c.GeoAutoUpdate,
		GeoUpdateInterval:  c.GeoUpdateHours,
		GeoxURL:            geox(c.GeoxSource),
		Proxies:            proxies,
		ProxyGroups:        append([]mGroup{main}, extra...),
		Rules:              MihomoRules(c),
	}
	if c.DNSEnable {
		dns := &mDNS{
			Enable:                true,
			IPv6:                  c.IPv6,
			EnhancedMode:          c.EnhancedMode,
			DefaultNameserver:     c.DefaultNameservers,
			Nameserver:            c.Nameservers,
			ProxyServerNameserver: c.Nameservers,
			Fallback:              c.Fallback,
		}
		if c.EnhancedMode == "fake-ip" {
			dns.FakeIPRange = c.FakeIPRange
			dns.FakeIPFilter = c.FakeIPFilter
		}
		if len(c.Fallback) > 0 {
			dns.FallbackFilter = &mFallbackFilter{GeoIP: c.FallbackGeoIP, GeoIPCode: "CN", IPCIDR: []string{"240.0.0.0/4", "0.0.0.0/32"}}
		}
		doc.DNS = dns
	}

	out, err := encodeYAML(doc)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# mihomo (Clash.Meta) config generated by mieru-ui for user %s\n", u.Name) + out, nil
}

// encodeYAML marshals v with 2-space indentation. yaml.v3 escapes characters outside
// the BMP (e.g. emoji become "\U0001F680"), which is valid but unreadable, so such
// scalars are swapped for placeholders and re-inserted as JSON strings (a JSON string
// is a valid YAML double-quoted scalar and keeps the emoji literal).
func encodeYAML(v any) (string, error) {
	var root yaml.Node
	if err := root.Encode(v); err != nil {
		return "", err
	}
	nonce := database.RandomHex(6)
	repl := map[string]string{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && strings.IndexFunc(n.Value, func(r rune) bool { return r > 0xFFFF }) >= 0 {
			var q bytes.Buffer
			je := json.NewEncoder(&q)
			je.SetEscapeHTML(false)
			_ = je.Encode(n.Value)
			token := fmt.Sprintf("MIERUUI%sX%d", nonce, len(repl))
			repl[token] = strings.TrimRight(q.String(), "\n")
			n.Value, n.Style = token, 0
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return "", err
	}
	_ = enc.Close()
	out := buf.String()
	for token, quoted := range repl {
		out = strings.ReplaceAll(out, token, quoted)
	}
	return out, nil
}

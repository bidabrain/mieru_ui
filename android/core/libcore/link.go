package libcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	portRangeRe = regexp.MustCompile(`^(\d{1,5})-(\d{1,5})$`)
	muxLevels   = map[string]bool{"": true, "MULTIPLEXING_DEFAULT": true, "MULTIPLEXING_OFF": true, "MULTIPLEXING_LOW": true, "MULTIPLEXING_MIDDLE": true, "MULTIPLEXING_HIGH": true}
	handshakes  = map[string]bool{"": true, "HANDSHAKE_DEFAULT": true, "HANDSHAKE_STANDARD": true, "HANDSHAKE_NO_WAIT": true}
)

// parseImport accepts mierus:// links (one per line) or a mieru client JSON config.
func parseImport(text string) ([]Profile, error) {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\uFEFF"))
	if text == "" {
		return nil, errors.New("内容为空")
	}
	if strings.HasPrefix(text, "{") {
		return parseClientJSON(text)
	}
	var out []Profile
	var errs []string
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == ' ' || r == '\t' }) {
		if !strings.HasPrefix(strings.ToLower(line), "mierus://") {
			continue
		}
		p, err := parseMierusLink(line)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		if len(errs) > 0 {
			return nil, errors.New(errs[0])
		}
		return nil, errors.New("未找到 mierus:// 链接")
	}
	return out, nil
}

func parseMierusLink(link string) (Profile, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Profile{}, fmt.Errorf("链接格式错误: %v", err)
	}
	if !strings.EqualFold(u.Scheme, "mierus") {
		return Profile{}, errors.New("不是 mierus:// 链接")
	}
	if u.User == nil {
		return Profile{}, errors.New("链接缺少用户名")
	}
	p := Profile{Server: u.Hostname(), Username: u.User.Username()}
	p.Password, _ = u.User.Password()
	q := u.Query()
	p.Name = q.Get("profile")
	if p.Name == "" {
		p.Name = p.Username + "@" + p.Server
	}
	if v := q.Get("mtu"); v != "" {
		p.MTU, _ = strconv.Atoi(v)
	}
	p.Multiplexing = q.Get("multiplexing")
	p.HandshakeMode = q.Get("handshake-mode")
	ports, protos := q["port"], q["protocol"]
	if len(ports) != len(protos) {
		return Profile{}, errors.New("port 与 protocol 数量不一致")
	}
	for i, port := range ports {
		b := Binding{Protocol: strings.ToUpper(protos[i])}
		if strings.Contains(port, "-") {
			b.PortRange = port
		} else {
			b.Port, _ = strconv.Atoi(port)
		}
		p.Bindings = append(p.Bindings, b)
	}
	return p, validateProfile(&p)
}

type clientJSON struct {
	Profiles []struct {
		ProfileName string `json:"profileName"`
		User        struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		} `json:"user"`
		Servers []struct {
			IPAddress    string `json:"ipAddress"`
			DomainName   string `json:"domainName"`
			PortBindings []struct {
				Port      int    `json:"port"`
				PortRange string `json:"portRange"`
				Protocol  string `json:"protocol"`
			} `json:"portBindings"`
		} `json:"servers"`
		MTU          int `json:"mtu"`
		Multiplexing struct {
			Level string `json:"level"`
		} `json:"multiplexing"`
		HandshakeMode string `json:"handshakeMode"`
	} `json:"profiles"`
}

func parseClientJSON(text string) ([]Profile, error) {
	var c clientJSON
	if err := json.Unmarshal([]byte(text), &c); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %v", err)
	}
	var out []Profile
	for _, cp := range c.Profiles {
		for _, s := range cp.Servers {
			p := Profile{
				Name: cp.ProfileName, Username: cp.User.Name, Password: cp.User.Password,
				MTU: cp.MTU, Multiplexing: cp.Multiplexing.Level, HandshakeMode: cp.HandshakeMode,
				Server: s.DomainName,
			}
			if p.Server == "" {
				p.Server = s.IPAddress
			}
			for _, b := range s.PortBindings {
				p.Bindings = append(p.Bindings, Binding{Port: b.Port, PortRange: b.PortRange, Protocol: strings.ToUpper(b.Protocol)})
			}
			if len(cp.Servers) > 1 {
				p.Name = cp.ProfileName + " · " + p.Server
			}
			if err := validateProfile(&p); err != nil {
				return nil, err
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("JSON 中没有可用的 profile")
	}
	return out, nil
}

func validateProfile(p *Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Server = strings.Trim(strings.TrimSpace(p.Server), "[]")
	if p.Name == "" {
		p.Name = p.Server
	}
	if strings.ContainsAny(p.Name, "\r\n") || len(p.Name) > 100 {
		return errors.New("名称无效")
	}
	if p.Server == "" || strings.ContainsAny(p.Server, " /\\\r\n,") {
		return errors.New("服务器地址无效")
	}
	if p.Username == "" || p.Password == "" {
		return errors.New("用户名和密码不能为空")
	}
	if len(p.Bindings) == 0 {
		return errors.New("至少需要一个端口")
	}
	for i := range p.Bindings {
		b := &p.Bindings[i]
		b.Protocol = strings.ToUpper(b.Protocol)
		if b.Protocol != "TCP" && b.Protocol != "UDP" {
			return fmt.Errorf("不支持的协议 %q", b.Protocol)
		}
		if b.PortRange != "" {
			m := portRangeRe.FindStringSubmatch(b.PortRange)
			if m == nil {
				return fmt.Errorf("端口范围无效 %q", b.PortRange)
			}
			lo, _ := strconv.Atoi(m[1])
			hi, _ := strconv.Atoi(m[2])
			if lo < 1 || hi > 65535 || lo > hi {
				return fmt.Errorf("端口范围无效 %q", b.PortRange)
			}
			b.Port = 0
		} else if b.Port < 1 || b.Port > 65535 {
			return fmt.Errorf("端口无效 %d", b.Port)
		}
	}
	if p.MTU != 0 && (p.MTU < 1280 || p.MTU > 1500) {
		return errors.New("MTU 需在 1280-1500 之间")
	}
	if !muxLevels[p.Multiplexing] {
		return fmt.Errorf("多路复用等级无效 %q", p.Multiplexing)
	}
	if !handshakes[p.HandshakeMode] {
		return fmt.Errorf("握手模式无效 %q", p.HandshakeMode)
	}
	return nil
}

func profileLink(p Profile) string {
	q := url.Values{}
	q.Set("profile", p.Name)
	if p.MTU > 0 {
		q.Set("mtu", strconv.Itoa(p.MTU))
	}
	if p.Multiplexing != "" {
		q.Set("multiplexing", p.Multiplexing)
	}
	if p.HandshakeMode != "" {
		q.Set("handshake-mode", p.HandshakeMode)
	}
	for _, b := range p.Bindings {
		if b.PortRange != "" {
			q.Add("port", b.PortRange)
		} else {
			q.Add("port", strconv.Itoa(b.Port))
		}
		q.Add("protocol", b.Protocol)
	}
	host := p.Server
	if strings.Contains(host, ":") && net.ParseIP(host) != nil {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("mierus://%s@%s?%s", url.UserPassword(p.Username, p.Password).String(), host, q.Encode())
}

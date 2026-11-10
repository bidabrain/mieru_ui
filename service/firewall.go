package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FirewallRule is one rule, normalized across backends.
type FirewallRule struct {
	ID     string `json:"id"`     // backend-specific identifier used for deletion
	Text   string `json:"text"`   // human readable rule
	Action string `json:"action"` // allow / deny / reject / limit
	Port   string `json:"port"`   // "2012" or "3000-3010", empty if not a simple port rule
	Proto  string `json:"proto"`  // tcp / udp / both / ""
}

// PortCheck reports whether an important port is allowed through the firewall.
type PortCheck struct {
	Label   string `json:"label"`
	Port    string `json:"port"`
	Proto   string `json:"proto"`
	Allowed bool   `json:"allowed"`
}

// FirewallStatus is what the firewall page shows.
type FirewallStatus struct {
	Backend       string         `json:"backend"` // ufw | firewalld | iptables | none
	Active        bool           `json:"active"`
	Editable      bool           `json:"editable"`
	Defaults      string         `json:"defaults"`
	Rules         []FirewallRule `json:"rules"`
	Raw           string         `json:"raw"`
	SSHPorts      []int          `json:"sshPorts"`
	Checks        []PortCheck    `json:"checks"`
	CanInstallUFW bool           `json:"canInstallUFW"`
	Message       string         `json:"message"`
}

// RequiredPort is a port the panel wants open.
type RequiredPort struct {
	Label string
	Port  string // "2012" or "3000-3010"
	Proto string // tcp / udp
}

var fwRun = func(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), text)
		}
		return text, fmt.Errorf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return text, nil
}

func has(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// DetectFirewall picks the firewall backend in use.
func DetectFirewall() string {
	if runtime.GOOS != "linux" {
		return "none"
	}
	if has("firewall-cmd") {
		if out, _ := fwRun(5*time.Second, "firewall-cmd", "--state"); strings.TrimSpace(out) == "running" {
			return "firewalld"
		}
	}
	if has("ufw") {
		return "ufw"
	}
	if has("firewall-cmd") {
		return "firewalld"
	}
	if has("iptables") {
		return "iptables"
	}
	return "none"
}

// ---------- port spec helpers ----------

var portSpecRe = regexp.MustCompile(`^(\d{1,5})(?:[-:](\d{1,5}))?$`)

func parseSpan(port string) (lo, hi int, ok bool) {
	m := portSpecRe.FindStringSubmatch(port)
	if m == nil {
		return 0, 0, false
	}
	lo, _ = strconv.Atoi(m[1])
	hi = lo
	if m[2] != "" {
		hi, _ = strconv.Atoi(m[2])
	}
	if lo < 1 || hi > 65535 || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// ValidatePortInput normalizes user input to "N" or "A-B" and checks the protocol.
func ValidatePortInput(port, proto string) (string, string, error) {
	port = strings.ReplaceAll(strings.TrimSpace(port), ":", "-")
	lo, hi, ok := parseSpan(port)
	if !ok {
		return "", "", errors.New("端口格式错误，应为 1-65535 的单个端口或范围（如 3000-3010）")
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto != "tcp" && proto != "udp" && proto != "both" {
		return "", "", errors.New("协议必须是 tcp、udp 或 both")
	}
	// "both" is always expanded into separate tcp and udp rules (ufw requires a protocol for ranges).
	if lo == hi {
		return strconv.Itoa(lo), proto, nil
	}
	return fmt.Sprintf("%d-%d", lo, hi), proto, nil
}

func protos(p string) []string {
	if p == "both" || p == "" {
		return []string{"tcp", "udp"}
	}
	return []string{p}
}

// covered reports whether an allow rule list covers port/proto.
func covered(rules []FirewallRule, port, proto string, sshPorts []int) bool {
	lo, hi, ok := parseSpan(port)
	if !ok {
		return false
	}
	for _, r := range rules {
		if r.Action != "allow" && r.Action != "limit" {
			continue
		}
		if r.Proto != "both" && r.Proto != "" && r.Proto != proto {
			continue
		}
		rlo, rhi, ok := parseSpan(r.Port)
		if !ok {
			continue
		}
		if rlo <= lo && hi <= rhi {
			return true
		}
	}
	return false
}

// ---------- ufw ----------

var ufwPortRe = regexp.MustCompile(`^(\d{1,5}(?::\d{1,5})?)(?:/(tcp|udp))?$`)

// parseUFWAdded parses `ufw show added`.
func parseUFWAdded(out string, sshPorts []int) []FirewallRule {
	rules := []FirewallRule{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ufw ") {
			continue
		}
		spec := strings.TrimPrefix(line, "ufw ")
		r := FirewallRule{ID: spec, Text: spec}
		f := strings.Fields(spec)
		if len(f) == 0 {
			continue
		}
		r.Action = f[0]
		rest := f[1:]
		if len(rest) > 0 && (rest[0] == "in" || rest[0] == "out") {
			if rest[0] == "out" {
				r.Action = "out-" + r.Action
			}
			rest = rest[1:]
		}
		if len(rest) == 1 {
			if m := ufwPortRe.FindStringSubmatch(rest[0]); m != nil {
				r.Port = strings.ReplaceAll(m[1], ":", "-")
				r.Proto = m[2]
				if r.Proto == "" {
					r.Proto = "both"
				}
			} else if strings.EqualFold(rest[0], "OpenSSH") || strings.EqualFold(rest[0], "ssh") {
				// Application profile: covers sshd's ports.
				for _, p := range sshPorts {
					rules = append(rules, FirewallRule{ID: spec, Text: spec, Action: r.Action, Port: strconv.Itoa(p), Proto: "tcp"})
				}
				continue
			}
		}
		rules = append(rules, r)
	}
	return rules
}

func ufwStatus(st *FirewallStatus) {
	st.Editable = true
	out, err := fwRun(10*time.Second, "ufw", "status", "verbose")
	st.Raw = out
	if err != nil {
		st.Message = err.Error()
		return
	}
	st.Active = strings.Contains(out, "Status: active")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Default:") {
			st.Defaults = strings.TrimSpace(strings.TrimPrefix(line, "Default:"))
		}
	}
	added, err := fwRun(10*time.Second, "ufw", "show", "added")
	if err != nil {
		st.Message = err.Error()
		return
	}
	st.Rules = parseUFWAdded(added, st.SSHPorts)
}

func ufwAllow(port, proto string) error {
	ufwPort := strings.ReplaceAll(port, "-", ":")
	for _, p := range protos(proto) {
		if _, err := fwRun(15*time.Second, "ufw", "allow", ufwPort+"/"+p); err != nil {
			return err
		}
	}
	return nil
}

// ---------- firewalld ----------

func firewalldStatus(st *FirewallStatus) {
	st.Editable = true
	state, _ := fwRun(5*time.Second, "firewall-cmd", "--state")
	st.Active = strings.TrimSpace(state) == "running"
	if !st.Active {
		st.Raw = state
		st.Message = "firewalld 未运行，规则列表需要服务运行时才能读取"
		return
	}
	all, err := fwRun(10*time.Second, "firewall-cmd", "--list-all")
	st.Raw = all
	if err != nil {
		st.Message = err.Error()
		return
	}
	st.Rules = parseFirewalld(all, st.SSHPorts)
	for _, line := range strings.Split(all, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "target:") {
			st.Defaults = "zone target: " + strings.TrimSpace(strings.TrimPrefix(line, "target:"))
		}
	}
}

// parseFirewalld parses `firewall-cmd --list-all`.
func parseFirewalld(out string, sshPorts []int) []FirewallRule {
	rules := []FirewallRule{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "services:"):
			for _, svc := range strings.Fields(strings.TrimPrefix(line, "services:")) {
				if svc == "ssh" {
					for _, p := range sshPorts {
						rules = append(rules, FirewallRule{ID: "service:ssh", Text: "service ssh", Action: "allow", Port: strconv.Itoa(p), Proto: "tcp"})
					}
					continue
				}
				rules = append(rules, FirewallRule{ID: "service:" + svc, Text: "service " + svc, Action: "allow"})
			}
		case strings.HasPrefix(line, "ports:"):
			for _, p := range strings.Fields(strings.TrimPrefix(line, "ports:")) {
				parts := strings.SplitN(p, "/", 2)
				r := FirewallRule{ID: "port:" + p, Text: p, Action: "allow", Port: parts[0]}
				if len(parts) == 2 {
					r.Proto = parts[1]
				}
				rules = append(rules, r)
			}
		}
	}
	return rules
}

func firewalldAllow(port, proto string) error {
	for _, p := range protos(proto) {
		if _, err := fwRun(15*time.Second, "firewall-cmd", "--permanent", "--add-port="+port+"/"+p); err != nil {
			return err
		}
	}
	_, err := fwRun(15*time.Second, "firewall-cmd", "--reload")
	return err
}

// ---------- ssh port detection ----------

// SSHPorts returns the ports sshd listens on (best effort, default 22).
func SSHPorts() []int {
	seen := map[int]bool{}
	if out, err := fwRun(5*time.Second, "sshd", "-T"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[0] == "port" {
				if n, err := strconv.Atoi(f[1]); err == nil {
					seen[n] = true
				}
			}
		}
	}
	if len(seen) == 0 {
		if raw, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				f := strings.Fields(line)
				if len(f) == 2 && strings.EqualFold(f[0], "Port") {
					if n, err := strconv.Atoi(f[1]); err == nil {
						seen[n] = true
					}
				}
			}
		}
	}
	if len(seen) == 0 {
		seen[22] = true
	}
	var ports []int
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

// ---------- public API ----------

// GetFirewallStatus collects the firewall state and checks the required ports.
func GetFirewallStatus(required []RequiredPort) FirewallStatus {
	st := FirewallStatus{Backend: DetectFirewall(), Rules: []FirewallRule{}}
	if st.Backend == "none" {
		st.Message = "未检测到防火墙工具（仅支持 Linux 上的 ufw / firewalld）"
		if runtime.GOOS == "linux" && has("apt-get") {
			st.CanInstallUFW = true
		}
		return st
	}
	st.SSHPorts = SSHPorts()
	switch st.Backend {
	case "ufw":
		ufwStatus(&st)
	case "firewalld":
		firewalldStatus(&st)
	case "iptables":
		out, _ := fwRun(10*time.Second, "iptables", "-S", "INPUT")
		st.Raw = out
		st.Message = "检测到仅有 iptables，面板只支持查看。Ubuntu/Debian 建议安装 ufw 以图形化管理。"
		st.CanInstallUFW = has("apt-get")
	}
	for _, p := range st.SSHPorts {
		required = append([]RequiredPort{{Label: "SSH", Port: strconv.Itoa(p), Proto: "tcp"}}, required...)
	}
	for _, r := range required {
		st.Checks = append(st.Checks, PortCheck{
			Label: r.Label, Port: r.Port, Proto: r.Proto,
			Allowed: covered(st.Rules, r.Port, r.Proto, st.SSHPorts),
		})
	}
	return st
}

// FirewallAllow opens port/proto ("both" means tcp+udp).
func FirewallAllow(port, proto string) error {
	port, proto, err := ValidatePortInput(port, proto)
	if err != nil {
		return err
	}
	switch DetectFirewall() {
	case "ufw":
		return ufwAllow(port, proto)
	case "firewalld":
		return firewalldAllow(port, proto)
	default:
		return errors.New("当前系统没有可管理的防火墙（ufw / firewalld）")
	}
}

// FirewallDelete removes a rule by the ID returned in FirewallStatus.Rules.
func FirewallDelete(id string) error {
	switch DetectFirewall() {
	case "ufw":
		// Only delete rules that currently exist, using their exact text.
		added, err := fwRun(10*time.Second, "ufw", "show", "added")
		if err != nil {
			return err
		}
		if !strings.Contains("\n"+added+"\n", "\nufw "+id+"\n") {
			return errors.New("规则不存在，可能已被删除，请刷新")
		}
		args := append([]string{"--force", "delete"}, strings.Fields(id)...)
		_, err = fwRun(15*time.Second, "ufw", args...)
		return err
	case "firewalld":
		var flag string
		switch {
		case strings.HasPrefix(id, "port:"):
			flag = "--remove-port=" + strings.TrimPrefix(id, "port:")
		case strings.HasPrefix(id, "service:"):
			flag = "--remove-service=" + strings.TrimPrefix(id, "service:")
		default:
			return errors.New("无效规则")
		}
		if _, err := fwRun(15*time.Second, "firewall-cmd", "--permanent", flag); err != nil {
			return err
		}
		_, err := fwRun(15*time.Second, "firewall-cmd", "--reload")
		return err
	default:
		return errors.New("当前系统没有可管理的防火墙")
	}
}

// FirewallEnable turns the firewall on, after making sure SSH and the panel stay reachable.
func FirewallEnable(keepOpen []RequiredPort) error {
	backend := DetectFirewall()
	if backend != "ufw" && backend != "firewalld" {
		return errors.New("当前系统没有可管理的防火墙")
	}
	if backend == "firewalld" {
		if _, err := fwRun(30*time.Second, "systemctl", "enable", "--now", "firewalld"); err != nil {
			return err
		}
	}
	for _, p := range SSHPorts() {
		keepOpen = append(keepOpen, RequiredPort{Port: strconv.Itoa(p), Proto: "tcp"})
	}
	for _, r := range keepOpen {
		if err := FirewallAllow(r.Port, r.Proto); err != nil {
			return err
		}
	}
	if backend == "ufw" {
		_, err := fwRun(30*time.Second, "ufw", "--force", "enable")
		return err
	}
	return nil
}

// FirewallDisable turns the firewall off.
func FirewallDisable() error {
	switch DetectFirewall() {
	case "ufw":
		_, err := fwRun(30*time.Second, "ufw", "disable")
		return err
	case "firewalld":
		_, err := fwRun(30*time.Second, "systemctl", "disable", "--now", "firewalld")
		return err
	default:
		return errors.New("当前系统没有可管理的防火墙")
	}
}

// InstallUFW installs ufw via apt (Debian/Ubuntu). It does not enable it.
func InstallUFW() (string, error) {
	if !has("apt-get") {
		return "", errors.New("仅支持 apt 系统（Debian/Ubuntu）自动安装 ufw")
	}
	return fwRun(5*time.Minute, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "ufw")
}

// RuleProtectsCritical reports whether deleting the rule would close SSH or a must-keep port.
func RuleProtectsCritical(st FirewallStatus, id string, critical []RequiredPort) string {
	var remaining []FirewallRule
	for _, r := range st.Rules {
		if r.ID != id {
			remaining = append(remaining, r)
		}
	}
	for _, c := range critical {
		if covered(st.Rules, c.Port, c.Proto, st.SSHPorts) && !covered(remaining, c.Port, c.Proto, st.SSHPorts) {
			return fmt.Sprintf("%s 端口 %s/%s", c.Label, c.Port, c.Proto)
		}
	}
	return ""
}

package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"mieru-ui/database"
)

// HostFromRequest strips the port from an HTTP Host header.
func HostFromRequest(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

func hostForURL(h string) string {
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}

// MierusLink builds a mieru simple share link (mierus://).
func MierusLink(u database.User, host string, sc database.ServerConf) string {
	q := url.Values{}
	q.Set("profile", u.Name)
	if sc.MTU > 0 {
		q.Set("mtu", strconv.Itoa(sc.MTU))
	}
	if sc.Multiplexing != "" {
		q.Set("multiplexing", sc.Multiplexing)
	}
	if sc.HandshakeMode != "" {
		q.Set("handshake-mode", sc.HandshakeMode)
	}
	for _, b := range sc.PortBindings {
		if b.PortRange != "" {
			q.Add("port", b.PortRange)
		} else {
			q.Add("port", strconv.Itoa(b.Port))
		}
		q.Add("protocol", b.Protocol)
	}
	return fmt.Sprintf("mierus://%s@%s?%s",
		url.UserPassword(u.Name, u.Password).String(), hostForURL(host), q.Encode())
}

// ClientJSON renders a mieru client config (for `mieru apply config`).
func ClientJSON(u database.User, host string, sc database.ServerConf) ([]byte, error) {
	server := map[string]any{}
	if net.ParseIP(host) != nil {
		server["ipAddress"] = host
	} else {
		server["domainName"] = host
	}
	bindings := []map[string]any{}
	for _, b := range sc.PortBindings {
		m := map[string]any{"protocol": b.Protocol}
		if b.PortRange != "" {
			m["portRange"] = b.PortRange
		} else {
			m["port"] = b.Port
		}
		bindings = append(bindings, m)
	}
	server["portBindings"] = bindings
	profile := map[string]any{
		"profileName": u.Name,
		"user":        map[string]any{"name": u.Name, "password": u.Password},
		"servers":     []any{server},
		"mtu":         sc.MTU,
	}
	if sc.Multiplexing != "" {
		profile["multiplexing"] = map[string]any{"level": sc.Multiplexing}
	}
	if sc.HandshakeMode != "" {
		profile["handshakeMode"] = sc.HandshakeMode
	}
	cfg := map[string]any{
		"profiles":           []any{profile},
		"activeProfile":      u.Name,
		"rpcPort":            8964,
		"socks5Port":         1080,
		"loggingLevel":       "INFO",
		"socks5ListenLAN":    false,
		"httpProxyPort":      8080,
		"httpProxyListenLAN": false,
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// MihomoYAML renders proxies for mihomo (Clash.Meta), one per port binding.
func MihomoYAML(u database.User, host string, sc database.ServerConf) string {
	var b strings.Builder
	b.WriteString("proxies:\n")
	for _, pb := range sc.PortBindings {
		portLabel := strconv.Itoa(pb.Port)
		if pb.PortRange != "" {
			portLabel = pb.PortRange
		}
		name := fmt.Sprintf("mieru-%s-%s-%s", u.Name, portLabel, pb.Protocol)
		fmt.Fprintf(&b, "  - name: %s\n", strconv.Quote(name))
		b.WriteString("    type: mieru\n")
		fmt.Fprintf(&b, "    server: %s\n", strconv.Quote(host))
		if pb.PortRange != "" {
			fmt.Fprintf(&b, "    port-range: %s\n", pb.PortRange)
		} else {
			fmt.Fprintf(&b, "    port: %d\n", pb.Port)
		}
		fmt.Fprintf(&b, "    transport: %s\n", pb.Protocol)
		b.WriteString("    udp: true\n")
		fmt.Fprintf(&b, "    username: %s\n", strconv.Quote(u.Name))
		fmt.Fprintf(&b, "    password: %s\n", strconv.Quote(u.Password))
		if sc.Multiplexing != "" {
			fmt.Fprintf(&b, "    multiplexing: %s\n", sc.Multiplexing)
		}
		if sc.HandshakeMode != "" {
			fmt.Fprintf(&b, "    handshake-mode: %s\n", sc.HandshakeMode)
		}
	}
	return b.String()
}

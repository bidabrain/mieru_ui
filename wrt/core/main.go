// mieru-wrt runs the Mieru client on OpenWrt: it reads /etc/config/mieru, starts the
// mihomo core (with the official mieru library) as a transparent proxy, and installs
// the nftables / policy-routing / dnsmasq glue, removing it again on exit.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bidabrain/mieru_ui/android/core/libcore"
)

var version = "dev"

const usage = `mieru-wrt ` + "%s" + ` — Mieru client for OpenWrt

usage: mieru-wrt <command> [args]

  run                 start the proxy (used by /etc/init.d/mieru)
  cleanup             remove firewall / routing / dnsmasq changes
  check               validate /etc/config/mieru
  gen                 print the generated core config and nftables rules
  parse-link <text>   parse mierus:// links (or "-" for stdin), print JSON
  delay [node]        test the latency of a node (default: the selected one)
  update-geo          download fresh GeoIP / GeoSite data
  version             print versions
`

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout) // procd logs stderr as daemon.err
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun()
	case "cleanup":
		removeNetfilter(nil)
		removeDnsmasq()
	case "check":
		_, _, err = loadSettings(configPath)
		if err == nil {
			fmt.Println("ok")
		}
	case "gen":
		err = cmdGen()
	case "parse-link":
		err = cmdParseLink(os.Args[2:])
	case "delay":
		err = cmdDelay(os.Args[2:])
	case "update-geo":
		err = cmdUpdateGeo()
	case "version", "-v", "--version":
		fmt.Printf("mieru-wrt %s (%s)\n", version, libcore.Version())
	default:
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// prepareData points the core at /etc/mieru and seeds the bundled geo data once.
func prepareData() error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	for _, f := range []string{"GeoSite.dat", "GeoIP.dat"} {
		dst := filepath.Join(dataDir, f)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := copyFile(filepath.Join(shareDir, f), dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return libcore.Init(dataDir)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func cmdRun() error {
	s, _, err := loadSettings(configPath)
	if err != nil {
		return err
	}
	if !s.Enabled {
		return errors.New("服务未启用（mieru.main.enabled=0）")
	}
	if err := prepareData(); err != nil {
		return err
	}
	opts, _ := json.Marshal(s.Core)
	if err := libcore.StartRouter(string(opts)); err != nil {
		return err
	}
	// Install the redirection only once the listeners are up, and always undo it on
	// the way out: a leftover TPROXY rule or dnsmasq forwarder would cut off the LAN.
	cleanup := func() {
		removeDnsmasq()
		removeNetfilter(s)
	}
	if err := applyNetfilter(s); err != nil {
		cleanup()
		return fmt.Errorf("设置防火墙失败: %w", err)
	}
	if err := applyDnsmasq(s); err != nil {
		cleanup()
		return fmt.Errorf("设置 dnsmasq 失败: %w", err)
	}
	log.Printf("mieru-wrt %s started: node %s, mode %s, LAN %v", version, s.NodeName, s.Core.Routing.Mode, s.LanIfnames)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	got := <-sig
	log.Printf("received %v, stopping", got)
	cleanup()
	libcore.Stop()
	return nil
}

func cmdGen() error {
	s, _, err := loadSettings(configPath)
	if err != nil {
		return err
	}
	opts, _ := json.Marshal(s.Core)
	cfg, err := libcore.PreviewConfig(string(opts))
	if err != nil {
		return err
	}
	fmt.Println("# ---- core config ----")
	fmt.Println(cfg)
	fmt.Println("# ---- nftables ----")
	fmt.Print(nftRuleset(s))
	return nil
}

func cmdParseLink(args []string) error {
	if len(args) != 1 {
		return errors.New("用法: parse-link <text|->")
	}
	text := args[0]
	if text == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		text = string(b)
	}
	js, err := libcore.ParseLinks(text)
	if err != nil {
		return err
	}
	// Re-shape for LuCI: ports as "2012/TCP" strings, ready for UCI lists.
	var ps []libcore.Profile
	if err := json.Unmarshal([]byte(js), &ps); err != nil {
		return err
	}
	type node struct {
		Name          string   `json:"name"`
		Server        string   `json:"server"`
		Username      string   `json:"username"`
		Password      string   `json:"password"`
		Ports         []string `json:"port"`
		MTU           int      `json:"mtu,omitempty"`
		Multiplexing  string   `json:"multiplexing,omitempty"`
		HandshakeMode string   `json:"handshake_mode,omitempty"`
	}
	out := make([]node, 0, len(ps))
	for _, p := range ps {
		n := node{Name: p.Name, Server: p.Server, Username: p.Username, Password: p.Password, MTU: p.MTU, Multiplexing: p.Multiplexing, HandshakeMode: p.HandshakeMode}
		for _, b := range p.Bindings {
			n.Ports = append(n.Ports, bindingSpec(b))
		}
		out = append(out, n)
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func cmdDelay(args []string) error {
	secs, err := loadSections(configPath)
	if err != nil {
		return err
	}
	main := findSection(secs, "mieru", "main")
	name := ""
	testURL := "https://www.gstatic.com/generate_204"
	if main != nil {
		name = main.get("node", "")
		testURL = main.get("test_url", testURL)
	}
	var n *uciSection
	switch {
	case len(args) == 2 && args[0] == "--json":
		// A node as LuCI currently holds it (possibly not saved yet).
		if n, err = nodeFromJSON(args[1]); err != nil {
			return err
		}
	case len(args) > 0:
		name = args[0]
		fallthrough
	default:
		if n = findSection(secs, "node", name); n == nil {
			return fmt.Errorf("节点 %q 不存在（导入或修改后请先保存并应用）", name)
		}
	}
	p, err := nodeProfile(n)
	if err != nil {
		return err
	}
	if err := prepareData(); err != nil {
		return err
	}
	pj, _ := json.Marshal(p)
	ms, err := libcore.TestDelay(string(pj), testURL, 6000)
	res := map[string]any{"node": n.Name}
	if err != nil {
		// mieru keys depend on the clock: a skewed router clock looks like a dead server.
		res["error"] = fmt.Sprintf("%v（请检查：服务器端口是否放行、用户名密码是否正确、路由器时间是否准确，当前 %s）",
			err, time.Now().Format("2006-01-02 15:04:05 MST"))
	} else {
		res["ms"] = ms
	}
	return json.NewEncoder(os.Stdout).Encode(res)
}

// nodeFromJSON builds a node section from {"name","server","username","password",
// "port":[...],"mtu","multiplexing","handshake_mode"} (the parse-link output shape).
func nodeFromJSON(js string) (*uciSection, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return nil, fmt.Errorf("节点 JSON 无效: %w", err)
	}
	n := &uciSection{Type: "node", Name: "unsaved", Options: map[string]string{}, Lists: map[string][]string{}}
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			if k == "port" {
				n.Lists[k] = []string{t}
			} else {
				n.Options[k] = t
			}
		case float64:
			n.Options[k] = fmt.Sprint(int(t))
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					n.Lists[k] = append(n.Lists[k], s)
				}
			}
		}
	}
	if s, ok := raw[".name"].(string); ok && s != "" {
		n.Name = s
	}
	return n, nil
}

func cmdUpdateGeo() error {
	s, _, err := loadSettings(configPath)
	if err != nil {
		return err
	}
	if err := prepareData(); err != nil {
		return err
	}
	opts, _ := json.Marshal(s.Core)
	if err := libcore.UpdateGeoFor(string(opts)); err != nil {
		return err
	}
	fmt.Println("规则数据已更新，重启服务后生效")
	return nil
}

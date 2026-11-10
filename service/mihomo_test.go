package service

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"mieru-ui/database"
)

func testInput() (database.User, database.ServerConf) {
	u := database.User{Name: "alice", Password: `p@ss "word": x`}
	sc := database.ServerConf{
		PortBindings: []database.PortBinding{{Port: 2012, Protocol: "TCP"}, {PortRange: "3000-3010", Protocol: "UDP"}},
		MTU:          1400, Multiplexing: "MULTIPLEXING_LOW",
	}
	return u, sc
}

func TestBuildMihomoDefault(t *testing.T) {
	u, sc := testInput()
	c := database.DefaultMihomo()
	if err := ValidateMihomo(&c); err != nil {
		t.Fatalf("default template invalid: %v", err)
	}
	out, err := BuildMihomo(u, "example.com", sc, c)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, out)
	}
	proxies := doc["proxies"].([]any)
	if len(proxies) != 2 {
		t.Fatalf("want 2 proxies, got %d", len(proxies))
	}
	p0 := proxies[0].(map[string]any)
	if p0["password"] != u.Password || p0["type"] != "mieru" || p0["port"] != 2012 {
		t.Errorf("proxy 0 wrong: %v", p0)
	}
	if proxies[1].(map[string]any)["port-range"] != "3000-3010" {
		t.Errorf("proxy 1 should use port-range: %v", proxies[1])
	}
	groups := doc["proxy-groups"].([]any)
	if len(groups) != 3 || groups[0].(map[string]any)["name"] != c.GroupName {
		t.Errorf("groups wrong: %v", groups)
	}
	rules := doc["rules"].([]any)
	last := rules[len(rules)-1].(string)
	if last != "MATCH,"+c.GroupName {
		t.Errorf("bypass_cn should end with MATCH to proxy, got %s", last)
	}
	joined := strings.Join(toStrings(rules), "\n")
	for _, want := range []string{"GEOSITE,cn,DIRECT", "GEOIP,CN,DIRECT", "GEOSITE,category-ads-all,REJECT", "GEOIP,telegram," + c.GroupName + ",no-resolve"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing rule %q", want)
		}
	}
}

func toStrings(in []any) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.(string)
	}
	return out
}

func TestMihomoRouteModes(t *testing.T) {
	c := database.DefaultMihomo()
	c.RouteMode = "gfw"
	r := MihomoRules(c)
	if r[len(r)-1] != "MATCH,DIRECT" || !contains(r, "GEOSITE,gfw,"+c.GroupName) {
		t.Errorf("gfw rules wrong: %v", r)
	}
	c.RouteMode = "global"
	r = MihomoRules(c)
	if r[len(r)-1] != "MATCH,"+c.GroupName || contains(r, "GEOSITE,cn,DIRECT") {
		t.Errorf("global rules wrong: %v", r)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestMihomoCustomRulesOrder(t *testing.T) {
	c := database.DefaultMihomo()
	c.CustomRules = []string{"ip-cidr, 10.1.0.0/16 , direct, no-resolve", "# comment", "PROCESS-NAME,Telegram.exe,PROXY"}
	c.DirectDomains = []string{"nas.example.com"}
	c.ProxyDomains = []string{" example.org "}
	c.RejectDomains = []string{"ads.example.net"}
	if err := ValidateMihomo(&c); err != nil {
		t.Fatal(err)
	}
	r := MihomoRules(c)
	want := []string{
		"IP-CIDR,10.1.0.0/16,DIRECT,no-resolve",
		"PROCESS-NAME,Telegram.exe," + c.GroupName,
		"DOMAIN-SUFFIX,ads.example.net,REJECT",
		"DOMAIN-SUFFIX,nas.example.com,DIRECT",
		"DOMAIN-SUFFIX,example.org," + c.GroupName,
	}
	for i, w := range want {
		if r[i] != w {
			t.Errorf("rule %d = %q, want %q", i, r[i], w)
		}
	}
}

func TestValidateMihomoRejectsBadInput(t *testing.T) {
	cases := map[string]func(c *database.MihomoConf){
		"group comma":      func(c *database.MihomoConf) { c.GroupName = "a,b" },
		"group newline":    func(c *database.MihomoConf) { c.GroupName = "a\nrules:" },
		"group reserved":   func(c *database.MihomoConf) { c.GroupName = "DIRECT" },
		"domain injection": func(c *database.MihomoConf) { c.DirectDomains = []string{"a.com\n- MATCH,DIRECT"} },
		"domain comma":     func(c *database.MihomoConf) { c.ProxyDomains = []string{"a.com,DIRECT"} },
		"rule target":      func(c *database.MihomoConf) { c.CustomRules = []string{"DOMAIN,a.com,SomeGroup"} },
		"rule type":        func(c *database.MihomoConf) { c.CustomRules = []string{"SCRIPT,x,DIRECT"} },
		"rule extra":       func(c *database.MihomoConf) { c.CustomRules = []string{"DOMAIN,a.com,DIRECT,foo"} },
		"dns space":        func(c *database.MihomoConf) { c.Nameservers = []string{"1.1.1.1 evil"} },
		"service":          func(c *database.MihomoConf) { c.ProxyServices = []string{"not-a-service"} },
		"port":             func(c *database.MihomoConf) { c.MixedPort = 70000 },
		"test url":         func(c *database.MihomoConf) { c.TestURL = "javascript:alert(1)" },
		"route mode":       func(c *database.MihomoConf) { c.RouteMode = "x" },
		"controller":       func(c *database.MihomoConf) { c.ExternalController = "0.0.0.0" },
		"fake range":       func(c *database.MihomoConf) { c.FakeIPRange = "nope" },
	}
	for name, mutate := range cases {
		c := database.DefaultMihomo()
		mutate(&c)
		if err := ValidateMihomo(&c); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestEncodeYAMLKeepsEmoji(t *testing.T) {
	u, sc := testInput()
	u.Name = "bob"
	u.Password = `🔑 "quoted" \U0001F680 back\slash`
	c := database.DefaultMihomo() // group name contains 🚀
	out, err := BuildMihomo(u, "example.com", sc, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `\U0001F680 节点`) || !strings.Contains(out, "🚀 节点选择") {
		t.Errorf("emoji should stay literal:\n%s", out)
	}
	var doc struct {
		Proxies []struct {
			Password string `yaml:"password"`
		} `yaml:"proxies"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, out)
	}
	if doc.Proxies[0].Password != u.Password {
		t.Errorf("password round-trip failed: got %q want %q", doc.Proxies[0].Password, u.Password)
	}
	if doc.Rules[len(doc.Rules)-1] != "MATCH,"+c.GroupName {
		t.Errorf("rule round-trip failed: %q", doc.Rules[len(doc.Rules)-1])
	}
}

func TestBuildMihomoNoDNS(t *testing.T) {
	u, sc := testInput()
	c := database.DefaultMihomo()
	c.DNSEnable = false
	c.AutoGroup, c.FallbackGroup = false, false
	c.GeoxSource = "default"
	out, err := BuildMihomo(u, "1.2.3.4", sc, c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\ndns:") || strings.Contains(out, "geox-url") || strings.Contains(out, autoGroupName) {
		t.Errorf("unexpected sections:\n%s", out)
	}
}

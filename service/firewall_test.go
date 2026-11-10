package service

import "testing"

const ufwAdded = `Added user rules (see 'ufw status' for running firewall):
ufw allow 22/tcp
ufw allow OpenSSH
ufw allow 2012/tcp
ufw allow 3000:3010/udp
ufw allow 8443
ufw deny 25
ufw allow from 1.2.3.4 to any port 9000
ufw limit in 2222/tcp
`

func TestParseUFWAdded(t *testing.T) {
	rules := parseUFWAdded(ufwAdded, []int{22, 2200})
	want := []FirewallRule{
		{ID: "allow 22/tcp", Action: "allow", Port: "22", Proto: "tcp"},
		{ID: "allow OpenSSH", Action: "allow", Port: "22", Proto: "tcp"},
		{ID: "allow OpenSSH", Action: "allow", Port: "2200", Proto: "tcp"},
		{ID: "allow 2012/tcp", Action: "allow", Port: "2012", Proto: "tcp"},
		{ID: "allow 3000:3010/udp", Action: "allow", Port: "3000-3010", Proto: "udp"},
		{ID: "allow 8443", Action: "allow", Port: "8443", Proto: "both"},
		{ID: "deny 25", Action: "deny", Port: "25", Proto: "both"},
		{ID: "allow from 1.2.3.4 to any port 9000", Action: "allow"},
		{ID: "limit in 2222/tcp", Action: "limit", Port: "2222", Proto: "tcp"},
	}
	if len(rules) != len(want) {
		t.Fatalf("got %d rules, want %d: %+v", len(rules), len(want), rules)
	}
	for i, w := range want {
		g := rules[i]
		if g.ID != w.ID || g.Action != w.Action || g.Port != w.Port || g.Proto != w.Proto {
			t.Errorf("rule %d: got %+v, want %+v", i, g, w)
		}
	}
}

func TestParseFirewalld(t *testing.T) {
	out := `public (active)
  target: default
  interfaces: eth0
  services: dhcpv6-client ssh
  ports: 2012/tcp 3000-3010/udp
  protocols:`
	rules := parseFirewalld(out, []int{22})
	if len(rules) != 4 {
		t.Fatalf("got %+v", rules)
	}
	if rules[1].ID != "service:ssh" || rules[1].Port != "22" {
		t.Errorf("ssh service not mapped: %+v", rules[1])
	}
	if rules[3].ID != "port:3000-3010/udp" || rules[3].Port != "3000-3010" || rules[3].Proto != "udp" {
		t.Errorf("range: %+v", rules[3])
	}
}

func TestCovered(t *testing.T) {
	rules := parseUFWAdded(ufwAdded, []int{22})
	cases := []struct {
		port, proto string
		want        bool
	}{
		{"22", "tcp", true},
		{"2012", "tcp", true},
		{"2012", "udp", false},
		{"3000-3010", "udp", true},
		{"3005", "udp", true},
		{"2999-3005", "udp", false},
		{"8443", "udp", true},
		{"25", "tcp", false}, // deny rule
		{"9000", "tcp", false},
		{"2222", "tcp", true}, // limit counts as allowed
	}
	for _, c := range cases {
		if got := covered(rules, c.port, c.proto, nil); got != c.want {
			t.Errorf("covered(%s/%s) = %v, want %v", c.port, c.proto, got, c.want)
		}
	}
}

func TestRuleProtectsCritical(t *testing.T) {
	st := FirewallStatus{Rules: parseUFWAdded(ufwAdded, []int{22}), SSHPorts: []int{22}}
	ssh := []RequiredPort{{Label: "SSH", Port: "22", Proto: "tcp"}, {Label: "面板", Port: "2053", Proto: "tcp"}}
	// 22/tcp is also covered by OpenSSH, so deleting one of them is safe.
	if w := RuleProtectsCritical(st, "allow 22/tcp", ssh); w != "" {
		t.Errorf("unexpected block: %s", w)
	}
	st.Rules = parseUFWAdded("ufw allow 22/tcp\nufw allow 2012/tcp\n", []int{22})
	if w := RuleProtectsCritical(st, "allow 22/tcp", ssh); w == "" {
		t.Error("deleting the only SSH rule should be blocked")
	}
	if w := RuleProtectsCritical(st, "allow 2012/tcp", ssh); w != "" {
		t.Errorf("unexpected block: %s", w)
	}
}

func TestValidatePortInput(t *testing.T) {
	good := map[string]string{"2012": "2012", " 3000:3010 ": "3000-3010", "3000-3010": "3000-3010", "65535": "65535"}
	for in, want := range good {
		got, _, err := ValidatePortInput(in, "tcp")
		if err != nil || got != want {
			t.Errorf("ValidatePortInput(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "70000", "3010-3000", "22; rm -rf /", "abc", "1-2-3"} {
		if _, _, err := ValidatePortInput(in, "tcp"); err == nil {
			t.Errorf("ValidatePortInput(%q) should fail", in)
		}
	}
	if _, _, err := ValidatePortInput("22", "icmp"); err == nil {
		t.Error("bad proto accepted")
	}
}

package database

// MihomoConf is the template used to generate full mihomo (Clash.Meta) client configs.
type MihomoConf struct {
	// General
	MixedPort          int    `json:"mixedPort"`
	AllowLan           bool   `json:"allowLan"`
	Mode               string `json:"mode"`     // rule | global | direct
	LogLevel           string `json:"logLevel"` // silent | error | warning | info | debug
	IPv6               bool   `json:"ipv6"`
	ExternalController string `json:"externalController"`
	Secret             string `json:"secret"`
	UnifiedDelay       bool   `json:"unifiedDelay"`
	TCPConcurrent      bool   `json:"tcpConcurrent"`

	// Routing
	RouteMode      string   `json:"routeMode"` // bypass_cn | gfw | global
	BlockAds       bool     `json:"blockAds"`
	PrivateDirect  bool     `json:"privateDirect"`
	AppleCNDirect  bool     `json:"appleCNDirect"`
	MSCNDirect     bool     `json:"msCNDirect"`
	ProxyServices  []string `json:"proxyServices"` // geosite categories forced through the proxy
	DirectDomains  []string `json:"directDomains"`
	ProxyDomains   []string `json:"proxyDomains"`
	RejectDomains  []string `json:"rejectDomains"`
	CustomRules    []string `json:"customRules"` // TYPE,VALUE,DIRECT|REJECT|PROXY[,no-resolve]
	GeoxSource     string   `json:"geoxSource"`  // default | github | jsdelivr
	GeoAutoUpdate  bool     `json:"geoAutoUpdate"`
	GeoUpdateHours int      `json:"geoUpdateHours"`

	// Proxy groups
	GroupName        string `json:"groupName"`
	AutoGroup        bool   `json:"autoGroup"`
	FallbackGroup    bool   `json:"fallbackGroup"`
	TestURL          string `json:"testURL"`
	TestIntervalSecs int    `json:"testIntervalSecs"`

	// DNS
	DNSEnable          bool     `json:"dnsEnable"`
	EnhancedMode       string   `json:"enhancedMode"` // fake-ip | redir-host
	FakeIPRange        string   `json:"fakeIPRange"`
	DefaultNameservers []string `json:"defaultNameservers"`
	Nameservers        []string `json:"nameservers"`
	Fallback           []string `json:"fallback"`
	FallbackGeoIP      bool     `json:"fallbackGeoIP"`
	FakeIPFilter       []string `json:"fakeIPFilter"`
}

// DefaultMihomo returns a sensible template for users in mainland China.
func DefaultMihomo() MihomoConf {
	return MihomoConf{
		MixedPort:          7890,
		Mode:               "rule",
		LogLevel:           "info",
		ExternalController: "127.0.0.1:9090",
		UnifiedDelay:       true,
		TCPConcurrent:      true,

		RouteMode:      "bypass_cn",
		BlockAds:       true,
		PrivateDirect:  true,
		AppleCNDirect:  true,
		MSCNDirect:     true,
		ProxyServices:  []string{"google", "telegram", "category-ai-!cn"},
		DirectDomains:  []string{},
		ProxyDomains:   []string{},
		RejectDomains:  []string{},
		CustomRules:    []string{},
		GeoxSource:     "jsdelivr",
		GeoAutoUpdate:  true,
		GeoUpdateHours: 24,

		GroupName:        "🚀 节点选择",
		AutoGroup:        true,
		FallbackGroup:    true,
		TestURL:          "https://www.gstatic.com/generate_204",
		TestIntervalSecs: 300,

		DNSEnable:          true,
		EnhancedMode:       "fake-ip",
		FakeIPRange:        "198.18.0.1/16",
		DefaultNameservers: []string{"223.5.5.5", "119.29.29.29"},
		Nameservers:        []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
		Fallback:           []string{"https://dns.cloudflare.com/dns-query", "https://dns.google/dns-query", "tls://8.8.4.4:853"},
		FallbackGeoIP:      true,
		FakeIPFilter: []string{
			"*.lan", "+.local", "+.msftconnecttest.com", "+.msftncsi.com",
			"time.*.com", "ntp.*.com", "+.pool.ntp.org", "+.stun.*.*", "localhost.ptlogin2.qq.com",
		},
	}
}

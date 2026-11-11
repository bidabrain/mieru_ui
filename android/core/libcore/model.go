package libcore

// Binding is one port (or port range) + transport of a mieru server.
type Binding struct {
	Port      int    `json:"port,omitempty"`
	PortRange string `json:"portRange,omitempty"`
	Protocol  string `json:"protocol"` // TCP | UDP
}

// Profile is one mieru server/account.
type Profile struct {
	ID            string    `json:"id,omitempty"`
	Name          string    `json:"name"`
	Server        string    `json:"server"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
	Bindings      []Binding `json:"bindings"`
	MTU           int       `json:"mtu,omitempty"`
	Multiplexing  string    `json:"multiplexing,omitempty"`
	HandshakeMode string    `json:"handshakeMode,omitempty"`
}

// Rule is a user routing rule.
type Rule struct {
	Type    string `json:"type"` // DOMAIN-SUFFIX, DOMAIN, DOMAIN-KEYWORD, GEOSITE, GEOIP, IP-CIDR, IP-CIDR6, DST-PORT
	Value   string `json:"value"`
	Action  string `json:"action"` // PROXY | DIRECT | REJECT
	Enabled bool   `json:"enabled"`
}

// Routing holds the husi-like routing (分流) settings.
type Routing struct {
	Mode          string   `json:"mode"` // bypass_cn | gfw | global | direct
	BypassLAN     bool     `json:"bypassLan"`
	BlockAds      bool     `json:"blockAds"`
	AppleCNDirect bool     `json:"appleCnDirect"`
	Rules         []Rule   `json:"rules"`
	DirectDNS     []string `json:"directDns"`
	RemoteDNS     []string `json:"remoteDns"`
	FakeIP        bool     `json:"fakeIp"`
	Sniff         bool     `json:"sniff"`
}

// Options is everything needed to build one runtime config.
type Options struct {
	Profile    Profile `json:"profile"`
	Routing    Routing `json:"routing"`
	IPv6       bool    `json:"ipv6"`
	Stack      string  `json:"stack"` // gvisor | system | mixed
	LogLevel   string  `json:"logLevel"`
	GeoxSource string  `json:"geoxSource"` // jsdelivr | github
	MixedPort  int     `json:"mixedPort"`  // local HTTP/SOCKS port on 127.0.0.1, 0 = off
	TunFd      int     `json:"-"`
	// Router switches from Android tun mode to OpenWrt transparent-proxy (TPROXY) mode.
	Router *Router `json:"router,omitempty"`
}

// Router holds the inbound settings for running on an OpenWrt router.
type Router struct {
	TProxyPort  int `json:"tproxyPort"`  // TPROXY listener for forwarded LAN traffic
	DNSPort     int `json:"dnsPort"`     // DNS listener on 127.0.0.1 (dnsmasq forwards to it)
	RoutingMark int `json:"routingMark"` // fwmark on the core's own sockets, 0 = none
}

// ProxyGroup is the name of the proxy group rules point at.
const ProxyGroup = "Proxy"

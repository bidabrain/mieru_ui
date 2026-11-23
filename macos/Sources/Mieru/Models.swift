import Foundation

func newId() -> String { UUID().uuidString.lowercased() }

// Codable models mirroring the Android client (and libcore's JSON). Every decoder falls
// back to the default for missing keys, since Go omits empty fields.

struct PortBinding: Codable, Hashable, Identifiable {
    var id = newId()
    var port: Int = 0
    var portRange: String = ""
    var proto: String = "TCP"

    var portText: String { portRange.isEmpty ? String(port) : portRange }

    enum CodingKeys: String, CodingKey { case port, portRange, proto = "protocol" }

    init(port: Int = 0, portRange: String = "", proto: String = "TCP") {
        self.port = port
        self.portRange = portRange
        self.proto = proto
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        port = try c.decodeIfPresent(Int.self, forKey: .port) ?? 0
        portRange = try c.decodeIfPresent(String.self, forKey: .portRange) ?? ""
        proto = try c.decodeIfPresent(String.self, forKey: .proto) ?? "TCP"
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(port, forKey: .port)
        try c.encode(portRange, forKey: .portRange)
        try c.encode(proto, forKey: .proto)
    }
}

struct Profile: Codable, Hashable, Identifiable {
    var id: String = newId()
    var name: String = ""
    var server: String = ""
    var username: String = ""
    var password: String = ""
    var bindings: [PortBinding] = [PortBinding()]
    var mtu: Int = 0
    var multiplexing: String = ""
    var handshakeMode: String = ""

    var summary: String {
        server + "  ·  " + bindings.map { "\($0.proto) \($0.portText)" }.joined(separator: "  ")
    }

    enum CodingKeys: String, CodingKey { case id, name, server, username, password, bindings, mtu, multiplexing, handshakeMode }

    init() {}

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? newId()
        if id.isEmpty { id = newId() }
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        server = try c.decodeIfPresent(String.self, forKey: .server) ?? ""
        username = try c.decodeIfPresent(String.self, forKey: .username) ?? ""
        password = try c.decodeIfPresent(String.self, forKey: .password) ?? ""
        bindings = try c.decodeIfPresent([PortBinding].self, forKey: .bindings) ?? []
        mtu = try c.decodeIfPresent(Int.self, forKey: .mtu) ?? 0
        multiplexing = try c.decodeIfPresent(String.self, forKey: .multiplexing) ?? ""
        handshakeMode = try c.decodeIfPresent(String.self, forKey: .handshakeMode) ?? ""
    }
}

struct Rule: Codable, Hashable, Identifiable {
    var type: String = "DOMAIN-SUFFIX"
    var value: String = ""
    var action: String = "PROXY"
    var enabled: Bool = true
    var id: String = newId()

    enum CodingKeys: String, CodingKey { case type, value, action, enabled, id }

    init(type: String = "DOMAIN-SUFFIX", value: String = "", action: String = "PROXY", enabled: Bool = true) {
        self.type = type
        self.value = value
        self.action = action
        self.enabled = enabled
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = try c.decodeIfPresent(String.self, forKey: .type) ?? "DOMAIN-SUFFIX"
        value = try c.decodeIfPresent(String.self, forKey: .value) ?? ""
        action = try c.decodeIfPresent(String.self, forKey: .action) ?? "PROXY"
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? true
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? newId()
    }
}

struct Routing: Codable, Hashable {
    var mode: String = "bypass_cn"
    var bypassLan: Bool = true
    var blockAds: Bool = false
    var appleCnDirect: Bool = true
    var rules: [Rule] = []
    var directDns: [String] = ["https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"]
    var remoteDns: [String] = ["https://1.1.1.1/dns-query", "https://dns.google/dns-query"]
    var fakeIp: Bool = true
    var sniff: Bool = true

    enum CodingKeys: String, CodingKey { case mode, bypassLan, blockAds, appleCnDirect, rules, directDns, remoteDns, fakeIp, sniff }

    init() {}

    init(from decoder: Decoder) throws {
        let d = Routing()
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? d.mode
        bypassLan = try c.decodeIfPresent(Bool.self, forKey: .bypassLan) ?? d.bypassLan
        blockAds = try c.decodeIfPresent(Bool.self, forKey: .blockAds) ?? d.blockAds
        appleCnDirect = try c.decodeIfPresent(Bool.self, forKey: .appleCnDirect) ?? d.appleCnDirect
        rules = try c.decodeIfPresent([Rule].self, forKey: .rules) ?? []
        directDns = try c.decodeIfPresent([String].self, forKey: .directDns) ?? d.directDns
        remoteDns = try c.decodeIfPresent([String].self, forKey: .remoteDns) ?? d.remoteDns
        fakeIp = try c.decodeIfPresent(Bool.self, forKey: .fakeIp) ?? d.fakeIp
        sniff = try c.decodeIfPresent(Bool.self, forKey: .sniff) ?? d.sniff
    }
}

enum PerAppMode {
    static let off = "off"
    static let proxy = "proxy"   // only selected apps use the proxy
    static let bypass = "bypass" // selected apps go direct
}

struct Settings: Codable, Hashable {
    var ipv6: Bool = false
    var stack: String = "gvisor"
    var logLevel: String = "info"
    var geoxSource: String = "jsdelivr"
    var mixedPort: Int = 17890
    var perAppMode: String = PerAppMode.off
    var perAppList: [String] = []
    var theme: String = "system"
    var autoCheckUpdate: Bool = true
    var lastUpdateCheck: Double = 0
    var testUrl: String = "https://www.gstatic.com/generate_204"
    var showInDock: Bool = true

    enum CodingKeys: String, CodingKey {
        case ipv6, stack, logLevel, geoxSource, mixedPort, perAppMode, perAppList, theme, autoCheckUpdate, lastUpdateCheck, testUrl, showInDock
    }

    init() {}

    init(from decoder: Decoder) throws {
        let d = Settings()
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ipv6 = try c.decodeIfPresent(Bool.self, forKey: .ipv6) ?? d.ipv6
        stack = try c.decodeIfPresent(String.self, forKey: .stack) ?? d.stack
        logLevel = try c.decodeIfPresent(String.self, forKey: .logLevel) ?? d.logLevel
        geoxSource = try c.decodeIfPresent(String.self, forKey: .geoxSource) ?? d.geoxSource
        mixedPort = try c.decodeIfPresent(Int.self, forKey: .mixedPort) ?? d.mixedPort
        perAppMode = try c.decodeIfPresent(String.self, forKey: .perAppMode) ?? d.perAppMode
        perAppList = try c.decodeIfPresent([String].self, forKey: .perAppList) ?? []
        theme = try c.decodeIfPresent(String.self, forKey: .theme) ?? d.theme
        autoCheckUpdate = try c.decodeIfPresent(Bool.self, forKey: .autoCheckUpdate) ?? d.autoCheckUpdate
        lastUpdateCheck = try c.decodeIfPresent(Double.self, forKey: .lastUpdateCheck) ?? 0
        testUrl = try c.decodeIfPresent(String.self, forKey: .testUrl) ?? d.testUrl
        showInDock = try c.decodeIfPresent(Bool.self, forKey: .showInDock) ?? d.showInDock
    }
}

struct AppData: Codable {
    var profiles: [Profile] = []
    var selectedId: String? = nil
    var routing = Routing()
    var settings = Settings()

    var selected: Profile? { profiles.first { $0.id == selectedId } ?? profiles.first }

    enum CodingKeys: String, CodingKey { case profiles, selectedId, routing, settings }

    init() {}

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        profiles = try c.decodeIfPresent([Profile].self, forKey: .profiles) ?? []
        selectedId = try c.decodeIfPresent(String.self, forKey: .selectedId)
        routing = try c.decodeIfPresent(Routing.self, forKey: .routing) ?? Routing()
        settings = try c.decodeIfPresent(Settings.self, forKey: .settings) ?? Settings()
    }
}

/// Shape sent to the Go core (mirrors libcore.Options).
struct CoreOptions: Encodable {
    struct Desktop: Encodable {
        var perAppMode: String
        var apps: [String]
    }
    var profile: Profile
    var routing: Routing
    var ipv6: Bool
    var stack: String
    var logLevel: String
    var geoxSource: String
    var mixedPort: Int
    var desktop: Desktop
}

struct Traffic: Codable {
    var up: Int64 = 0
    var down: Int64 = 0
    var upTotal: Int64 = 0
    var downTotal: Int64 = 0
    var conns: Int = 0
}

struct LogLine: Codable, Identifiable {
    var seq: Int64
    var time: Int64
    var level: String
    var msg: String
    var id: Int64 { seq }
}

struct LogBatch: Codable {
    var next: Int64
    var lines: [LogLine]
}

struct CoreStatus: Codable {
    var running: Bool = false
    var version: String = ""
    var coreVersion: String = ""
    var daemon: Bool = false
    var lastError: String? = nil
}

let routeModes: [(String, String)] = [
    ("bypass_cn", "绕过大陆"),
    ("gfw", "GFW 列表"),
    ("global", "全局"),
    ("direct", "直连"),
]

func routeModeLabel(_ m: String) -> String { routeModes.first { $0.0 == m }?.1 ?? m }

let routeModeHelp: [String: String] = [
    "bypass_cn": "国内网站和 IP 直连，其余走代理（推荐）",
    "gfw": "只有 GFW 列表里的网站走代理，其余直连",
    "global": "所有流量都走代理",
    "direct": "全部直连，只保留自定义规则",
]

let ruleTypes = ["DOMAIN-SUFFIX", "DOMAIN", "DOMAIN-KEYWORD", "GEOSITE", "GEOIP", "IP-CIDR", "DST-PORT"]
let ruleActions: [(String, String)] = [("PROXY", "代理"), ("DIRECT", "直连"), ("REJECT", "拦截")]
func ruleActionLabel(_ a: String) -> String { ruleActions.first { $0.0 == a }?.1 ?? a }

let ruleHints: [String: String] = [
    "DOMAIN-SUFFIX": "example.com（含所有子域名）",
    "DOMAIN": "www.example.com（完全匹配）",
    "DOMAIN-KEYWORD": "google（域名包含该词）",
    "GEOSITE": "如 netflix、openai、category-ads-all、cn",
    "GEOIP": "如 CN、telegram、google、netflix、private",
    "IP-CIDR": "如 10.0.0.0/8 或 2001:db8::/32",
    "DST-PORT": "如 22 或 6881-6889 或 80/443",
]

func formatBytes(_ b: Int64) -> String {
    if b < 1024 { return "\(b) B" }
    let units = ["KB", "MB", "GB", "TB"]
    var v = Double(b) / 1024
    var i = 0
    while v >= 1024 && i < units.count - 1 { v /= 1024; i += 1 }
    return v >= 100 ? String(format: "%.0f %@", v, units[i]) : String(format: "%.1f %@", v, units[i])
}

func formatSpeed(_ b: Int64) -> String { formatBytes(b) + "/s" }

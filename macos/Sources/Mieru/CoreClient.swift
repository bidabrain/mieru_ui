import Foundation
import Darwin

struct CoreError: LocalizedError {
    let message: String
    init(_ m: String) { message = m }
    var errorDescription: String? { message }
}

/// The daemon socket is missing or refused the connection.
struct CoreUnavailable: LocalizedError {
    var errorDescription: String? { "核心服务未运行，请在「设置」中安装核心服务" }
}

/// Arguments of a core API call; nil fields are left out.
struct CoreRequest: Encodable {
    var text: String? = nil
    var profile: Profile? = nil
    var rule: Rule? = nil
    var options: CoreOptions? = nil
    var url: String? = nil
    var timeoutMs: Int? = nil
    var after: Int64? = nil
}

/// Talks to mieru-core: the root daemon over its unix socket, or, for calls that do not
/// need the running proxy, the bundled binary directly (`mieru-core call <method>`).
enum CoreClient {
    static let socketPath = "/var/run/mieru-core.sock"
    static let statelessMethods: Set<String> = ["parse", "link", "checkProfile", "checkRule", "defaultRouting", "preview", "delay", "status"]

    static var bundledCore: URL? {
        if let u = Bundle.main.url(forAuxiliaryExecutable: "mieru-core") { return u }
        // `swift run` during development: look next to the executable.
        let exe = URL(fileURLWithPath: CommandLine.arguments[0]).deletingLastPathComponent().appendingPathComponent("mieru-core")
        return FileManager.default.isExecutableFile(atPath: exe.path) ? exe : nil
    }

    /// Calls `method` and decodes its result. Blocking: never call on the main thread.
    static func call<T: Decodable>(_ method: String, _ req: CoreRequest = CoreRequest(), as: T.Type = T.self, timeout: Int = 30) throws -> T {
        let body = try JSONEncoder().encode(req)
        var raw: Data
        do {
            raw = try daemonRequest(method, body, timeout: timeout)
        } catch is CoreUnavailable {
            guard statelessMethods.contains(method) else { throw CoreUnavailable() }
            raw = try runLocal(method, body)
        }
        let result = try unwrap(raw)
        // Wrap in an array so top-level strings / numbers decode on every macOS version.
        let wrapped = Data("[".utf8) + result + Data("]".utf8)
        do {
            return try JSONDecoder().decode([T].self, from: wrapped)[0]
        } catch {
            throw CoreError("核心返回数据无法解析: \(error.localizedDescription)")
        }
    }

    static func unwrap(_ raw: Data) throws -> Data {
        guard let obj = (try? JSONSerialization.jsonObject(with: raw)) as? [String: Any] else {
            throw CoreError("核心返回格式错误")
        }
        if (obj["ok"] as? Bool) != true {
            throw CoreError((obj["error"] as? String) ?? "未知错误")
        }
        guard let r = obj["result"], !(r is NSNull) else { return Data("null".utf8) }
        return try JSONSerialization.data(withJSONObject: r, options: [.fragmentsAllowed])
    }

    static var daemonSocketExists: Bool { FileManager.default.fileExists(atPath: socketPath) }

    /// Minimal HTTP/1.0 POST over the unix socket; the daemon closes the connection after replying.
    static func daemonRequest(_ method: String, _ body: Data, timeout: Int) throws -> Data {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw CoreUnavailable() }
        defer { close(fd) }

        var tv = timeval(tv_sec: timeout, tv_usec: 0)
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        var one: Int32 = 1
        setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, socklen_t(MemoryLayout<Int32>.size))

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let path = Array(socketPath.utf8CString)
        withUnsafeMutableBytes(of: &addr.sun_path) { dst in
            path.withUnsafeBytes { src in
                dst.copyMemory(from: UnsafeRawBufferPointer(rebasing: src.prefix(dst.count)))
            }
        }
        let rc = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        if rc != 0 { throw CoreUnavailable() }

        var req = Data("POST /\(method) HTTP/1.0\r\nHost: mieru-core\r\nContent-Type: application/json\r\nContent-Length: \(body.count)\r\n\r\n".utf8)
        req.append(body)
        try req.withUnsafeBytes { (p: UnsafeRawBufferPointer) in
            var off = 0
            while off < p.count {
                let n = Darwin.write(fd, p.baseAddress! + off, p.count - off)
                if n <= 0 { throw CoreError("发送请求到核心服务失败") }
                off += n
            }
        }

        var out = Data()
        var buf = [UInt8](repeating: 0, count: 65536)
        while true {
            let n = Darwin.read(fd, &buf, buf.count)
            if n < 0 { throw CoreError("核心服务响应超时") }
            if n == 0 { break }
            out.append(buf, count: n)
        }
        guard let sep = out.range(of: Data("\r\n\r\n".utf8)) else { throw CoreError("核心服务响应不完整") }
        let head = String(decoding: out[out.startIndex..<sep.lowerBound], as: UTF8.self)
        let status = head.split(separator: " ", maxSplits: 2).dropFirst().first.map(String.init) ?? ""
        let payload = out[sep.upperBound...]
        if status != "200" {
            throw CoreError("核心服务返回 \(status): \(String(decoding: payload, as: UTF8.self))")
        }
        return Data(payload)
    }

    /// Runs the bundled binary for calls that do not need root.
    static func runLocal(_ method: String, _ body: Data) throws -> Data {
        guard let exe = bundledCore else { throw CoreError("找不到内置的 mieru-core") }
        let p = Process()
        p.executableURL = exe
        p.arguments = ["call", method]
        let input = Pipe(), output = Pipe()
        p.standardInput = input
        p.standardOutput = output
        p.standardError = FileHandle.nullDevice
        try p.run()
        input.fileHandleForWriting.write(body)
        try? input.fileHandleForWriting.close()
        let out = output.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        if out.isEmpty { throw CoreError("mieru-core 运行失败（退出码 \(p.terminationStatus)）") }
        return out
    }
}

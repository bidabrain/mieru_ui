import AppKit
import Foundation

/// Installs / removes the root launchd daemon (mieru-core) with one administrator prompt.
enum Helper {
    static let label = "io.github.bidabrain.mieru.core"
    static let installedBinary = "/Library/PrivilegedHelperTools/\(label)"
    static let plist = "/Library/LaunchDaemons/\(label).plist"
    static let logFile = "/Library/Logs/Mieru/core.log"

    static var isInstalled: Bool { FileManager.default.fileExists(atPath: plist) }

    static var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "dev"
    }

    private static func shQuote(_ s: String) -> String {
        "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }

    /// Runs the bundled install script as root (shows the system password dialog).
    private static func runPrivileged(_ args: [String]) throws {
        guard let script = Bundle.main.url(forResource: "install-helper", withExtension: "sh") else {
            throw CoreError("应用包缺少 install-helper.sh")
        }
        let cmd = (["/bin/sh", script.path] + args).map(shQuote).joined(separator: " ")
        let escaped = cmd.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
        let source = "do shell script \"\(escaped)\" with administrator privileges"
        var err: NSDictionary?
        guard let apple = NSAppleScript(source: source) else { throw CoreError("无法创建授权脚本") }
        apple.executeAndReturnError(&err)
        if let err = err {
            let code = err[NSAppleScript.errorNumber] as? Int ?? 0
            if code == -128 { throw CoreError("已取消授权") }
            throw CoreError((err[NSAppleScript.errorMessage] as? String) ?? "安装失败（\(code)）")
        }
    }

    /// Must be called on the main thread (the authorization dialog is modal).
    static func install() throws {
        guard let core = CoreClient.bundledCore else { throw CoreError("找不到内置的 mieru-core") }
        let seed = Bundle.main.resourceURL?.appendingPathComponent("geo").path ?? ""
        try runPrivileged(["install", core.path, String(getuid()), seed])
    }

    static func uninstall() throws {
        try runPrivileged(["uninstall"])
    }
}

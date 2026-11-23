import AppKit
import Combine
import Foundation

/// Runs blocking core calls off the main thread.
func background<T>(_ work: @escaping () throws -> T) async throws -> T {
    try await withCheckedThrowingContinuation { cont in
        DispatchQueue.global(qos: .userInitiated).async {
            do { cont.resume(returning: try work()) } catch { cont.resume(throwing: error) }
        }
    }
}

enum ConnState: Equatable {
    case stopped, starting, running, stopping
}

/// App-wide state: connection, traffic, latency results and toasts.
@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()

    @Published var state: ConnState = .stopped
    @Published var daemonReachable = false
    @Published var coreStatus = CoreStatus()
    @Published var traffic = Traffic()
    @Published var delays: [String: Int] = [:] // profile id → ms; 0 = testing, -1 = failed
    @Published var toast: String?
    @Published var lastError: String?
    @Published var pendingImport: String?
    @Published var installing = false

    let store = Store.shared
    private var timer: Timer?
    private var toastTask: Task<Void, Never>?
    private var polling = false

    var running: Bool { state == .running }
    var busy: Bool { state == .starting || state == .stopping }

    /// Daemon is reachable but was built for a different app version.
    var helperOutdated: Bool {
        daemonReachable && coreStatus.version != Helper.appVersion && Helper.appVersion != "dev"
    }

    private init() {
        timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.poll() }
        }
        Task { await poll() }
    }

    func show(_ msg: String) {
        toast = msg
        toastTask?.cancel()
        toastTask = Task {
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            if !Task.isCancelled { self.toast = nil }
        }
    }

    func poll() async {
        if polling { return }
        polling = true
        defer { polling = false }
        guard CoreClient.daemonSocketExists else {
            daemonReachable = false
            if state == .running { state = .stopped }
            return
        }
        do {
            let st: CoreStatus = try await background { try CoreClient.call("status", timeout: 3) }
            daemonReachable = st.daemon
            coreStatus = st
            if !busy { state = st.running ? .running : .stopped }
            if st.running {
                traffic = try await background { try CoreClient.call("traffic", timeout: 3) }
            } else {
                traffic = Traffic()
            }
        } catch {
            daemonReachable = false
            if !busy { state = .stopped }
        }
    }

    func options(_ p: Profile) -> CoreOptions {
        let d = store.data
        return CoreOptions(
            profile: p, routing: d.routing, ipv6: d.settings.ipv6, stack: d.settings.stack,
            logLevel: d.settings.logLevel, geoxSource: d.settings.geoxSource, mixedPort: d.settings.mixedPort,
            desktop: .init(perAppMode: d.settings.perAppMode, apps: d.settings.perAppList)
        )
    }

    func toggle() {
        if running { disconnect() } else { connect() }
    }

    func connect() {
        guard !busy else { return }
        guard let p = store.data.selected else {
            show("请先添加节点")
            return
        }
        guard daemonReachable || CoreClient.daemonSocketExists else {
            lastError = "核心服务未安装或未运行。连接需要管理员权限创建虚拟网卡，请在「设置 → 核心服务」中安装。"
            return
        }
        let opts = options(p)
        state = .starting
        lastError = nil
        Task {
            do {
                let _: Bool = try await background { try CoreClient.call("start", CoreRequest(options: opts), timeout: 60) }
                state = .running
            } catch {
                state = .stopped
                lastError = error.localizedDescription
            }
            await poll()
        }
    }

    func disconnect() {
        guard !busy else { return }
        state = .stopping
        Task {
            do {
                let _: Bool = try await background { try CoreClient.call("stop", timeout: 30) }
            } catch {
                lastError = error.localizedDescription
            }
            state = .stopped
            traffic = Traffic()
            await poll()
        }
    }

    /// Applies changed settings by restarting the core when connected.
    func reloadIfRunning() {
        guard running, let p = store.data.selected else { return }
        let opts = options(p)
        state = .starting
        Task {
            do {
                let _: Bool = try await background { try CoreClient.call("start", CoreRequest(options: opts), timeout: 60) }
                state = .running
            } catch {
                state = .stopped
                lastError = error.localizedDescription
            }
        }
    }

    func select(_ p: Profile) {
        guard p.id != store.data.selected?.id else { return }
        store.select(p.id)
        reloadIfRunning()
    }

    func setMode(_ mode: String) {
        guard mode != store.data.routing.mode else { return }
        store.updateRouting { $0.mode = mode }
        reloadIfRunning()
    }

    func testDelay(_ p: Profile) {
        if delays[p.id] == 0 { return }
        delays[p.id] = 0
        let url = store.data.settings.testUrl
        Task {
            do {
                let ms: Int = try await background {
                    try CoreClient.call("delay", CoreRequest(profile: p, url: url, timeoutMs: 6000), timeout: 20)
                }
                delays[p.id] = max(ms, 1)
            } catch {
                delays[p.id] = -1
            }
        }
    }

    func testAll() { store.data.profiles.forEach(testDelay) }

    /// Imports mierus:// links or a mieru client JSON. Returns the number imported.
    @discardableResult
    func importText(_ text: String) async -> Int {
        do {
            var ps: [Profile] = try await background { try CoreClient.call("parse", CoreRequest(text: text)) }
            for i in ps.indices { ps[i].id = newId() }
            store.addProfiles(ps)
            show("已导入 \(ps.count) 个节点")
            return ps.count
        } catch {
            show(error.localizedDescription)
            return 0
        }
    }

    func installHelper() {
        installing = true
        defer { installing = false }
        do {
            try Helper.install()
            show("核心服务已安装")
        } catch {
            lastError = error.localizedDescription
        }
        Task { await poll() }
    }

    func uninstallHelper() {
        do {
            try Helper.uninstall()
            state = .stopped
            daemonReachable = false
            show("核心服务已卸载")
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Stop the proxy before quitting so the routes are removed.
    func shutdown() {
        guard running || busy else { return }
        _ = try? CoreClient.call("stop", timeout: 10) as Bool
    }
}

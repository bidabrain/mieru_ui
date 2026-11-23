import AppKit
import ServiceManagement
import SwiftUI

@main
struct MieruApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model = AppModel.shared
    @StateObject private var store = Store.shared

    var body: some Scene {
        Window("Mieru", id: "main") {
            ContentView()
                .environmentObject(model)
                .environmentObject(store)
                .frame(minWidth: 820, minHeight: 560)
        }
        .defaultSize(width: 960, height: 660)
        .commands {
            CommandGroup(replacing: .newItem) {}
            CommandMenu("连接") {
                Button(model.running ? "断开连接" : "连接") { model.toggle() }
                    .keyboardShortcut("k", modifiers: .command)
                    .disabled(model.busy)
                Button("全部测速") { model.testAll() }
                    .keyboardShortcut("t", modifiers: [.command, .shift])
            }
        }

        MenuBarExtra {
            MenuBarContent()
                .environmentObject(model)
                .environmentObject(store)
        } label: {
            Image(systemName: model.running ? "bolt.horizontal.circle.fill" : "bolt.horizontal.circle")
        }
        .menuBarExtraStyle(.menu)
    }
}

/// Keeps the app alive in the menu bar when the window is closed, and shows the Dock
/// icon only while a window is open.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var observers: [NSObjectProtocol] = []

    func applicationDidFinishLaunching(_ notification: Notification) {
        applyTheme(Store.shared.data.settings.theme)
        let nc = NotificationCenter.default
        observers.append(nc.addObserver(forName: NSWindow.didBecomeKeyNotification, object: nil, queue: .main) { _ in
            Task { @MainActor in AppDelegate.updateDockIcon() }
        })
        observers.append(nc.addObserver(forName: NSWindow.willCloseNotification, object: nil, queue: .main) { _ in
            // the closing window is still visible during willClose; check right after
            Task { @MainActor in
                await Task.yield()
                AppDelegate.updateDockIcon()
            }
        })
        Updater.shared.checkOnLaunch()
    }

    static func isAppWindow(_ w: NSWindow) -> Bool {
        w.isVisible && w.styleMask.contains(.titled) && w.sheetParent == nil && !(w is NSPanel)
    }

    static func updateDockIcon() {
        let hasWindow = NSApp.windows.contains(where: isAppWindow)
        let want: NSApplication.ActivationPolicy = (hasWindow && Store.shared.data.settings.showInDock) ? .regular : .accessory
        if NSApp.activationPolicy() != want {
            NSApp.setActivationPolicy(want)
            if want == .regular { NSApp.activate(ignoringOtherApps: true) }
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { AppDelegate.showMainWindow() }
        return true
    }

    func applicationWillTerminate(_ notification: Notification) {
        AppModel.shared.shutdown()
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        let links = urls.map(\.absoluteString).filter { $0.lowercased().hasPrefix("mierus://") }
        guard !links.isEmpty else { return }
        AppModel.shared.pendingImport = links.joined(separator: "\n")
        AppDelegate.showMainWindow()
    }

    /// Brings the main window back (from the menu bar / Dock).
    static func showMainWindow() {
        NSApp.setActivationPolicy(Store.shared.data.settings.showInDock ? .regular : .accessory)
        NSApp.activate(ignoringOtherApps: true)
        if let w = NSApp.windows.first(where: { $0.identifier?.rawValue.hasPrefix("main") == true }) {
            w.makeKeyAndOrderFront(nil)
        } else {
            OpenWindowBridge.open?()
        }
    }
}

/// SwiftUI's openWindow action, captured from a view so AppKit code can reopen the window.
@MainActor
enum OpenWindowBridge {
    static var open: (() -> Void)?
}

func applyTheme(_ theme: String) {
    switch theme {
    case "light": NSApp.appearance = NSAppearance(named: .aqua)
    case "dark": NSApp.appearance = NSAppearance(named: .darkAqua)
    default: NSApp.appearance = nil
    }
}

enum LoginItem {
    static var enabled: Bool { SMAppService.mainApp.status == .enabled }

    static func set(_ on: Bool) throws {
        if on { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
    }
}

@MainActor
struct MenuBarContent: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        let d = store.data
        Text(statusText(d))
        if model.running {
            Text("↑ \(formatSpeed(model.traffic.up))   ↓ \(formatSpeed(model.traffic.down))")
        }
        Button(model.running ? "断开连接" : "连接") { model.toggle() }
            .keyboardShortcut("k")
            .disabled(model.busy || (!model.running && d.profiles.isEmpty))

        Divider()

        if !d.profiles.isEmpty {
            Picker("节点", selection: Binding(
                get: { d.selected?.id ?? "" },
                set: { id in if let p = d.profiles.first(where: { $0.id == id }) { model.select(p) } }
            )) {
                ForEach(d.profiles) { p in
                    Text(p.name).tag(p.id)
                }
            }
        }
        Picker("分流模式", selection: Binding(get: { d.routing.mode }, set: { model.setMode($0) })) {
            ForEach(routeModes.indices, id: \.self) { i in
                Text(routeModes[i].1).tag(routeModes[i].0)
            }
        }

        if d.settings.mixedPort > 0 {
            Button("复制终端代理命令") {
                let p = d.settings.mixedPort
                let cmd = "export https_proxy=http://127.0.0.1:\(p) http_proxy=http://127.0.0.1:\(p) all_proxy=socks5://127.0.0.1:\(p)"
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(cmd, forType: .string)
            }
        }

        Divider()

        Button("打开主窗口") {
            openWindow(id: "main")
            AppDelegate.showMainWindow()
        }
        .keyboardShortcut("o")
        Button("退出 Mieru") { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }

    private func statusText(_ d: AppData) -> String {
        switch model.state {
        case .starting: return "正在连接…"
        case .stopping: return "正在断开…"
        case .running: return "已连接 · \(d.selected?.name ?? "")"
        case .stopped: return model.daemonReachable ? "未连接" : "未连接（核心服务未安装）"
        }
    }
}

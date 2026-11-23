import AppKit
import SwiftUI

enum Page: String, CaseIterable, Identifiable {
    case home, profiles, routing, apps, logs, settings
    var id: String { rawValue }

    var title: String {
        switch self {
        case .home: return "首页"
        case .profiles: return "节点"
        case .routing: return "分流"
        case .apps: return "分应用代理"
        case .logs: return "运行日志"
        case .settings: return "设置"
        }
    }

    var icon: String {
        switch self {
        case .home: return "house"
        case .profiles: return "server.rack"
        case .routing: return "arrow.triangle.branch"
        case .apps: return "square.grid.2x2"
        case .logs: return "doc.text"
        case .settings: return "gearshape"
        }
    }
}

@MainActor
struct ContentView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @Environment(\.openWindow) private var openWindow
    @State private var page: Page? = .home

    var body: some View {
        NavigationSplitView {
            List(selection: $page) {
                ForEach(Page.allCases) { p in
                    Label(p.title, systemImage: p.icon).tag(p)
                }
            }
            .navigationSplitViewColumnWidth(min: 170, ideal: 190, max: 240)
            .safeAreaInset(edge: .bottom) { SidebarStatus().padding(12) }
        } detail: {
            Group {
                switch page ?? .home {
                case .home: HomeView()
                case .profiles: ProfilesView()
                case .routing: RoutingView()
                case .apps: AppsView()
                case .logs: LogsView()
                case .settings: SettingsView()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .overlay(alignment: .bottom) {
            if let t = model.toast {
                Text(t)
                    .padding(.horizontal, 16).padding(.vertical, 10)
                    .background(.regularMaterial, in: Capsule())
                    .shadow(radius: 6)
                    .padding(.bottom, 24)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut(duration: 0.2), value: model.toast)
        .alert("出错了", isPresented: Binding(get: { model.lastError != nil }, set: { if !$0 { model.lastError = nil } })) {
            if !model.daemonReachable {
                Button("安装核心服务") { model.lastError = nil; model.installHelper() }
            }
            Button("好", role: .cancel) { model.lastError = nil }
        } message: {
            Text(model.lastError ?? "")
        }
        .alert("导入节点", isPresented: Binding(get: { model.pendingImport != nil }, set: { if !$0 { model.pendingImport = nil } })) {
            Button("导入") {
                if let t = model.pendingImport {
                    model.pendingImport = nil
                    Task { await model.importText(t) }
                }
            }
            Button("取消", role: .cancel) { model.pendingImport = nil }
        } message: {
            Text("是否导入这个 mierus:// 链接？")
        }
        .onAppear {
            OpenWindowBridge.open = { openWindow(id: "main") }
            AppDelegate.updateDockIcon()
        }
        .onChange(of: store.data.settings.theme) { applyTheme($0) }
    }
}

@MainActor
struct SidebarStatus: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(model.running ? Color.green : (model.busy ? Color.orange : Color.secondary.opacity(0.5)))
                .frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 2) {
                Text(model.running ? "已连接" : (model.busy ? "处理中…" : "未连接"))
                    .font(.caption.weight(.semibold))
                if model.running {
                    Text("↑ \(formatSpeed(model.traffic.up))  ↓ \(formatSpeed(model.traffic.down))")
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
        }
    }
}

/// A rounded card used on the home page.
@MainActor
struct Card<Content: View>: View {
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 10) { content }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Color.primary.opacity(0.06)))
    }
}

@MainActor
struct DelayBadge: View {
    @EnvironmentObject var model: AppModel
    let profile: Profile

    var body: some View {
        let r = model.delays[profile.id]
        let (text, color): (String, Color) = {
            switch r {
            case nil: return ("测速", .secondary)
            case 0?: return ("…", .secondary)
            case let v? where v < 0: return ("超时", .red)
            case let v? where v < 300: return ("\(v) ms", .green)
            case let v? where v < 800: return ("\(v) ms", .orange)
            case let v?: return ("\(v) ms", .red)
            }
        }()
        Button { model.testDelay(profile) } label: {
            Text(text)
                .font(.caption.weight(.semibold).monospacedDigit())
                .foregroundStyle(color)
                .padding(.horizontal, 9).padding(.vertical, 3)
                .background(color.opacity(0.14), in: Capsule())
        }
        .buttonStyle(.plain)
        .help("测试延迟")
    }
}

func copyToPasteboard(_ s: String) {
    NSPasteboard.general.clearContents()
    NSPasteboard.general.setString(s, forType: .string)
}

import AppKit
import SwiftUI

@MainActor
struct SettingsView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @ObservedObject private var updater = Updater.shared
    @State private var portText = ""
    @State private var testUrl = ""
    @State private var preview: String?
    @State private var loginItem = LoginItem.enabled
    @State private var confirmUninstall = false

    private func change(reload: Bool = true, _ fn: (inout Settings) -> Void) {
        store.updateSettings(fn)
        if reload { model.reloadIfRunning() }
    }

    private func binding<T>(_ key: WritableKeyPath<Settings, T>, reload: Bool = true) -> SwiftUI.Binding<T> {
        SwiftUI.Binding(get: { store.data.settings[keyPath: key] }, set: { v in change(reload: reload) { $0[keyPath: key] = v } })
    }

    var body: some View {
        let s = store.data.settings
        Form {
            Section("核心服务") {
                LabeledContent {
                    HStack {
                        if model.installing { ProgressView().controlSize(.small) }
                        Button(model.daemonReachable ? "重新安装" : "安装") { model.installHelper() }
                            .disabled(model.installing)
                        if Helper.isInstalled || model.daemonReachable {
                            Button("卸载") { confirmUninstall = true }
                        }
                    }
                } label: {
                    SettingLabel("后台核心服务",
                                 model.daemonReachable
                                     ? "运行中 · \(model.coreStatus.version)" + (model.helperOutdated ? "（需要更新）" : "")
                                     : "未安装或未运行。创建虚拟网卡需要管理员权限，安装时会请求一次授权。")
                }
            }

            Section("通用") {
                Toggle(isOn: SwiftUI.Binding(get: { loginItem }, set: { v in
                    do {
                        try LoginItem.set(v)
                    } catch {
                        model.lastError = "设置开机启动失败：\(error.localizedDescription)（请把 Mieru 放到「应用程序」文件夹后再试）"
                    }
                    loginItem = LoginItem.enabled
                })) { SettingLabel("开机时启动", "登录后自动在菜单栏运行") }
                Toggle(isOn: SwiftUI.Binding(get: { s.showInDock }, set: { v in
                    change(reload: false) { $0.showInDock = v }
                    AppDelegate.updateDockIcon()
                })) { SettingLabel("窗口打开时显示 Dock 图标", "关闭窗口后 Mieru 始终留在菜单栏") }
                Picker("主题", selection: binding(\.theme, reload: false)) {
                    Text("跟随系统").tag("system")
                    Text("浅色").tag("light")
                    Text("深色").tag("dark")
                }
            }

            Section("网络") {
                Toggle(isOn: binding(\.ipv6)) { SettingLabel("IPv6", "为虚拟网卡分配 IPv6 地址并代理 IPv6 流量") }
                Picker("TUN 网络栈", selection: binding(\.stack)) {
                    Text("gVisor（兼容性最好）").tag("gvisor")
                    Text("Mixed").tag("mixed")
                    Text("System（性能更好）").tag("system")
                }
                LabeledContent {
                    HStack {
                        TextField("", text: $portText, prompt: Text("留空关闭"))
                            .frame(width: 90)
                            .multilineTextAlignment(.trailing)
                            .onSubmit(savePort)
                        Button("保存", action: savePort)
                            .disabled(portText == (s.mixedPort > 0 ? String(s.mixedPort) : ""))
                    }
                } label: {
                    SettingLabel("本地代理端口", s.mixedPort > 0 ? "127.0.0.1:\(String(s.mixedPort))（HTTP / SOCKS5），仅本机可访问" : "已关闭")
                }
                Picker("规则数据下载源", selection: binding(\.geoxSource)) {
                    Text("jsDelivr CDN（国内较快）").tag("jsdelivr")
                    Text("GitHub Releases").tag("github")
                }
                Picker("日志级别", selection: binding(\.logLevel)) {
                    Text("调试").tag("debug")
                    Text("信息").tag("info")
                    Text("警告").tag("warning")
                    Text("错误").tag("error")
                    Text("关闭").tag("silent")
                }
            }

            Section("工具") {
                LabeledContent {
                    HStack {
                        TextField("", text: $testUrl).frame(minWidth: 240).onSubmit(saveTestUrl)
                        Button("保存", action: saveTestUrl).disabled(testUrl == s.testUrl)
                    }
                } label: {
                    SettingLabel("测速地址", "节点延迟测试请求的 URL")
                }
                LabeledContent {
                    Button("查看") { loadPreview() }
                } label: {
                    SettingLabel("查看生成的配置", "当前节点 + 分流设置生成的 mihomo 配置")
                }
                LabeledContent {
                    Button("打开") { NSWorkspace.shared.open(store.dir) }
                } label: {
                    SettingLabel("数据目录", store.dir.path)
                }
            }

            Section("更新") {
                Toggle(isOn: binding(\.autoCheckUpdate, reload: false)) {
                    SettingLabel("自动检查更新", "启动时每 12 小时检查一次 GitHub Release")
                }
                LabeledContent {
                    updateButton
                } label: {
                    SettingLabel("检查更新", updateText)
                }
                if case let .downloading(_, p) = updater.state {
                    ProgressView(value: p)
                }
                if case let .available(info) = updater.state, !info.notes.isEmpty {
                    Text(info.notes).font(.callout).foregroundStyle(.secondary).textSelection(.enabled)
                }
            }

            Section("关于") {
                LabeledContent("Mieru", value: Helper.appVersion)
                LabeledContent("内核", value: model.coreStatus.coreVersion.isEmpty ? "—" : model.coreStatus.coreVersion)
                LabeledContent {
                    Button("github.com/\(Updater.repo)") {
                        NSWorkspace.shared.open(URL(string: "https://github.com/\(Updater.repo)")!)
                    }
                    .buttonStyle(.link)
                } label: {
                    Text("项目主页")
                }
                LabeledContent("开源协议", value: "GPL-3.0 · 内核 mihomo + mieru")
            }
        }
        .formStyle(.grouped)
        .navigationTitle("设置")
        .onAppear {
            portText = s.mixedPort > 0 ? String(s.mixedPort) : ""
            testUrl = s.testUrl
            loginItem = LoginItem.enabled
        }
        .sheet(item: SwiftUI.Binding(get: { preview.map { PreviewText(text: $0) } }, set: { preview = $0?.text })) { p in
            VStack(alignment: .leading, spacing: 12) {
                Text("mihomo 配置").font(.title3.weight(.semibold))
                ScrollView {
                    Text(p.text)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .background(Color(nsColor: .textBackgroundColor))
                HStack {
                    Text("包含密码，请勿外传").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("复制") { copyToPasteboard(p.text); model.show("已复制（含密码）") }
                    Button("关闭") { preview = nil }.keyboardShortcut(.defaultAction)
                }
            }
            .padding(20)
            .frame(width: 640, height: 560)
        }
        .confirmationDialog("卸载核心服务？", isPresented: $confirmUninstall) {
            Button("卸载", role: .destructive) { model.uninstallHelper() }
        } message: {
            Text("卸载后将断开连接，再次连接前需要重新安装。")
        }
    }

    private var updateText: String {
        switch updater.state {
        case .idle: return "当前版本 \(Helper.appVersion)"
        case .checking: return "正在检查…"
        case .upToDate: return "已是最新版本 \(Helper.appVersion)"
        case let .available(i): return "发现新版本 \(i.version) · \(formatBytes(i.size))"
        case let .downloading(_, p): return "正在下载 \(Int(p * 100))%"
        case let .ready(_, f): return "已下载到 \(f.path)"
        case let .failed(m): return m
        }
    }

    @ViewBuilder private var updateButton: some View {
        switch updater.state {
        case let .available(i):
            HStack {
                Button("发布页") { NSWorkspace.shared.open(i.pageURL) }
                Button("下载") { updater.download(i) }.buttonStyle(.borderedProminent)
            }
        case .downloading, .checking:
            ProgressView().controlSize(.small)
        case let .ready(_, f):
            Button("打开安装包") { updater.install(f) }.buttonStyle(.borderedProminent)
        default:
            Button("检查") { Task { await updater.check() } }
        }
    }

    private func savePort() {
        let t = portText.trimmingCharacters(in: .whitespaces)
        if t.isEmpty {
            change { $0.mixedPort = 0 }
            return
        }
        guard let v = Int(t), (1...65535).contains(v) else {
            model.show("端口无效")
            return
        }
        change { $0.mixedPort = v }
        model.show("已保存")
    }

    private func saveTestUrl() {
        let t = testUrl.trimmingCharacters(in: .whitespaces)
        guard let u = URL(string: t), u.scheme == "http" || u.scheme == "https" else {
            model.show("地址无效")
            return
        }
        change(reload: false) { $0.testUrl = t }
        model.show("已保存")
    }

    private func loadPreview() {
        guard let p = store.data.selected else {
            model.show("没有节点")
            return
        }
        let o = model.options(p)
        Task {
            do {
                preview = try await background { try CoreClient.call("preview", CoreRequest(options: o)) }
            } catch {
                preview = "生成失败：" + error.localizedDescription
            }
        }
    }
}

struct PreviewText: Identifiable {
    let text: String
    var id: String { text }
}

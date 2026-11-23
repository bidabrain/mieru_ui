import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct InstalledApp: Identifiable, Hashable {
    let path: String
    let name: String
    var id: String { path }
}

enum AppScanner {
    static func scan() -> [InstalledApp] {
        let fm = FileManager.default
        var roots = ["/Applications", "/Applications/Utilities", "/System/Applications", "/System/Applications/Utilities"]
        roots.append((NSHomeDirectory() as NSString).appendingPathComponent("Applications"))
        var seen = Set<String>()
        var out: [InstalledApp] = []
        func add(_ path: String) {
            guard !seen.contains(path) else { return }
            seen.insert(path)
            out.append(InstalledApp(path: path, name: displayName(path)))
        }
        for root in roots {
            guard let items = try? fm.contentsOfDirectory(atPath: root) else { continue }
            for item in items {
                let path = (root as NSString).appendingPathComponent(item)
                if item.hasSuffix(".app") {
                    add(path)
                } else if !item.hasPrefix("."), let sub = try? fm.contentsOfDirectory(atPath: path) {
                    // one folder level, e.g. /Applications/Microsoft Office/
                    for s in sub where s.hasSuffix(".app") {
                        add((path as NSString).appendingPathComponent(s))
                    }
                }
            }
        }
        return out.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    static func displayName(_ path: String) -> String {
        FileManager.default.displayName(atPath: path).replacingOccurrences(of: ".app", with: "")
    }
}

@MainActor
struct AppsView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @State private var apps: [InstalledApp] = []
    @State private var loading = true
    @State private var query = ""
    @State private var onlySelected = false
    @State private var reloadTask: Task<Void, Never>?

    private var selected: Set<String> { Set(store.data.settings.perAppList) }

    private var allApps: [InstalledApp] {
        // keep apps chosen by hand (outside the scanned folders) in the list
        let known = Set(apps.map(\.path))
        let extra = store.data.settings.perAppList.filter { !known.contains($0) }
            .map { InstalledApp(path: $0, name: AppScanner.displayName($0)) }
        return extra + apps
    }

    private var visible: [InstalledApp] {
        let q = query.trimmingCharacters(in: .whitespaces)
        return allApps.filter { a in
            (!onlySelected || selected.contains(a.path)) &&
                (q.isEmpty || a.name.localizedCaseInsensitiveContains(q) || a.path.localizedCaseInsensitiveContains(q))
        }
    }

    private func update(mode: String? = nil, list: Set<String>? = nil) {
        store.updateSettings { s in
            if let mode { s.perAppMode = mode }
            if let list { s.perAppList = list.sorted() }
        }
        // toggling several apps in a row restarts the core once
        reloadTask?.cancel()
        reloadTask = Task {
            try? await Task.sleep(nanoseconds: 1_200_000_000)
            if !Task.isCancelled { model.reloadIfRunning() }
        }
    }

    var body: some View {
        let s = store.data.settings
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 10) {
                Picker("模式", selection: SwiftUI.Binding(get: { s.perAppMode }, set: { update(mode: $0) })) {
                    Text("关闭").tag(PerAppMode.off)
                    Text("仅选中应用走代理").tag(PerAppMode.proxy)
                    Text("选中应用不走代理").tag(PerAppMode.bypass)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                Text(help(s.perAppMode))
                    .font(.callout).foregroundStyle(.secondary)
                HStack {
                    TextField("搜索应用或路径", text: $query)
                        .textFieldStyle(.roundedBorder)
                    Toggle("只看已选", isOn: $onlySelected)
                        .toggleStyle(.checkbox)
                }
            }
            .padding(16)
            Divider()
            if loading {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List(visible) { app in
                    AppRow(app: app, checked: selected.contains(app.path)) { on in
                        var set = selected
                        if on { set.insert(app.path) } else { set.remove(app.path) }
                        update(list: set)
                    }
                }
                .listStyle(.inset(alternatesRowBackgrounds: true))
                .disabled(s.perAppMode == PerAppMode.off)
                .opacity(s.perAppMode == PerAppMode.off ? 0.55 : 1)
            }
        }
        .navigationTitle("分应用代理")
        .navigationSubtitle("已选 \(s.perAppList.count) 个")
        .toolbar {
            Menu {
                Button("全选当前列表") { update(list: selected.union(visible.map(\.path))) }
                Button("反选当前列表") {
                    let vis = Set(visible.map(\.path))
                    update(list: selected.subtracting(vis).union(vis.subtracting(selected)))
                }
                Button("清空") { update(list: []) }
                Divider()
                Button("添加其他应用…") { addOther() }
                Button("重新扫描") { load() }
            } label: {
                Label("操作", systemImage: "ellipsis.circle")
            }
        }
        .onAppear { if apps.isEmpty { load() } }
    }

    private func help(_ mode: String) -> String {
        switch mode {
        case PerAppMode.proxy: return "只有勾选的应用经过分流规则，其他应用全部直连。"
        case PerAppMode.bypass: return "勾选的应用直连，其他应用照常经过分流规则。"
        default: return "所有应用都经过分流规则。按应用（含其内部的辅助进程）的安装路径匹配。"
        }
    }

    private func load() {
        loading = true
        Task {
            let list = (try? await offMain { AppScanner.scan() }) ?? []
            apps = list
            loading = false
        }
    }

    private func addOther() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.application]
        panel.allowsMultipleSelection = true
        panel.directoryURL = URL(fileURLWithPath: "/Applications")
        guard panel.runModal() == .OK else { return }
        update(list: selected.union(panel.urls.map(\.path)))
    }
}

@MainActor
struct AppRow: View {
    let app: InstalledApp
    let checked: Bool
    let onChange: (Bool) -> Void

    var body: some View {
        HStack(spacing: 10) {
            Toggle("", isOn: SwiftUI.Binding(get: { checked }, set: onChange))
                .labelsHidden()
                .toggleStyle(.checkbox)
            Image(nsImage: NSWorkspace.shared.icon(forFile: app.path))
                .resizable()
                .frame(width: 26, height: 26)
            VStack(alignment: .leading, spacing: 1) {
                Text(app.name)
                Text(app.path).font(.caption).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
            }
            Spacer()
        }
        .contentShape(Rectangle())
        .onTapGesture { onChange(!checked) }
        .padding(.vertical, 2)
    }
}

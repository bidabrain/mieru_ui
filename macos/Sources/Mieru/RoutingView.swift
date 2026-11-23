import SwiftUI

@MainActor
struct RoutingView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @State private var editing: Rule?
    @State private var editingIsNew = false
    @State private var dnsEdit = false
    @State private var updatingGeo = false

    private func change(_ fn: (inout Routing) -> Void) {
        store.updateRouting(fn)
        model.reloadIfRunning()
    }

    private func toggle(_ key: WritableKeyPath<Routing, Bool>) -> SwiftUI.Binding<Bool> {
        SwiftUI.Binding(get: { store.data.routing[keyPath: key] }, set: { v in change { $0[keyPath: key] = v } })
    }

    var body: some View {
        let r = store.data.routing
        let s = store.data.settings
        Form {
            Section("模式") {
                Picker("分流模式", selection: SwiftUI.Binding(get: { r.mode }, set: { model.setMode($0) })) {
                    ForEach(routeModes.indices, id: \.self) { i in
                        Text(routeModes[i].1).tag(routeModes[i].0)
                    }
                }
                .pickerStyle(.segmented)
                Text(routeModeHelp[r.mode] ?? "").font(.callout).foregroundStyle(.secondary)
            }

            Section("常用") {
                Toggle(isOn: toggle(\.bypassLan)) { SettingLabel("绕过局域网", "局域网和保留地址直连") }
                Toggle(isOn: toggle(\.blockAds)) { SettingLabel("拦截广告", "GEOSITE category-ads-all") }
                Toggle(isOn: toggle(\.appleCnDirect)) { SettingLabel("Apple / 微软国内服务直连", "仅绕过大陆模式生效") }
                Toggle(isOn: toggle(\.sniff)) { SettingLabel("流量嗅探", "从 TLS / HTTP 中识别域名，规则更准") }
                LabeledContent {
                    Text(perAppSummary(s)).foregroundStyle(.secondary)
                } label: {
                    SettingLabel("分应用代理", "在侧边栏「分应用代理」中设置")
                }
            }

            Section {
                if r.rules.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("还没有自定义规则").font(.headline)
                        Text("自定义规则优先于上面的模式。例：让 netflix 走代理、公司域名直连、某个 IP 段拦截。")
                            .font(.callout).foregroundStyle(.secondary)
                        HStack {
                            ForEach([Rule(type: "GEOSITE", value: "openai", action: "PROXY"),
                                     Rule(type: "GEOSITE", value: "netflix", action: "PROXY"),
                                     Rule(type: "GEOSITE", value: "steam@cn", action: "DIRECT")]) { preset in
                                Button {
                                    change { $0.rules.append(preset) }
                                } label: {
                                    Label("\(preset.value) → \(ruleActionLabel(preset.action))", systemImage: "plus")
                                }
                            }
                        }
                    }
                    .padding(.vertical, 4)
                } else {
                    ForEach(Array(r.rules.enumerated()), id: \.element.id) { i, rule in
                        RuleRow(
                            rule: rule,
                            onToggle: { v in change { rt in if let j = rt.rules.firstIndex(where: { $0.id == rule.id }) { rt.rules[j].enabled = v } } },
                            onEdit: { editingIsNew = false; editing = rule },
                            onUp: i > 0 ? { change { $0.rules.swapAt(i, i - 1) } } : nil,
                            onDown: i < r.rules.count - 1 ? { change { $0.rules.swapAt(i, i + 1) } } : nil,
                            onDelete: { change { $0.rules.removeAll { $0.id == rule.id } } }
                        )
                    }
                }
            } header: {
                HStack {
                    Text("自定义规则")
                    Spacer()
                    Button {
                        editingIsNew = true
                        editing = Rule()
                    } label: {
                        Label("添加", systemImage: "plus")
                    }
                    .buttonStyle(.borderless)
                }
            } footer: {
                Text("规则自上而下匹配，命中即停止。").font(.caption).foregroundStyle(.secondary)
            }

            Section("DNS 与规则数据") {
                LabeledContent {
                    Button("编辑…") { dnsEdit = true }
                } label: {
                    SettingLabel("DNS 服务器", "直连 \(r.directDns.first ?? "") · 远程 \(r.remoteDns.first ?? "")")
                }
                Toggle(isOn: toggle(\.fakeIp)) { SettingLabel("Fake-IP", "更快、防 DNS 污染；关闭后使用真实 IP") }
                LabeledContent {
                    if updatingGeo {
                        ProgressView().controlSize(.small)
                    } else {
                        Button("立即更新") { updateGeo() }
                    }
                } label: {
                    SettingLabel("更新 GeoIP / GeoSite", "内核每 24 小时自动更新；下载源可在设置中切换")
                }
            }
        }
        .formStyle(.grouped)
        .navigationTitle("分流")
        .sheet(item: $editing) { rule in
            RuleEditView(original: rule, isNew: editingIsNew) { saved in
                change { rt in
                    if let j = rt.rules.firstIndex(where: { $0.id == saved.id }) {
                        rt.rules[j] = saved
                    } else {
                        rt.rules.append(saved)
                    }
                }
            } onDelete: {
                change { $0.rules.removeAll { $0.id == rule.id } }
            }
        }
        .sheet(isPresented: $dnsEdit) {
            DNSEditView(direct: r.directDns, remote: r.remoteDns) { d, rm in
                change { $0.directDns = d; $0.remoteDns = rm }
            }
        }
    }

    private func perAppSummary(_ s: Settings) -> String {
        switch s.perAppMode {
        case PerAppMode.proxy: return "仅 \(s.perAppList.count) 个应用走代理"
        case PerAppMode.bypass: return "\(s.perAppList.count) 个应用不走代理"
        default: return "关闭"
        }
    }

    private func updateGeo() {
        var p: Profile
        if let sel = store.data.selected {
            p = sel
        } else {
            // any valid profile works: only the geox URLs of the options matter here
            p = Profile()
            p.name = "geo"
            p.server = "127.0.0.1"
            p.username = "u"
            p.password = "p"
            p.bindings = [PortBinding(port: 1, portRange: "", proto: "TCP")]
        }
        let o = model.options(p)
        updatingGeo = true
        Task {
            do {
                let _: Bool = try await offMain { try CoreClient.call("updateGeo", CoreRequest(options: o), timeout: 300) }
                model.show("规则数据已更新")
                model.reloadIfRunning()
            } catch {
                model.show("更新失败：" + error.localizedDescription)
            }
            updatingGeo = false
        }
    }
}

@MainActor
struct SettingLabel: View {
    let title: String
    let subtitle: String
    init(_ title: String, _ subtitle: String) {
        self.title = title
        self.subtitle = subtitle
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
            Text(subtitle).font(.caption).foregroundStyle(.secondary)
        }
    }
}

@MainActor
struct RuleRow: View {
    let rule: Rule
    let onToggle: (Bool) -> Void
    let onEdit: () -> Void
    let onUp: (() -> Void)?
    let onDown: (() -> Void)?
    let onDelete: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Toggle("", isOn: SwiftUI.Binding(get: { rule.enabled }, set: onToggle))
                .labelsHidden()
                .toggleStyle(.checkbox)
            VStack(alignment: .leading, spacing: 2) {
                Text(rule.value).font(.body.monospaced()).lineLimit(1)
                Text(rule.type).font(.caption).foregroundStyle(.secondary)
            }
            .opacity(rule.enabled ? 1 : 0.5)
            Spacer()
            Text(ruleActionLabel(rule.action))
                .font(.caption.weight(.semibold))
                .padding(.horizontal, 8).padding(.vertical, 2)
                .background(actionColor.opacity(0.15), in: Capsule())
                .foregroundStyle(actionColor)
            Button { onUp?() } label: { Image(systemName: "chevron.up") }
                .buttonStyle(.borderless).disabled(onUp == nil)
            Button { onDown?() } label: { Image(systemName: "chevron.down") }
                .buttonStyle(.borderless).disabled(onDown == nil)
            Button { onEdit() } label: { Image(systemName: "pencil") }
                .buttonStyle(.borderless)
        }
        .contextMenu {
            Button("编辑…", action: onEdit)
            Button("删除", role: .destructive, action: onDelete)
        }
    }

    private var actionColor: Color {
        switch rule.action {
        case "DIRECT": return .green
        case "REJECT": return .red
        default: return .accentColor
        }
    }
}

@MainActor
struct RuleEditView: View {
    @Environment(\.dismiss) private var dismiss
    let original: Rule
    let isNew: Bool
    let onSave: (Rule) -> Void
    let onDelete: () -> Void

    @State private var type = "DOMAIN-SUFFIX"
    @State private var value = ""
    @State private var action = "PROXY"
    @State private var error: String?
    @State private var saving = false

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(isNew ? "添加规则" : "编辑规则").font(.title3.weight(.semibold))
            Form {
                Picker("类型", selection: $type) {
                    ForEach(ruleTypes, id: \.self) { Text($0).tag($0) }
                }
                TextField("内容", text: $value, prompt: Text(ruleHints[type] ?? ""))
                Picker("动作", selection: $action) {
                    ForEach(ruleActions.indices, id: \.self) { i in
                        Text(ruleActions[i].1).tag(ruleActions[i].0)
                    }
                }
                .pickerStyle(.segmented)
            }
            if let error {
                Text(error).foregroundStyle(.red).font(.callout)
            }
            HStack {
                if !isNew {
                    Button("删除", role: .destructive) {
                        onDelete()
                        dismiss()
                    }
                }
                Spacer()
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("保存") { save() }.keyboardShortcut(.defaultAction).disabled(saving)
            }
        }
        .padding(20)
        .frame(width: 440)
        .onAppear {
            type = original.type
            value = original.value
            action = original.action
        }
    }

    private func save() {
        var r = original
        r.type = type
        r.value = value
        r.action = action
        let draft = r
        saving = true
        Task {
            do {
                var checked: Rule = try await offMain { try CoreClient.call("checkRule", CoreRequest(rule: draft)) }
                checked.id = draft.id
                checked.enabled = draft.enabled
                onSave(checked)
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
            saving = false
        }
    }
}

@MainActor
struct DNSEditView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var direct: String
    @State private var remote: String
    let onSave: ([String], [String]) -> Void

    init(direct: [String], remote: [String], onSave: @escaping ([String], [String]) -> Void) {
        _direct = State(initialValue: direct.joined(separator: "\n"))
        _remote = State(initialValue: remote.joined(separator: "\n"))
        self.onSave = onSave
    }

    private func lines(_ s: String) -> [String] {
        s.split(whereSeparator: \.isNewline).map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("DNS 服务器").font(.title3.weight(.semibold))
            Text("直连 DNS（国内）").font(.headline)
            TextEditor(text: $direct).font(.body.monospaced()).frame(height: 80)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(Color.secondary.opacity(0.3)))
            Text("每行一个，支持 https:// tls:// 或 IP").font(.caption).foregroundStyle(.secondary)
            Text("远程 DNS（经代理）").font(.headline)
            TextEditor(text: $remote).font(.body.monospaced()).frame(height: 80)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(Color.secondary.opacity(0.3)))
            Text("用于解析被污染的域名").font(.caption).foregroundStyle(.secondary)
            HStack {
                Spacer()
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("保存") {
                    onSave(lines(direct), lines(remote))
                    dismiss()
                }
                .keyboardShortcut(.defaultAction)
                .disabled(lines(direct).isEmpty || lines(remote).isEmpty)
            }
        }
        .padding(20)
        .frame(width: 460)
    }
}

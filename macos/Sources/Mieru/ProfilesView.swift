import AppKit
import CoreImage
import CoreImage.CIFilterBuiltins
import SwiftUI
import UniformTypeIdentifiers

@MainActor
struct ProfilesView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @State private var editing: Profile?
    @State private var sharing: Profile?
    @State private var deleting: Profile?

    var body: some View {
        let d = store.data
        Group {
            if d.profiles.isEmpty {
                VStack(spacing: 12) {
                    Image(systemName: "server.rack").font(.system(size: 40)).foregroundStyle(.secondary)
                    Text("还没有节点").font(.title3.weight(.semibold))
                    Text("在 mieru-ui 面板的「用户管理 → 分享」里复制 mierus:// 链接，\n然后点右上角「添加」从剪贴板导入，或导入二维码图片。")
                        .multilineTextAlignment(.center)
                        .foregroundStyle(.secondary)
                    HStack {
                        Button("从剪贴板导入") { importClipboard() }.buttonStyle(.borderedProminent)
                        Button("手动添加") { editing = Profile() }
                    }
                }
                .padding(40)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    ForEach(d.profiles) { p in
                        ProfileRow(profile: p, selected: p.id == d.selected?.id)
                            .contentShape(Rectangle())
                            .onTapGesture { model.select(p) }
                            .contextMenu {
                                Button("设为当前节点") { model.select(p) }
                                Button("测速") { model.testDelay(p) }
                                Divider()
                                Button("编辑…") { editing = p }
                                Button("分享…") { sharing = p }
                                Button("复制链接") { copyLink(p) }
                                Divider()
                                Button("删除", role: .destructive) { deleting = p }
                            }
                    }
                    .onMove { store.moveProfiles(from: $0, to: $1) }
                }
                .listStyle(.inset(alternatesRowBackgrounds: false))
            }
        }
        .navigationTitle("节点")
        .navigationSubtitle("\(d.profiles.count) 个节点")
        .toolbar {
            ToolbarItemGroup {
                Button { model.testAll() } label: { Label("全部测速", systemImage: "speedometer") }
                    .disabled(d.profiles.isEmpty)
                Menu {
                    Button("从剪贴板导入") { importClipboard() }
                    Button("导入二维码图片…") { importQRImage() }
                    Button("导入 mieru 客户端配置文件…") { importFile() }
                    Divider()
                    Button("手动添加…") { editing = Profile() }
                } label: {
                    Label("添加", systemImage: "plus")
                }
            }
        }
        .sheet(item: $editing) { p in
            ProfileEditView(original: p)
        }
        .sheet(item: $sharing) { p in
            ShareView(profile: p)
        }
        .confirmationDialog("删除节点", isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } }), presenting: deleting) { p in
            Button("删除「\(p.name)」", role: .destructive) {
                let wasSelected = p.id == store.data.selected?.id
                store.deleteProfile(p.id)
                if wasSelected { model.reloadIfRunning() }
                deleting = nil
            }
        } message: { p in
            Text("确定删除「\(p.name)」？")
        }
    }

    private func importClipboard() {
        guard let text = NSPasteboard.general.string(forType: .string), !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            model.show("剪贴板是空的")
            return
        }
        Task { await model.importText(text) }
    }

    private func importQRImage() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.image]
        panel.allowsMultipleSelection = true
        panel.message = "选择包含 mierus:// 二维码的图片"
        guard panel.runModal() == .OK else { return }
        var texts: [String] = []
        for url in panel.urls {
            texts.append(contentsOf: QR.decode(url: url))
        }
        if texts.isEmpty {
            model.show("图片中没有找到二维码")
            return
        }
        Task { await model.importText(texts.joined(separator: "\n")) }
    }

    private func importFile() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json, .plainText, .text]
        guard panel.runModal() == .OK, let url = panel.url,
              let text = try? String(contentsOf: url, encoding: .utf8) else { return }
        Task { await model.importText(text) }
    }

    private func copyLink(_ p: Profile) {
        Task {
            do {
                let link: String = try await offMain { try CoreClient.call("link", CoreRequest(profile: p)) }
                copyToPasteboard(link)
                model.show("已复制（链接包含密码）")
            } catch {
                model.show(error.localizedDescription)
            }
        }
    }
}

@MainActor
struct ProfileRow: View {
    let profile: Profile
    let selected: Bool

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                .font(.title3)
                .foregroundStyle(selected ? Color.accentColor : Color.secondary.opacity(0.5))
            VStack(alignment: .leading, spacing: 3) {
                Text(profile.name).font(.body.weight(selected ? .semibold : .regular))
                Text(profile.summary).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer()
            DelayBadge(profile: profile)
        }
        .padding(.vertical, 6)
    }
}

// MARK: - Edit

let multiplexingLevels: [(String, String)] = [
    ("", "默认"), ("MULTIPLEXING_OFF", "关闭"), ("MULTIPLEXING_LOW", "低"),
    ("MULTIPLEXING_MIDDLE", "中"), ("MULTIPLEXING_HIGH", "高"),
]
let handshakeModes: [(String, String)] = [
    ("", "默认"), ("HANDSHAKE_STANDARD", "标准"), ("HANDSHAKE_NO_WAIT", "不等待（更快）"),
]

@MainActor
struct ProfileEditView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store
    @Environment(\.dismiss) private var dismiss

    let original: Profile
    @State private var p = Profile()
    @State private var ports: [PortRow] = []
    @State private var mtuText = ""
    @State private var error: String?
    @State private var saving = false

    struct PortRow: Identifiable {
        let id = UUID()
        var text: String
        var proto: String
    }

    private var isNew: Bool { !store.data.profiles.contains { $0.id == original.id } }

    var body: some View {
        VStack(spacing: 0) {
            Form {
                Section("基本") {
                    TextField("名称", text: $p.name, prompt: Text("留空则使用 用户名@服务器"))
                    TextField("服务器", text: $p.server, prompt: Text("IP 或域名"))
                    TextField("用户名", text: $p.username)
                    SecureField("密码", text: $p.password)
                }
                Section {
                    ForEach($ports) { $row in
                        HStack {
                            TextField("端口或范围", text: $row.text, prompt: Text("2012 或 3000-3010"))
                            Picker("", selection: $row.proto) {
                                Text("TCP").tag("TCP")
                                Text("UDP").tag("UDP")
                            }
                            .labelsHidden()
                            .frame(width: 90)
                            Button {
                                ports.removeAll { $0.id == row.id }
                            } label: {
                                Image(systemName: "minus.circle")
                            }
                            .buttonStyle(.borderless)
                            .disabled(ports.count <= 1)
                        }
                    }
                    Button {
                        ports.append(PortRow(text: "", proto: "TCP"))
                    } label: {
                        Label("添加端口", systemImage: "plus")
                    }
                    .buttonStyle(.borderless)
                } header: {
                    Text("端口")
                }
                Section("高级") {
                    TextField("MTU", text: $mtuText, prompt: Text("默认"))
                    Picker("多路复用", selection: $p.multiplexing) {
                        ForEach(multiplexingLevels.indices, id: \.self) { i in
                            Text(multiplexingLevels[i].1).tag(multiplexingLevels[i].0)
                        }
                    }
                    Picker("握手模式", selection: $p.handshakeMode) {
                        ForEach(handshakeModes.indices, id: \.self) { i in
                            Text(handshakeModes[i].1).tag(handshakeModes[i].0)
                        }
                    }
                }
                if let error {
                    Text(error).foregroundStyle(.red)
                }
            }
            .formStyle(.grouped)

            Divider()
            HStack {
                Spacer()
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Button(isNew ? "添加" : "保存") { save() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(saving)
            }
            .padding(14)
        }
        .frame(width: 500, height: 600)
        .onAppear {
            p = original
            ports = original.bindings.map { PortRow(text: $0.portText == "0" ? "" : $0.portText, proto: $0.proto) }
            if ports.isEmpty { ports = [PortRow(text: "", proto: "TCP")] }
            mtuText = original.mtu > 0 ? String(original.mtu) : ""
        }
    }

    private func save() {
        var draft = p
        draft.name = draft.name.trimmingCharacters(in: .whitespaces)
        draft.server = draft.server.trimmingCharacters(in: .whitespaces)
        draft.username = draft.username.trimmingCharacters(in: .whitespaces)
        if draft.name.isEmpty { draft.name = "\(draft.username)@\(draft.server)" }
        let mtu = mtuText.trimmingCharacters(in: .whitespaces)
        if mtu.isEmpty {
            draft.mtu = 0
        } else if let v = Int(mtu) {
            draft.mtu = v
        } else {
            error = "MTU 必须是数字"
            return
        }
        var bindings: [PortBinding] = []
        for row in ports {
            let t = row.text.trimmingCharacters(in: .whitespaces)
            if t.isEmpty { continue }
            if t.contains("-") {
                bindings.append(PortBinding(port: 0, portRange: t, proto: row.proto))
            } else if let v = Int(t) {
                bindings.append(PortBinding(port: v, portRange: "", proto: row.proto))
            } else {
                error = "端口无效：\(t)"
                return
            }
        }
        draft.bindings = bindings
        saving = true
        let id = draft.id
        Task {
            do {
                var checked: Profile = try await offMain { try CoreClient.call("checkProfile", CoreRequest(profile: draft)) }
                checked.id = id
                store.upsertProfile(checked)
                if store.data.selected?.id == id { model.reloadIfRunning() }
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
            saving = false
        }
    }
}

// MARK: - Share

@MainActor
struct ShareView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let profile: Profile
    @State private var link: String?
    @State private var error: String?

    var body: some View {
        VStack(spacing: 14) {
            Text(profile.name).font(.title3.weight(.semibold))
            if let link, let img = QR.image(for: link) {
                Image(nsImage: img)
                    .interpolation(.none)
                    .resizable()
                    .frame(width: 240, height: 240)
                    .padding(10)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: 10))
                Text(link)
                    .font(.caption.monospaced())
                    .textSelection(.enabled)
                    .lineLimit(3)
                    .frame(maxWidth: 360)
                Text("链接包含密码，请只分享给自己的设备。").font(.caption).foregroundStyle(.secondary)
            } else if let error {
                Text(error).foregroundStyle(.red)
            } else {
                ProgressView()
            }
            HStack {
                Button("复制链接") {
                    if let link {
                        copyToPasteboard(link)
                        model.show("已复制")
                    }
                }
                .disabled(link == nil)
                Button("关闭") { dismiss() }.keyboardShortcut(.defaultAction)
            }
        }
        .padding(24)
        .frame(width: 420)
        .task {
            do {
                let p = profile
                link = try await offMain { try CoreClient.call("link", CoreRequest(profile: p)) }
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

enum QR {
    static func image(for text: String) -> NSImage? {
        let f = CIFilter.qrCodeGenerator()
        f.message = Data(text.utf8)
        f.correctionLevel = "M"
        guard let out = f.outputImage?.transformed(by: CGAffineTransform(scaleX: 8, y: 8)) else { return nil }
        let rep = NSCIImageRep(ciImage: out)
        let img = NSImage(size: rep.size)
        img.addRepresentation(rep)
        return img
    }

    static func decode(url: URL) -> [String] {
        guard let ci = CIImage(contentsOf: url),
              let det = CIDetector(ofType: CIDetectorTypeQRCode, context: nil, options: [CIDetectorAccuracy: CIDetectorAccuracyHigh])
        else { return [] }
        return det.features(in: ci).compactMap { ($0 as? CIQRCodeFeature)?.messageString }
    }
}

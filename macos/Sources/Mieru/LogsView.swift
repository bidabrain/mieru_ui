import AppKit
import SwiftUI

@MainActor
struct LogsView: View {
    @EnvironmentObject var model: AppModel
    @State private var lines: [LogLine] = []
    @State private var next: Int64 = 0
    @State private var follow = true
    @State private var filter = ""

    private static let timeFmt: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()

    private var shown: [LogLine] {
        let q = filter.trimmingCharacters(in: .whitespaces)
        return q.isEmpty ? lines : lines.filter { $0.msg.localizedCaseInsensitiveContains(q) }
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                TextField("过滤", text: $filter).textFieldStyle(.roundedBorder).frame(maxWidth: 260)
                Toggle("自动滚动", isOn: $follow).toggleStyle(.checkbox)
                Spacer()
                Text("\(lines.count) 行").font(.caption).foregroundStyle(.secondary)
            }
            .padding(12)
            Divider()
            if lines.isEmpty {
                Text(model.daemonReachable ? "暂无日志。连接后这里会显示内核输出。" : "核心服务未运行。")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollViewReader { proxy in
                    List(shown) { l in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Text(Self.timeFmt.string(from: Date(timeIntervalSince1970: Double(l.time) / 1000)))
                                .foregroundStyle(.secondary)
                            Text(l.level.uppercased().prefix(4))
                                .foregroundStyle(color(l.level))
                                .frame(width: 38, alignment: .leading)
                            Text(l.msg).textSelection(.enabled)
                        }
                        .font(.system(.caption, design: .monospaced))
                        .id(l.seq)
                    }
                    .listStyle(.plain)
                    .onChange(of: lines.last?.seq) { last in
                        if follow, let last { proxy.scrollTo(last, anchor: .bottom) }
                    }
                }
            }
        }
        .navigationTitle("运行日志")
        .toolbar {
            ToolbarItemGroup {
                Button {
                    copyToPasteboard(shown.map { "\($0.level) \($0.msg)" }.joined(separator: "\n"))
                    model.show("已复制日志")
                } label: { Label("复制", systemImage: "doc.on.doc") }
                Button {
                    NSWorkspace.shared.open(URL(fileURLWithPath: Helper.logFile))
                } label: { Label("核心服务日志", systemImage: "doc.text.magnifyingglass") }
                    .help("在「控制台」中打开 \(Helper.logFile)")
                Button {
                    lines = []
                    Task { _ = try? await background { try CoreClient.call("clearLogs", as: Bool.self) } }
                } label: { Label("清空", systemImage: "trash") }
            }
        }
        .task {
            while !Task.isCancelled {
                await fetch()
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        }
    }

    private func fetch() async {
        guard model.daemonReachable else { return }
        let after = next
        guard let batch: LogBatch = try? await background({ try CoreClient.call("logs", CoreRequest(after: after), timeout: 5) }) else { return }
        if batch.next < next { lines = [] } // core restarted
        next = batch.next
        if !batch.lines.isEmpty {
            lines.append(contentsOf: batch.lines)
            if lines.count > 1000 { lines.removeFirst(lines.count - 1000) }
        }
    }

    private func color(_ level: String) -> Color {
        switch level.lowercased() {
        case "error": return .red
        case "warning": return .orange
        case "debug": return .secondary
        default: return .accentColor
        }
    }
}

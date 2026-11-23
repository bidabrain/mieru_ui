import SwiftUI

@MainActor
struct HomeView: View {
    @EnvironmentObject var model: AppModel
    @EnvironmentObject var store: Store

    var body: some View {
        let d = store.data
        ScrollView {
            VStack(spacing: 16) {
                if !model.daemonReachable || model.helperOutdated {
                    HelperBanner()
                }

                ConnectButton()
                    .padding(.vertical, 12)

                Card {
                    HStack {
                        Text("当前节点").font(.subheadline).foregroundStyle(.secondary)
                        Spacer()
                        if !d.profiles.isEmpty {
                            Menu {
                                ForEach(d.profiles) { p in
                                    Button {
                                        model.select(p)
                                    } label: {
                                        if p.id == d.selected?.id { Label(p.name, systemImage: "checkmark") } else { Text(p.name) }
                                    }
                                }
                            } label: {
                                Text("切换")
                            }
                            .menuStyle(.borderlessButton)
                            .fixedSize()
                        }
                    }
                    if let p = d.selected {
                        HStack(alignment: .center) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(p.name).font(.title3.weight(.semibold))
                                Text(p.summary).font(.callout).foregroundStyle(.secondary).lineLimit(1)
                            }
                            Spacer()
                            DelayBadge(profile: p)
                        }
                    } else {
                        Text("还没有节点，请到「节点」页面添加。").foregroundStyle(.secondary)
                    }
                }

                Card {
                    Text("分流模式").font(.headline)
                    Picker("", selection: Binding(get: { d.routing.mode }, set: { model.setMode($0) })) {
                        ForEach(routeModes.indices, id: \.self) { i in
                            Text(routeModes[i].1).tag(routeModes[i].0)
                        }
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    Text(routeModeHelp[d.routing.mode] ?? "").font(.callout).foregroundStyle(.secondary)
                }

                Card {
                    HStack(spacing: 0) {
                        TrafficStat(title: "上传", icon: "arrow.up", speed: model.traffic.up, total: model.traffic.upTotal)
                        Divider().frame(height: 44)
                        TrafficStat(title: "下载", icon: "arrow.down", speed: model.traffic.down, total: model.traffic.downTotal)
                    }
                    HStack {
                        Text("活动连接 \(model.traffic.conns)")
                        Spacer()
                        if d.settings.mixedPort > 0 {
                            Text("本地代理 127.0.0.1:\(String(d.settings.mixedPort))")
                        }
                    }
                    .font(.caption)
                    .foregroundStyle(.secondary)
                }
            }
            .padding(24)
            .frame(maxWidth: 640)
            .frame(maxWidth: .infinity)
        }
        .navigationTitle("Mieru")
    }
}

@MainActor
struct TrafficStat: View {
    let title: String
    let icon: String
    let speed: Int64
    let total: Int64

    var body: some View {
        VStack(spacing: 4) {
            Label(title, systemImage: icon).font(.caption).foregroundStyle(.secondary)
            Text(formatSpeed(speed)).font(.title3.weight(.semibold).monospacedDigit())
            Text("共 \(formatBytes(total))").font(.caption2).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
    }
}

@MainActor
struct ConnectButton: View {
    @EnvironmentObject var model: AppModel
    @State private var hover = false

    var body: some View {
        let running = model.running
        VStack(spacing: 14) {
            Button { model.toggle() } label: {
                ZStack {
                    Circle()
                        .fill(running ? Color.accentColor : Color(nsColor: .controlBackgroundColor))
                        .shadow(color: (running ? Color.accentColor : .black).opacity(running ? 0.45 : 0.12), radius: running ? 22 : 10)
                    Circle()
                        .stroke(Color.accentColor.opacity(running ? 0 : 0.5), lineWidth: 2)
                    if model.busy {
                        ProgressView().controlSize(.large)
                    } else {
                        Image(systemName: "power")
                            .font(.system(size: 46, weight: .semibold))
                            .foregroundStyle(running ? Color.white : Color.accentColor)
                    }
                }
                .frame(width: 140, height: 140)
                .scaleEffect(hover ? 1.04 : 1)
                .animation(.spring(response: 0.3), value: hover)
                .animation(.easeInOut(duration: 0.25), value: running)
            }
            .buttonStyle(.plain)
            .onHover { hover = $0 }
            .disabled(model.busy)

            Text(stateText)
                .font(.headline)
                .foregroundStyle(running ? Color.accentColor : .secondary)
        }
    }

    private var stateText: String {
        switch model.state {
        case .stopped: return "未连接 · 点击连接"
        case .starting: return "正在连接…"
        case .running: return "已连接"
        case .stopping: return "正在断开…"
        }
    }
}

@MainActor
struct HelperBanner: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "lock.shield").font(.title2).foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 4) {
                Text(model.helperOutdated ? "核心服务需要更新" : "需要安装核心服务").font(.headline)
                Text(model.helperOutdated
                     ? "已安装的核心服务版本（\(model.coreStatus.version)）与应用（\(Helper.appVersion)）不一致，请重新安装。"
                     : "Mieru 通过系统虚拟网卡（TUN）接管流量，需要一次管理员授权来安装后台核心服务。")
                    .font(.callout).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer()
            Button(model.helperOutdated ? "更新" : "安装") { model.installHelper() }
                .buttonStyle(.borderedProminent)
                .disabled(model.installing)
        }
        .padding(14)
        .background(Color.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

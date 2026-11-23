import AppKit
import Foundation

struct UpdateInfo: Equatable {
    let version: String
    let notes: String
    let assetName: String
    let assetURL: URL
    let size: Int64
    let pageURL: URL
}

enum UpdateState: Equatable {
    case idle, checking, upToDate
    case available(UpdateInfo)
    case downloading(UpdateInfo, Double)
    case ready(UpdateInfo, URL)
    case failed(String)
}

/// Checks GitHub releases tagged `macos-v*` and downloads the new .dmg.
@MainActor
final class Updater: ObservableObject {
    static let shared = Updater()
    static let repo = "bidabrain/mieru_ui"
    static let tagPrefix = "macos-v"

    @Published var state: UpdateState = .idle

    func checkOnLaunch() {
        let s = Store.shared.data.settings
        guard s.autoCheckUpdate, Date().timeIntervalSince1970 - s.lastUpdateCheck > 12 * 3600 else { return }
        Task { await check(silent: true) }
    }

    static func compare(_ a: String, _ b: String) -> Int {
        let pa = a.split(whereSeparator: { !$0.isNumber }).compactMap { Int($0) }
        let pb = b.split(whereSeparator: { !$0.isNumber }).compactMap { Int($0) }
        for i in 0..<max(pa.count, pb.count) {
            let x = i < pa.count ? pa[i] : 0, y = i < pb.count ? pb[i] : 0
            if x != y { return x < y ? -1 : 1 }
        }
        return 0
    }

    func check(silent: Bool = false) async {
        if !silent { state = .checking }
        do {
            var req = URLRequest(url: URL(string: "https://api.github.com/repos/\(Self.repo)/releases?per_page=30")!)
            req.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
            req.setValue("Mieru-macOS/\(Helper.appVersion)", forHTTPHeaderField: "User-Agent")
            req.timeoutInterval = 20
            let (data, resp) = try await URLSession.shared.data(for: req)
            let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
            guard code == 200 else { throw CoreError("GitHub 返回 \(code)") }
            guard let list = try JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { throw CoreError("GitHub 返回格式错误") }
            Store.shared.updateSettings { $0.lastUpdateCheck = Date().timeIntervalSince1970 }

            var best: UpdateInfo?
            for r in list {
                guard let tag = r["tag_name"] as? String, tag.hasPrefix(Self.tagPrefix),
                      (r["draft"] as? Bool) != true, (r["prerelease"] as? Bool) != true else { continue }
                let ver = String(tag.dropFirst(Self.tagPrefix.count))
                if let b = best, Self.compare(ver, b.version) <= 0 { continue }
                let assets = r["assets"] as? [[String: Any]] ?? []
                guard let a = assets.first(where: { ($0["name"] as? String)?.hasSuffix(".dmg") == true }),
                      let name = a["name"] as? String,
                      let urlStr = a["browser_download_url"] as? String, let url = URL(string: urlStr),
                      let page = URL(string: r["html_url"] as? String ?? "") else { continue }
                best = UpdateInfo(version: ver, notes: r["body"] as? String ?? "", assetName: name, assetURL: url,
                                  size: (a["size"] as? NSNumber)?.int64Value ?? 0, pageURL: page)
            }
            if let b = best, Helper.appVersion == "dev" || Self.compare(b.version, Helper.appVersion) > 0 {
                state = .available(b)
            } else {
                state = silent ? .idle : .upToDate
            }
        } catch {
            state = silent ? .idle : .failed("检查失败：\(error.localizedDescription)")
        }
    }

    func download(_ info: UpdateInfo) {
        state = .downloading(info, 0)
        Task {
            do {
                let (bytes, resp) = try await URLSession.shared.bytes(from: info.assetURL)
                let total = max(resp.expectedContentLength, info.size)
                let dir = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first
                    ?? FileManager.default.temporaryDirectory
                let dest = dir.appendingPathComponent(info.assetName)
                let tmp = dest.appendingPathExtension("part")
                FileManager.default.createFile(atPath: tmp.path, contents: nil)
                let fh = try FileHandle(forWritingTo: tmp)
                var buf = Data()
                var done: Int64 = 0
                for try await b in bytes {
                    buf.append(b)
                    if buf.count >= 1 << 16 {
                        try fh.write(contentsOf: buf)
                        done += Int64(buf.count)
                        buf.removeAll(keepingCapacity: true)
                        if total > 0 { state = .downloading(info, Double(done) / Double(total)) }
                    }
                }
                try fh.write(contentsOf: buf)
                try fh.close()
                try? FileManager.default.removeItem(at: dest)
                try FileManager.default.moveItem(at: tmp, to: dest)
                state = .ready(info, dest)
            } catch {
                state = .failed("下载失败：\(error.localizedDescription)")
            }
        }
    }

    /// Opens the downloaded disk image; the user drags the new app to Applications.
    func install(_ file: URL) {
        NSWorkspace.shared.open(file)
    }
}

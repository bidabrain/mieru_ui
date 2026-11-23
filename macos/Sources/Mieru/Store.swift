import Foundation
import SwiftUI // Array.move(fromOffsets:toOffset:)

/// Single JSON file holding everything the user configures
/// (~/Library/Application Support/Mieru/data.json).
final class Store: ObservableObject {
    static let shared = Store()

    @Published private(set) var data = AppData()

    let dir: URL
    private let file: URL
    private let io = DispatchQueue(label: "mieru.store")

    private init() {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        dir = base.appendingPathComponent("Mieru", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        file = dir.appendingPathComponent("data.json")
        if let raw = try? Data(contentsOf: file) {
            do {
                data = try JSONDecoder().decode(AppData.self, from: raw)
            } catch {
                NSLog("Mieru: load data failed: \(error)")
                try? FileManager.default.copyItem(at: file, to: dir.appendingPathComponent("data.broken.json"))
            }
        }
    }

    func update(_ fn: (inout AppData) -> Void) {
        var d = data
        fn(&d)
        data = d
        let snapshot = d
        let file = self.file
        io.async {
            let enc = JSONEncoder()
            enc.outputFormatting = [.prettyPrinted, .sortedKeys]
            guard let raw = try? enc.encode(snapshot) else { return }
            try? raw.write(to: file, options: .atomic)
        }
    }

    func updateRouting(_ fn: (inout Routing) -> Void) { update { fn(&$0.routing) } }
    func updateSettings(_ fn: (inout Settings) -> Void) { update { fn(&$0.settings) } }

    func upsertProfile(_ p: Profile) {
        update { d in
            if let i = d.profiles.firstIndex(where: { $0.id == p.id }) {
                d.profiles[i] = p
            } else {
                d.profiles.append(p)
            }
            if d.selectedId == nil { d.selectedId = p.id }
        }
    }

    func addProfiles(_ ps: [Profile]) {
        update { d in
            d.profiles.append(contentsOf: ps)
            if d.selectedId == nil { d.selectedId = ps.first?.id }
        }
    }

    func deleteProfile(_ id: String) {
        update { d in
            d.profiles.removeAll { $0.id == id }
            if d.selectedId == id { d.selectedId = d.profiles.first?.id }
        }
    }

    func moveProfiles(from: IndexSet, to: Int) {
        update { $0.profiles.move(fromOffsets: from, toOffset: to) }
    }

    func select(_ id: String) { update { $0.selectedId = id } }
}

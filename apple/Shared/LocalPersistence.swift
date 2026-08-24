import Foundation
import PriceCore

struct TriggerRecord: Codable, Identifiable, Hashable {
    let id: String
    let symbol: String
    let direction: TriggerDirection
    let changePercent: Decimal
    let windowMinutes: Int
    let thresholdText: String
    let priceText: String
    let eventTime: Int64
}

enum LocalPersistence {
    private static let encoder = JSONEncoder()
    private static let decoder = JSONDecoder()

    static func loadRules() -> [AlertRule] { load("rules.json") }
    static func saveRules(_ rules: [AlertRule]) { save(rules, name: "rules.json") }
    static func loadHistory() -> [TriggerRecord] { load("history.json") }
    static func saveHistory(_ history: [TriggerRecord]) { save(history, name: "history.json") }

    static func loadPrices() -> [PricePoint] {
        let cutoff = Int64(Date().addingTimeInterval(-60 * 60).timeIntervalSince1970 * 1_000)
        guard let files = try? FileManager.default.contentsOfDirectory(at: priceDirectory, includingPropertiesForKeys: nil) else { return [] }
        return files.filter { $0.pathExtension == "json" }.flatMap { file -> [PricePoint] in
            guard let data = try? Data(contentsOf: file),
                  let points = try? decoder.decode([PricePoint].self, from: data) else { return [] }
            return points.filter { $0.eventTime >= cutoff }
        }
    }

    static func savePrices(_ points: [PricePoint], symbol: String) {
        guard symbol.range(of: "^[A-Z0-9]{2,30}$", options: .regularExpression) != nil,
              let data = try? encoder.encode(points) else { return }
        try? FileManager.default.createDirectory(at: priceDirectory, withIntermediateDirectories: true)
        try? data.write(to: priceDirectory.appending(path: "\(symbol).json"), options: .atomic)
    }

    private static func load<T: Decodable>(_ name: String) -> [T] {
        guard let data = try? Data(contentsOf: file(name)),
              let values = try? decoder.decode([T].self, from: data) else { return [] }
        return values
    }

    private static func save<T: Encodable>(_ values: [T], name: String) {
        guard let data = try? encoder.encode(values) else { return }
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try? data.write(to: file(name), options: .atomic)
    }

    private static var directory: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appending(path: "PriceReminder", directoryHint: .isDirectory)
    }

    private static var priceDirectory: URL {
        directory.appending(path: "prices", directoryHint: .isDirectory)
    }

    private static func file(_ name: String) -> URL { directory.appending(path: name) }
}

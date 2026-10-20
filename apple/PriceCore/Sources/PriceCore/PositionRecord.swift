import Foundation

public struct PositionRecord: Codable, Identifiable, Equatable, Sendable {
    public let id: UUID
    public var setting: EntryPriceSetting
    public init(id: UUID = UUID(), setting: EntryPriceSetting) { self.id = id; self.setting = setting }
}

public struct PositionStore {
    public let directory: URL
    public init(directory: URL) { self.directory = directory }
    public func load() throws -> [PositionRecord] {
        let file = directory.appendingPathComponent("positions.json")
        if FileManager.default.fileExists(atPath: file.path) {
            let records = try JSONDecoder().decode([PositionRecord].self, from: Data(contentsOf: file))
            guard Set(records.map(\.id)).count == records.count else { throw CocoaError(.fileReadCorruptFile) }
            return records
        }
        let legacy = directory.appendingPathComponent("entry-prices.json")
        let settings = FileManager.default.fileExists(atPath: legacy.path)
            ? try JSONDecoder().decode([EntryPriceSetting].self, from: Data(contentsOf: legacy)) : []
        let records = settings.map { PositionRecord(setting: $0) }
        try save(records)
        return records
    }
    public func save(_ records: [PositionRecord]) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try JSONEncoder().encode(records).write(to: directory.appendingPathComponent("positions.json"), options: .atomic)
    }
}

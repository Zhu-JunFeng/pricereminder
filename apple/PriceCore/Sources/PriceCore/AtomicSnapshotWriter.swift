import Foundation

/// Serializes file access. Only successful writes enter the cache.
public final class AtomicSnapshotWriter: @unchecked Sendable {
    private let lock = NSLock()
    private var saved: [URL: Data] = [:]
    public init() {}

    @discardableResult
    public func write(_ data: Data, to url: URL) throws -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if saved[url] == nil { saved[url] = try? Data(contentsOf: url) }
        guard saved[url] != data else { return false }
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try data.write(to: url, options: .atomic)
        saved[url] = data
        return true
    }
}

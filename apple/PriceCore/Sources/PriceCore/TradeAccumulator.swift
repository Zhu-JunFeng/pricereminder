import Foundation

/// Keeps only the newest trade per contract between one-second delivery ticks.
public final class TradeAccumulator: @unchecked Sendable {
    private let lock = NSLock()
    private var pending: [String: PricePoint] = [:]
    private var emitted: [String: Int64] = [:]
    public init() {}
    public func add(_ point: PricePoint) {
        lock.lock(); defer { lock.unlock() }
        guard point.eventTime > (emitted[point.symbol] ?? -1),
              point.eventTime > (pending[point.symbol]?.eventTime ?? -1) else { return }
        pending[point.symbol] = point
    }
    public func drain() -> [PricePoint] {
        lock.lock(); defer { lock.unlock() }
        let points = pending.values.sorted { $0.symbol < $1.symbol }
        for point in points { emitted[point.symbol] = point.eventTime }
        pending.removeAll(keepingCapacity: true)
        return points
    }
}

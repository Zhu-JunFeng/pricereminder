import Foundation

public struct PriceBuffer: Sendable {
    public static let retentionMilliseconds: Int64 = 60 * 60 * 1_000
    private var pointsBySymbol: [String: [PricePoint]] = [:]

    public init() {}

    @discardableResult
    public mutating func add(_ point: PricePoint) -> Bool {
        var points = pointsBySymbol[point.symbol, default: []]
        if let last = points.last {
            guard point.eventTime >= last.eventTime else { return false }
            if point.eventTime / 1_000 == last.eventTime / 1_000 {
                points[points.count - 1] = point
                pointsBySymbol[point.symbol] = points
                return true
            }
        }

        points.append(point)
        let cutoff = point.eventTime - Self.retentionMilliseconds
        if let first = points.firstIndex(where: { $0.eventTime >= cutoff }), first > points.startIndex {
            points.removeFirst(first)
        }
        pointsBySymbol[point.symbol] = points
        return true
    }

    public func point(atOrBefore eventTime: Int64, symbol: String) -> PricePoint? {
        guard let points = pointsBySymbol[symbol], !points.isEmpty else { return nil }
        var lower = 0
        var upper = points.count
        while lower < upper {
            let middle = (lower + upper) / 2
            if points[middle].eventTime <= eventTime { lower = middle + 1 } else { upper = middle }
        }
        return lower == 0 ? nil : points[lower - 1]
    }

    public func latest(symbol: String) -> PricePoint? { pointsBySymbol[symbol]?.last }

    public func covers(symbol: String, durationMilliseconds: Int64) -> Bool {
        guard let points = pointsBySymbol[symbol], points.count >= 2,
              let first = points.first, let last = points.last else { return false }
        return last.eventTime - first.eventTime >= durationMilliseconds
    }

    public func points(symbol: String, after eventTime: Int64) -> [PricePoint] {
        pointsBySymbol[symbol, default: []].filter { $0.eventTime > eventTime }
    }

    public var symbols: Set<String> { Set(pointsBySymbol.keys) }

    public func points(symbol: String) -> [PricePoint] { pointsBySymbol[symbol, default: []] }

    public mutating func restore(_ points: some Sequence<PricePoint>) {
        for point in points.sorted(by: { lhs, rhs in
            lhs.eventTime == rhs.eventTime ? lhs.symbol < rhs.symbol : lhs.eventTime < rhs.eventTime
        }) {
            add(point)
        }
    }
}

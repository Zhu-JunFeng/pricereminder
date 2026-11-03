import Foundation

public struct GoldQuote: Decodable, Sendable {
    public static let symbol = "SGE_AU9999"
    public static let contract = Contract(symbol: symbol, baseAsset: "上海黄金 Au99.99", quoteAsset: "CNY", tickSize: "0.01")
    public let price: String
    public let previousClose: String
    public let eventTime: Int64
    public let receivedAt: Int64
    public let state: String
    public let source: String
    public var eligible: Bool { state == "quoted" && Int64(Date().timeIntervalSince1970 * 1000) - eventTime <= 120_000 }
    public var quoteTimeText: String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 8 * 3600)
        formatter.dateFormat = "HH:mm:ss"
        return formatter.string(from: Date(timeIntervalSince1970: Double(eventTime) / 1000))
    }
    public var change: EntryPriceChange? {
        guard let p = Decimal(string: price), let close = Decimal(string: previousClose), close > 0 else { return nil }
        return EntryPriceCalculator.change(currentPrice: p, entryPrice: close, positionSide: .long)
    }
}

public extension PriceBuffer {
    mutating func remove(symbol: String) {
        let keep = symbols.filter { $0 != symbol }.flatMap { points(symbol: $0) }
        self = PriceBuffer()
        restore(keep)
    }
}

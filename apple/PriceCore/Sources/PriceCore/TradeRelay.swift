import Foundation

public struct TradeRelayConfiguration: Encodable, Sendable {
    public let type = "configure"
    public let `protocol` = 2
    public let version: Int64
    public let symbols: [String]
    public let marketRules: [MarketAlertRule]
    public init(version: Int64, symbols: [String], marketRules: [MarketAlertRule]) {
        self.version = version; self.symbols = symbols.sorted(); self.marketRules = marketRules.filter(\.isEnabled).sorted { $0.id.uuidString < $1.id.uuidString }
    }
}
public struct TradeRelayBatch: Decodable, Sendable {
    public let type: String
    public let `protocol`: Int
    public let version: Int64
    public let session: String
    public let eventId: String?
    public let prices: [WirePrice]?
    public let triggers: [IOSBackgroundTrigger]?
    public let latestEventTime: Int64
    public struct WirePrice: Decodable, Sendable {
        public let symbol: String
        public let price: String
        public let eventTime: Int64
        public func point() throws -> PricePoint { try PricePoint(symbol: symbol, priceText: price, eventTime: eventTime) }
    }
    public func alerts() -> [AlertTrigger] {
        (triggers ?? []).compactMap { t in
            guard let id = UUID(uuidString: t.ruleId) else { return nil }
            return AlertTrigger(ruleID: id, symbol: t.symbol, kind: .marketPercentage, direction: t.direction,
                changePercent: t.changePct.flatMap { Decimal(string: $0) }, thresholdText: t.thresholdPct ?? "",
                windowMinutes: t.windowMinutes, targetPriceText: nil, priceText: t.price,
                baselinePriceText: t.baselinePrice, eventTime: t.eventTime)
        }
    }
}

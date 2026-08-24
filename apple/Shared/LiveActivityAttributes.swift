#if os(iOS)
import ActivityKit
import Foundation

public struct PriceActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        public let symbol: String
        public let price: String
        public let direction: String
        public let eventTime: Int64

        public init(symbol: String, price: String, direction: String, eventTime: Int64) {
            self.symbol = symbol
            self.price = price
            self.direction = direction
            self.eventTime = eventTime
        }
    }

    public let startedAt: Date

    public init(startedAt: Date) { self.startedAt = startedAt }
}
#endif

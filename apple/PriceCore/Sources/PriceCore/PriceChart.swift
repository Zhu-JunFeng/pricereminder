import Foundation

public struct PriceChart: Decodable, Sendable {
    public struct Point: Decodable, Sendable {
        public let time: Int64
        public let open: String?
        public let high: String?
        public let low: String?
        public let close: String
    }

    public let symbol: String
    public let date: String
    public let source: String
    public let updatedAt: Int64
    public let intervalMS: Int64
    public let open: String?
    public let finalClose: String?
    public let stale: Bool
    public let points: [Point]
}

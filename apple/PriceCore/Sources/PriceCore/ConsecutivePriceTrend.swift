import Foundation

public enum ConsecutivePriceTrend: Equatable, Sendable {
    case rise
    case fall
}

public struct ConsecutivePriceTrendTracker: Sendable {
    private var previousPrice: Decimal?
    private var previousMovement: ConsecutivePriceTrend?

    public init() {}

    public mutating func update(price: Decimal) -> ConsecutivePriceTrend? {
        guard let previousPrice else {
            self.previousPrice = price
            return nil
        }

        let movement: ConsecutivePriceTrend?
        if price > previousPrice {
            movement = .rise
        } else if price < previousPrice {
            movement = .fall
        } else {
            movement = nil
        }

        self.previousPrice = price
        defer { previousMovement = movement }
        guard let movement, movement == previousMovement else { return nil }
        return movement
    }
}

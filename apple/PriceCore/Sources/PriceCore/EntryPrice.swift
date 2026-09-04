import Foundation

public enum PositionSide: String, Codable, Hashable, Sendable {
    case long
    case short

    public var displayName: String {
        switch self {
        case .long: "多"
        case .short: "空"
        }
    }
}

public enum EntryPriceDirection: String, Codable, Sendable {
    case rise
    case fall
    case flat
}

public struct EntryPriceSetting: Codable, Hashable, Sendable {
    public let symbol: String
    public let priceText: String
    public let positionSide: PositionSide

    public var price: Decimal {
        Decimal(string: priceText, locale: Locale(identifier: "en_US_POSIX"))!
    }

    public init(symbol: String, priceText: String, positionSide: PositionSide = .long) throws {
        let normalizedSymbol = symbol.uppercased()
        let normalizedPrice = priceText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalizedSymbol.range(of: "^[A-Z0-9]{2,30}$", options: .regularExpression) != nil,
              let price = Decimal(string: normalizedPrice, locale: Locale(identifier: "en_US_POSIX")),
              price > .zero else {
            throw PriceCoreError.invalidEntryPrice
        }
        self.symbol = normalizedSymbol
        self.priceText = normalizedPrice
        self.positionSide = positionSide
    }

    private enum CodingKeys: String, CodingKey {
        case symbol, priceText, positionSide
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        try self.init(
            symbol: values.decode(String.self, forKey: .symbol),
            priceText: values.decode(String.self, forKey: .priceText),
            positionSide: values.decodeIfPresent(PositionSide.self, forKey: .positionSide) ?? .long
        )
    }
}

public struct EntryPriceChange: Equatable, Sendable {
    public let percentage: Decimal
    public let percentageText: String
    public let direction: EntryPriceDirection
}

public enum EntryPriceCalculator {
    public static func isStale(eventTime: Int64, nowMilliseconds: Int64) -> Bool {
        nowMilliseconds - eventTime > 30_000
    }

    public static func change(
        currentPrice: Decimal, entryPrice: Decimal, positionSide: PositionSide
    ) -> EntryPriceChange {
        let priceDifference = switch positionSide {
        case .long: currentPrice - entryPrice
        case .short: entryPrice - currentPrice
        }
        let rawPercentage = priceDifference / entryPrice * 100
        var source = rawPercentage
        var rounded = Decimal()
        NSDecimalRound(&rounded, &source, 2, .plain)
        if rounded == .zero { rounded = .zero }

        let direction: EntryPriceDirection = if rounded > .zero {
            .rise
        } else if rounded < .zero {
            .fall
        } else {
            .flat
        }
        let absoluteText = decimalText(rounded.magnitude, scale: 2)
        let percentageText = switch direction {
        case .rise: "+\(absoluteText)%"
        case .fall: "-\(absoluteText)%"
        case .flat: "0.00%"
        }
        return EntryPriceChange(
            percentage: rounded, percentageText: percentageText, direction: direction
        )
    }

    private static func decimalText(_ value: Decimal, scale: Int) -> String {
        let number = NSDecimalNumber(decimal: value)
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = false
        formatter.minimumFractionDigits = scale
        formatter.maximumFractionDigits = scale
        return formatter.string(from: number)!
    }
}

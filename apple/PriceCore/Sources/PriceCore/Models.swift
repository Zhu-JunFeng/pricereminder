import Foundation

public struct Contract: Codable, Hashable, Identifiable, Sendable {
    public var id: String { symbol }
    public let symbol: String
    public let baseAsset: String
    public let quoteAsset: String
    public let tickSize: String

    public init(symbol: String, baseAsset: String, quoteAsset: String, tickSize: String) {
        self.symbol = symbol
        self.baseAsset = baseAsset
        self.quoteAsset = quoteAsset
        self.tickSize = tickSize
    }
}

public struct PricePoint: Codable, Hashable, Sendable {
    public let symbol: String
    public let price: Decimal
    public let priceText: String
    public let eventTime: Int64
    public let replay: Bool

    public init(symbol: String, priceText: String, eventTime: Int64, replay: Bool = false) throws {
        guard let price = Decimal(string: priceText, locale: Locale(identifier: "en_US_POSIX")) else {
            throw PriceCoreError.invalidPrice
        }
        self.symbol = symbol
        self.price = price
        self.priceText = priceText
        self.eventTime = eventTime
        self.replay = replay
    }
}

public enum TriggerDirection: String, Codable, Sendable {
    case rise
    case fall
}

public struct AlertRule: Codable, Hashable, Identifiable, Sendable {
    public let id: UUID
    public var symbol: String
    public var windowMinutes: Int
    public var thresholdText: String
    public var isEnabled: Bool
    public var riseTriggered: Bool
    public var fallTriggered: Bool

    public var threshold: Decimal {
        Decimal(string: thresholdText, locale: Locale(identifier: "en_US_POSIX")) ?? .zero
    }

    public init(
        id: UUID = UUID(), symbol: String, windowMinutes: Int, thresholdText: String,
        isEnabled: Bool = true, riseTriggered: Bool = false, fallTriggered: Bool = false
    ) throws {
        guard (1...60).contains(windowMinutes) else { throw PriceCoreError.invalidWindow }
        guard let threshold = Decimal(string: thresholdText, locale: Locale(identifier: "en_US_POSIX")),
              threshold >= Decimal(string: "0.1")!, threshold <= 100 else {
            throw PriceCoreError.invalidThreshold
        }
        self.id = id
        self.symbol = symbol.uppercased()
        self.windowMinutes = windowMinutes
        self.thresholdText = thresholdText
        self.isEnabled = isEnabled
        self.riseTriggered = riseTriggered
        self.fallTriggered = fallTriggered
    }
}

public struct AlertTrigger: Codable, Hashable, Sendable {
    public let ruleID: UUID
    public let symbol: String
    public let direction: TriggerDirection
    public let changePercent: Decimal
    public let thresholdText: String
    public let windowMinutes: Int
    public let priceText: String
    public let baselinePriceText: String
    public let eventTime: Int64
}

public struct IOSBackgroundTrigger: Codable, Hashable, Sendable {
    public let ruleId: String
    public let symbol: String
    public let direction: TriggerDirection
    public let changePct: String
    public let thresholdPct: String
    public let windowMinutes: Int
    public let price: String
    public let baselinePrice: String
    public let eventTime: Int64
}

public struct IOSBackgroundEvent: Codable, Hashable, Identifiable, Sendable {
    public let id: String
    public let symbol: String
    public let eventTime: Int64
    public let triggers: [IOSBackgroundTrigger]
}

public enum MonitorState: Equatable, Sendable {
    case disconnected
    case connecting
    case warmingUp
    case live
    case stale
    case notificationUnavailable
}

public enum PriceStreamEvent: Sendable {
    case price(PricePoint)
    case warmingUp(symbol: String?, reason: String?)
    case ready(serverTime: Int64?)
}

public enum PriceCoreError: LocalizedError {
    case invalidPrice
    case invalidWindow
    case invalidThreshold
    case duplicateRule
    case invalidServerResponse
    case invalidExchangeResponse
    case invalidSubscription

    public var errorDescription: String? {
        switch self {
        case .invalidPrice: "价格格式无效"
        case .invalidWindow: "时间窗口必须在 1 到 60 分钟之间"
        case .invalidThreshold: "变化阈值必须在 0.1% 到 100% 之间"
        case .duplicateRule: "相同合约、窗口和阈值的规则已存在"
        case .invalidServerResponse: "服务器响应无效"
        case .invalidExchangeResponse: "币安响应无效"
        case .invalidSubscription: "至少选择一个、最多选择 50 个有效合约"
        }
    }
}

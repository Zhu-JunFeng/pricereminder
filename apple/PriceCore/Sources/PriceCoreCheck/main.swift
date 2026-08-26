import Foundation
import PriceCore

enum CheckError: Error { case failed(String) }

func expect(_ condition: @autoclosure () -> Bool, _ message: String) throws {
    if !condition() { throw CheckError.failed(message) }
}

func equalThresholdTriggersAndRearms() throws {
    var buffer = PriceBuffer()
    var rule = try AlertRule(symbol: "BTCUSDT", windowMinutes: 1, thresholdText: "5")
    buffer.add(try PricePoint(symbol: "BTCUSDT", priceText: "100", eventTime: 0))
    var current = try PricePoint(symbol: "BTCUSDT", priceText: "105", eventTime: 60_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).map(\.direction) == [.rise], "equal threshold must trigger")
    current = try PricePoint(symbol: "BTCUSDT", priceText: "104", eventTime: 61_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).isEmpty, "inside threshold must not trigger")
    try expect(!rule.riseTriggered, "inside threshold must immediately rearm rise")
    current = try PricePoint(symbol: "BTCUSDT", priceText: "106", eventTime: 62_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).map(\.direction) == [.rise], "rearmed direction must trigger again")
}

func directionsAreIndependent() throws {
    var buffer = PriceBuffer()
    var rule = try AlertRule(symbol: "ETHUSDT", windowMinutes: 1, thresholdText: "5")
    buffer.add(try PricePoint(symbol: "ETHUSDT", priceText: "100", eventTime: 0))
    var current = try PricePoint(symbol: "ETHUSDT", priceText: "106", eventTime: 60_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).map(\.direction) == [.rise], "rise must trigger")
    current = try PricePoint(symbol: "ETHUSDT", priceText: "94", eventTime: 61_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).map(\.direction) == [.fall], "fall must remain independently armed")
}

func incompleteWindowDoesNotTrigger() throws {
    var buffer = PriceBuffer()
    var rule = try AlertRule(symbol: "SOLUSDT", windowMinutes: 5, thresholdText: "2")
    buffer.add(try PricePoint(symbol: "SOLUSDT", priceText: "100", eventTime: 0))
    let current = try PricePoint(symbol: "SOLUSDT", priceText: "110", eventTime: 299_999)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).isEmpty, "incomplete window must not trigger")
}

func targetPriceTriggersAndRearms() throws {
    var buffer = PriceBuffer()
    var rule = try AlertRule(symbol: "BTCUSDT", targetDirection: .above, targetPriceText: "105")
    var current = try PricePoint(symbol: "BTCUSDT", priceText: "104", eventTime: 1_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).isEmpty, "target must wait below threshold")
    current = try PricePoint(symbol: "BTCUSDT", priceText: "105", eventTime: 2_000)
    buffer.add(current)
    let first = RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer)
    try expect(first.count == 1 && first[0].kind == .target && first[0].targetPriceText == "105", "equal target must trigger")
    current = try PricePoint(symbol: "BTCUSDT", priceText: "106", eventTime: 3_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).isEmpty, "target must not repeat while reached")
    current = try PricePoint(symbol: "BTCUSDT", priceText: "104", eventTime: 4_000)
    buffer.add(current)
    try expect(RuleEngine.evaluate(rule: &rule, current: current, buffer: buffer).isEmpty && !rule.targetTriggered, "leaving target must rearm")
}

func restoredBufferKeepsLatestPointPerSecondAndOneHour() throws {
    var buffer = PriceBuffer()
    buffer.restore([
        try PricePoint(symbol: "BTCUSDT", priceText: "99", eventTime: 1_000),
        try PricePoint(symbol: "BTCUSDT", priceText: "100", eventTime: 2_000),
        try PricePoint(symbol: "BTCUSDT", priceText: "101", eventTime: 2_900),
        try PricePoint(symbol: "BTCUSDT", priceText: "102", eventTime: PriceBuffer.retentionMilliseconds + 2_000),
    ])
    try expect(buffer.points(symbol: "BTCUSDT").map(\.priceText) == ["101", "102"], "restore must compact each second and retain one hour")
    try expect(buffer.symbols == ["BTCUSDT"], "restored symbols must be available")
}

func decodesBinanceContractsAndTrades() throws {
    let exchangeInfo = Data("""
    {"symbols":[
      {"symbol":"ETHUSDT","status":"TRADING","contractType":"PERPETUAL","baseAsset":"ETH","quoteAsset":"USDT","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.01"}]},
      {"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL","baseAsset":"BTC","quoteAsset":"USDT","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"}]},
      {"symbol":"BTCUSDC","status":"TRADING","contractType":"PERPETUAL","baseAsset":"BTC","quoteAsset":"USDC","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"}]},
      {"symbol":"OLDUSDT","status":"SETTLING","contractType":"PERPETUAL","baseAsset":"OLD","quoteAsset":"USDT","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.001"}]}
    ]}
    """.utf8)
    let contracts = try BinanceMarketClient.decodeContracts(from: exchangeInfo)
    try expect(contracts.map(\.symbol) == ["BTCUSDT", "ETHUSDT"], "catalog must contain only trading USDT perpetual contracts")
    try expect(contracts.first?.tickSize == "0.10", "catalog must retain the Binance price tick size")

    let trade = Data(#"{"e":"trade","E":1787363070000,"s":"BTCUSDT","p":"64231.50","q":"0.010"}"#.utf8)
    let combinedTrade = Data(#"{"stream":"btcusdt@trade","data":{"e":"trade","E":1787363070001,"s":"BTCUSDT","p":"64232.00","q":"0.020"}}"#.utf8)
    let acknowledgement = Data(#"{"result":null,"id":1}"#.utf8)
    let point = try PriceStream.decodeEvent(from: trade)
    let combinedPoint = try PriceStream.decodeEvent(from: combinedTrade)
    let ignored = try PriceStream.decodeEvent(from: acknowledgement)
    try expect(point?.symbol == "BTCUSDT" && point?.priceText == "64231.50", "trade stream must decode the latest Binance price")
    try expect(combinedPoint?.priceText == "64232.00", "combined trade stream must decode its nested Binance price")
    try expect(ignored == nil, "subscription acknowledgements must not become prices")
}

func decodesServerRelayEvents() throws {
    let price = Data(#"{"type":"price","symbol":"BTCUSDT","price":"64235.10","eventTime":1787363071000,"replay":true}"#.utf8)
    let ready = Data(#"{"type":"ready","serverTime":1787363071001}"#.utf8)
    guard case .price(let point) = try PriceStream.decodeServerEvent(from: price) else {
        throw CheckError.failed("server relay price must decode")
    }
    try expect(point.symbol == "BTCUSDT" && point.replay, "server relay must preserve replay state")
    guard case .ready(let serverTime) = try PriceStream.decodeServerEvent(from: ready) else {
        throw CheckError.failed("server relay ready must decode")
    }
    try expect(serverTime == 1787363071001, "server relay must decode server time")
}

func recentContractsLeadMatchingResultsWithoutChangingTheRest() throws {
    let contracts = [
        Contract(symbol: "ETHUSDT", baseAsset: "ETH", quoteAsset: "USDT", tickSize: "0.01"),
        Contract(symbol: "BTCUSDT", baseAsset: "BTC", quoteAsset: "USDT", tickSize: "0.10"),
        Contract(symbol: "SOLUSDT", baseAsset: "SOL", quoteAsset: "USDT", tickSize: "0.001"),
        Contract(symbol: "BTCDOMUSDT", baseAsset: "BTCDOM", quoteAsset: "USDT", tickSize: "0.10"),
    ]
    let all = ContractOrdering.ordered(contracts, recentSymbols: ["SOLUSDT", "BTCUSDT"])
    try expect(all.map(\.symbol) == ["SOLUSDT", "BTCUSDT", "ETHUSDT", "BTCDOMUSDT"], "recent symbols must lead")
    let matching = ContractOrdering.ordered(contracts, recentSymbols: ["SOLUSDT", "BTCDOMUSDT"], query: "btc")
    try expect(matching.map(\.symbol) == ["BTCDOMUSDT", "BTCUSDT"], "only matching recent symbols must lead search")
}

do {
    try equalThresholdTriggersAndRearms()
    try directionsAreIndependent()
    try incompleteWindowDoesNotTrigger()
    try targetPriceTriggersAndRearms()
    try restoredBufferKeepsLatestPointPerSecondAndOneHour()
    try decodesBinanceContractsAndTrades()
    try decodesServerRelayEvents()
    try recentContractsLeadMatchingResultsWithoutChangingTheRest()
    print("PriceCore checks passed")
} catch {
    fputs("PriceCore check failed: \(error)\n", stderr)
    exit(1)
}

import PriceCore

@main
struct PriceCoreLiveCheck {
    static func main() async throws {
        let market = BinanceMarketClient()
        let contracts = try await market.contracts()
        print("contracts=\(contracts.count)")

        let stream = PriceStream()
        if CommandLine.arguments.contains("--all-market") {
            let updates = try await stream.connectAllMarket(symbols: contracts.map(\.symbol))
            var scanner = MarketScanner()
            let rule = try MarketAlertRule(windowMinutes: 1, thresholdText: "0.1")
            for try await batch in updates {
                print("marketBatch=\(batch.count) first=\(batch.first?.symbol ?? "--")")
                let triggers = batch.flatMap { scanner.evaluate(rule: rule, current: $0) }
                if let trigger = triggers.first {
                    print(
                        "trigger=\(trigger.symbol) direction=\(trigger.direction.rawValue) "
                            + "change=\(trigger.changePercent?.description ?? "--")"
                    )
                }
                if CommandLine.arguments.contains("--wait-for-trigger"), triggers.isEmpty { continue }
                await stream.disconnect()
                return
            }
        }

        let updates = try await stream.connect(symbols: ["BTCUSDT"])
        for try await event in updates {
            guard case .price(let point) = event else { continue }
            print("price=\(point.symbol) \(point.priceText) eventTime=\(point.eventTime)")
            await stream.disconnect()
            return
        }
    }
}

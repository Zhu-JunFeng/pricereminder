import PriceCore

@main
struct PriceCoreLiveCheck {
    static func main() async throws {
        let market = BinanceMarketClient()
        let contracts = try await market.contracts()
        print("contracts=\(contracts.count)")

        let stream = PriceStream()
        let updates = try await stream.connect(symbols: ["BTCUSDT"])
        for try await event in updates {
            guard case .price(let point) = event else { continue }
            print("price=\(point.symbol) \(point.priceText) eventTime=\(point.eventTime)")
            await stream.disconnect()
            return
        }
    }
}

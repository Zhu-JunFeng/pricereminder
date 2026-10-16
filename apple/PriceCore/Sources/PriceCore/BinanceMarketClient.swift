import Foundation

public actor BinanceMarketClient {
    private struct ExchangeInfo: Decodable {
        let symbols: [Symbol]
    }

    private struct Symbol: Decodable {
        let symbol: String
        let status: String
        let contractType: String
        let baseAsset: String
        let quoteAsset: String
        let filters: [Filter]
    }

    private struct Filter: Decodable {
        let filterType: String
        let tickSize: String?
    }

    private let exchangeInfoURL: URL
    private let session: URLSession

    public init(
        exchangeInfoURL: URL = URL(string: "https://fapi.binance.com/fapi/v1/exchangeInfo")!,
        session: URLSession = .shared
    ) {
        self.exchangeInfoURL = exchangeInfoURL
        self.session = session
    }

    public func contracts() async throws -> [Contract] {
        let (data, response) = try await session.data(from: exchangeInfoURL)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw PriceCoreError.invalidExchangeResponse
        }
        return try Self.decodeContracts(from: data)
    }

    public func dailyOpen(symbol: String, start: Int64) async throws -> Decimal {
        var url = URLComponents(url: exchangeInfoURL, resolvingAgainstBaseURL: false)!
        url.path = "/fapi/v1/klines"
        url.queryItems = [
            URLQueryItem(name: "symbol", value: symbol),
            URLQueryItem(name: "interval", value: "1m"),
            URLQueryItem(name: "startTime", value: String(start)),
            URLQueryItem(name: "endTime", value: String(start + 59_999)),
            URLQueryItem(name: "limit", value: "1")
        ]
        let (data, response) = try await session.data(for: URLRequest(url: url.url!, timeoutInterval: 15))
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw PriceCoreError.invalidExchangeResponse
        }
        return try BeijingDailyPrice.decodeOpen(from: data, start: start)
    }

    package static func decodeContracts(from data: Data) throws -> [Contract] {
        let exchangeInfo = try JSONDecoder().decode(ExchangeInfo.self, from: data)
        return exchangeInfo.symbols.compactMap { item in
            guard item.status == "TRADING",
                  item.contractType == "PERPETUAL",
                  (item.quoteAsset == "USDT" || item.quoteAsset == "USDC"),
                  let tickSize = item.filters.first(where: { $0.filterType == "PRICE_FILTER" })?.tickSize else {
                return nil
            }
            return Contract(
                symbol: item.symbol,
                baseAsset: item.baseAsset,
                quoteAsset: item.quoteAsset,
                tickSize: tickSize
            )
        }.sorted { $0.symbol < $1.symbol }
    }
}
